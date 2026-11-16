package netdisco

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gatework/motor/common/secret"

	sdk "github.com/orange-cloudfoundry/go-netdisco"

	"github.com/gatework/motor/app/net/models/inventory"
)

// Config is server-owned; requests cannot override the upstream endpoint.
type Config struct {
	URL, Username    string
	Password, APIKey secret.Secret
}

// FromEnvironment loads the server-owned device transport configuration.
func FromEnvironment() Config {
	return Config{URL: os.Getenv("NETDISCO_URL"), Username: os.Getenv("NETDISCO_USERNAME"), Password: secret.Secret(os.Getenv("NETDISCO_PASSWORD")), APIKey: secret.Secret(os.Getenv("NETDISCO_API_KEY"))}
}

// Client describes netdisco for network operations.
type Client struct {
	config   Config
	timeout  time.Duration
	maxBytes int64
}

// New provides the new operation within the caller's authorized network scope.
func New(config Config) *Client {
	config.URL = strings.TrimRight(strings.TrimSpace(config.URL), "/")
	return &Client{config: config, timeout: 20 * time.Second, maxBytes: 32 << 20}
}

// Configured provides the configured operation within the caller's authorized network scope.
func (p *Client) Configured() bool {
	u, err := url.Parse(p.config.URL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && !strings.ContainsRune(p.config.URL, '#') && (p.config.APIKey.IsSet() || (p.config.Username != "" && p.config.Password.IsSet()))
}

// Fetch provides the fetch operation within the caller's authorized network scope.
func (p *Client) Fetch(ctx context.Context) ([]inventory.SourceFacts, error) {
	if !p.Configured() {
		return nil, inventory.NotConfiguredErr
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	// Each fetch owns its transport/token; no mutable SDK Client state or retries.
	transport := sdk.NewTransport(p.config.APIKey.Reveal(), false)
	if owned, ok := transport.WrapTransport.(*http.Transport); ok {
		defer owned.CloseIdleConnections()
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if !p.config.APIKey.IsSet() {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.URL+"/login", nil)
		if err != nil {
			return nil, inventory.NotConfiguredErr
		}
		req.SetBasicAuth(p.config.Username, p.config.Password.Reveal())
		body, err := readResponse(client, req, 64<<10)
		if err != nil {
			return nil, err
		}
		var login struct {
			APIKey string `json:"api_key"`
		}
		if json.Unmarshal(body, &login) != nil || strings.TrimSpace(login.APIKey) == "" {
			return nil, inventory.AuthenticationErr
		}
		transport.ApiKey = login.APIKey
	}
	query := (&sdk.SearchDeviceQuery{Q: "%", SeeAllColumns: true}).Serialize().Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.URL+"/api/v1/search/device?"+query, nil)
	if err != nil {
		return nil, inventory.NotConfiguredErr
	}
	body, err := readResponse(client, req, p.maxBytes)
	if err != nil {
		return nil, err
	}
	return decodeInventory(body)
}

func readResponse(client *http.Client, req *http.Request, maxBytes int64) ([]byte, error) {
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return nil, inventory.TimeoutErr
		}
		return nil, inventory.UpstreamErr
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, inventory.AuthenticationErr
	}
	if response.StatusCode != http.StatusOK {
		return nil, inventory.UpstreamErr
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		if req.Context().Err() != nil {
			return nil, inventory.TimeoutErr
		}
		return nil, inventory.UpstreamErr
	}
	if int64(len(body)) > maxBytes {
		return nil, inventory.InvalidInventoryErr
	}
	return body, nil
}

type sourceDevice struct {
	IP           string   `json:"ip"`
	Name         string   `json:"name"`
	DNS          string   `json:"dns"`
	Serial       string   `json:"serial"`
	Vendor       string   `json:"vendor"`
	Model        string   `json:"model"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os_ver"`
	Location     string   `json:"location"`
	Contact      string   `json:"contact"`
	Description  string   `json:"description"`
	MACAddress   string   `json:"mac"`
	Tags         []string `json:"tags"`
	Creation     string   `json:"creation"`
	LastDiscover string   `json:"last_discover"`
}

func decodeInventory(body []byte) ([]inventory.SourceFacts, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '[' {
		return nil, inventory.InvalidInventoryErr
	}
	var source []*sourceDevice
	if json.Unmarshal(body, &source) != nil {
		return nil, inventory.InvalidInventoryErr
	}
	rows := make([]inventory.SourceFacts, 0, len(source))
	seen := make(map[string]bool, len(source))
	for _, d := range source {
		if d == nil {
			return nil, inventory.InvalidInventoryErr
		}
		address, err := netip.ParseAddr(strings.TrimSpace(d.IP))
		if err != nil || address.Zone() != "" {
			return nil, inventory.InvalidInventoryErr
		}
		ip := address.Unmap().String()
		if seen[ip] {
			return nil, inventory.InvalidInventoryErr
		}
		seen[ip] = true
		tags := make([]string, 0, len(d.Tags))
		tagSet := make(map[string]bool)
		for _, tag := range d.Tags {
			if tag = strings.TrimSpace(tag); tag != "" && !tagSet[tag] {
				tags = append(tags, tag)
				tagSet[tag] = true
			}
		}
		rows = append(rows, inventory.SourceFacts{IP: ip, Name: strings.TrimSpace(d.Name), Hostname: strings.TrimSpace(d.DNS), Serial: strings.TrimSpace(d.Serial), Vendor: strings.TrimSpace(d.Vendor), HardwareModel: strings.TrimSpace(d.Model), OS: strings.TrimSpace(d.OS), OSVersion: strings.TrimSpace(d.OSVersion), Location: strings.TrimSpace(d.Location), Contact: strings.TrimSpace(d.Contact), Description: strings.TrimSpace(d.Description), MACAddress: strings.TrimSpace(d.MACAddress), SourceTags: tags, SourceCreatedAt: sourceTime(d.Creation), LastDiscoveredAt: sourceTime(d.LastDiscover)})
	}
	return rows, nil
}
func sourceTime(value string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999-0700", "2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05.999999999 -0700"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil && !parsed.IsZero() {
			return parsed.UTC().Format(time.RFC3339Nano)
		}
	}
	// PostgreSQL timestamps without a zone are source wall-clock values.
	// Preserve that missing-zone evidence instead of assigning the server zone.
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02 15:04"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil && !parsed.IsZero() {
			return parsed.Format("2006-01-02T15:04:05.999999999")
		}
	}
	return ""
}
