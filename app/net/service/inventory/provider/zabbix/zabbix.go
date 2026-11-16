package zabbix

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gatework/motor/common/secret"

	zbx "github.com/gatework/go-zabbix"
	"github.com/gatework/motor/app/net/models/inventory"
)

// Config describes zabbix config for network operations.
type Config struct {
	URL   string
	Token secret.Secret
}

// Client queries one Zabbix server with its API token.
type Client struct {
	config  Config
	client  *zbx.Client
	initErr error
}

// Requests own their SDK client/token but share the standard transport's
// connection pool and finite idle timeout. Repeated traffic refreshes must
// not leave a new, indefinitely idle SDK transport behind each time.
var zabbixHTTPTransport = cloneHTTPTransport()

func cloneHTTPTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}
	return &http.Transport{}
}

// FromEnvironment provides the zabbix from environment operation within the caller's authorized network scope.
func FromEnvironment() Config {
	return Config{URL: os.Getenv("ZABBIX_API_URL"), Token: secret.Secret(os.Getenv("ZABBIX_API_TOKEN"))}
}

// New provides the new zabbix operation within the caller's authorized network scope.
func New(config Config) *Client {
	config.URL = strings.TrimSpace(config.URL)
	p := &Client{config: config}
	if p.Configured() {
		httpClient := &http.Client{Transport: zabbixHTTPTransport, Timeout: 20 * time.Second}
		client, err := zbx.New(config.URL,
			zbx.WithHTTPClient(httpClient),
			zbx.WithAPIToken(config.Token.Reveal()),
			zbx.WithMaxResponseBytes(4<<20),
		)
		if err != nil {
			p.initErr = err
			return p
		}
		p.client = client
	}
	return p
}

// Configured provides the configured operation within the caller's authorized network scope.
func (p *Client) Configured() bool {
	u, err := url.Parse(p.config.URL)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && !strings.ContainsRune(p.config.URL, '#') && strings.TrimSpace(p.config.Token.Reveal()) != ""
}
func queryZabbix[T any](ctx context.Context, p *Client, query func(context.Context) ([]T, error)) ([]T, error) {
	if !p.Configured() {
		return nil, inventory.ZabbixNotConfiguredErr
	}
	if p.initErr != nil || p.client == nil {
		return nil, inventory.ZabbixUpstreamErr
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rows, err := query(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, inventory.ZabbixTimeoutErr
		}
		return nil, inventory.ZabbixUpstreamErr
	}
	return rows, nil
}

// Host describes zabbix host for network operations.
type Host struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}

// HostList describes zabbix host list for network operations.
type HostList struct {
	Items   []Host `json:"items"`
	HasMore bool   `json:"hasMore"`
}

// Item describes zabbix item for network operations.
type Item struct {
	ID        string `json:"id"`
	HostID    string `json:"hostId"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Units     string `json:"units"`
	ValueType string `json:"-"`
	Status    string `json:"-"`
	State     string `json:"-"`
	LastValue string `json:"-"`
	LastClock int64  `json:"-"`
}

// IsRateFor provides the is rate for operation within the caller's authorized network scope.
func (i Item) IsRateFor(hostID string) bool {
	return i.HostID == hostID && (i.ValueType == "0" || i.ValueType == "3") && i.Status == "0" && (i.Units == "bps" || i.Units == "Bps")
}

// ItemList describes zabbix item list for network operations.
type ItemList struct {
	Items   []Item `json:"items"`
	HasMore bool   `json:"hasMore"`
}

// Sample describes zabbix sample for network operations.
type Sample struct {
	Clock int64
	Value string
}

// Trend describes zabbix trend for network operations.
type Trend struct {
	Clock    int64
	Avg, Max string
	Count    int64
}

func zabbixItem(r zbx.Item) Item {
	clock, _ := strconv.ParseInt(r.LastClock, 10, 64)
	return Item{
		ID: r.ItemID, HostID: r.HostID, Name: r.Name, Key: r.Key, Units: r.Units,
		ValueType: r.ValueType, Status: r.Status, State: r.State, LastValue: r.LastValue, LastClock: clock,
	}
}

var zabbixIdentifier = regexp.MustCompile(`^[1-9][0-9]{0,31}$`)

func validZabbixText(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && !strings.ContainsRune(value, 0)
}

var zabbixItemOutput = []string{"itemid", "hostid", "name", "key_", "units", "value_type", "status", "state", "lastvalue", "lastclock"}

// Hosts provides the hosts operation within the caller's authorized network scope.
func (p *Client) Hosts(ctx context.Context, keyword string) (HostList, error) {
	result := HostList{Items: []Host{}}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || !validZabbixText(keyword, 128) {
		return result, inventory.InvalidErr
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.Host, error) {
		return p.client.Host().Get(ctx, zbx.HostGetParams{
			Output:      []string{"hostid", "name", "host"},
			Search:      map[string]any{"name": keyword, "host": keyword},
			SearchByAny: true,
			SortField:   []string{"name"},
			Limit:       51,
		})
	})
	if err != nil {
		return result, err
	}
	result.HasMore = len(rows) > 50
	if len(rows) > 50 {
		rows = rows[:50]
	}
	for _, r := range rows {
		if !zabbixIdentifier.MatchString(r.HostID) || !validZabbixText(r.Name, 512) || !validZabbixText(r.Host, 512) {
			return result, inventory.ZabbixUpstreamErr
		}
		result.Items = append(result.Items, Host{ID: r.HostID, Name: r.Name, Host: r.Host})
	}
	return result, nil
}

// Items provides the items operation within the caller's authorized network scope.
func (p *Client) Items(ctx context.Context, hostID, keyword string) (ItemList, error) {
	result := ItemList{Items: []Item{}}
	keyword = strings.TrimSpace(keyword)
	if !zabbixIdentifier.MatchString(hostID) || !validZabbixText(keyword, 128) {
		return result, inventory.InvalidErr
	}
	params := zbx.ItemGetParams{
		Output:    zabbixItemOutput,
		HostIDs:   []string{hostID},
		Filter:    map[string]any{"value_type": []string{"0", "3"}, "status": "0", "units": []string{"bps", "Bps"}},
		SortField: []string{"name"},
		Limit:     201,
	}
	if keyword != "" {
		params.Search = map[string]any{"name": keyword, "key_": keyword}
		params.SearchByAny = true
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.Item, error) {
		return p.client.Item().Get(ctx, params)
	})
	if err != nil {
		return result, err
	}
	result.HasMore = len(rows) > 200
	if len(rows) > 200 {
		rows = rows[:200]
	}
	for _, r := range rows {
		i := zabbixItem(r)
		if i.IsRateFor(hostID) && zabbixIdentifier.MatchString(i.ID) && validZabbixText(i.Name, 512) && validZabbixText(i.Key, 2048) {
			result.Items = append(result.Items, i)
		}
	}
	return result, nil
}

// Host provides the host operation within the caller's authorized network scope.
func (p *Client) Host(ctx context.Context, id string) (Host, error) {
	var result Host
	if !zabbixIdentifier.MatchString(id) {
		return result, inventory.CircuitBindingErr
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.Host, error) {
		return p.client.Host().Get(ctx, zbx.HostGetParams{
			Output: []string{"hostid", "name", "host"}, HostIDs: []string{id}, Limit: 2,
		})
	})
	if err != nil {
		return result, err
	}
	if len(rows) != 1 || rows[0].HostID != id || !validZabbixText(rows[0].Name, 512) || !validZabbixText(rows[0].Host, 512) {
		return result, inventory.CircuitBindingErr
	}
	return Host{ID: id, Name: rows[0].Name, Host: rows[0].Host}, nil
}

// Item provides the item operation within the caller's authorized network scope.
func (p *Client) Item(ctx context.Context, id string) (Item, error) {
	var result Item
	if !zabbixIdentifier.MatchString(id) {
		return result, inventory.CircuitBindingErr
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.Item, error) {
		return p.client.Item().Get(ctx, zbx.ItemGetParams{
			Output: zabbixItemOutput, ItemIDs: []string{id}, Limit: 2,
		})
	})
	if err != nil {
		return result, err
	}
	if len(rows) != 1 || rows[0].ItemID != id || !validZabbixText(rows[0].Name, 512) {
		return result, inventory.CircuitBindingErr
	}
	return zabbixItem(rows[0]), nil
}

// History provides the history operation within the caller's authorized network scope.
func (p *Client) History(ctx context.Context, id, valueType string, from, till int64) ([]Sample, bool, error) {
	if !zabbixIdentifier.MatchString(id) || (valueType != "0" && valueType != "3") {
		return nil, false, inventory.CircuitBindingErr
	}
	history := 0
	if valueType == "3" {
		history = 3
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.HistoryValue, error) {
		return p.client.History().Get(ctx, zbx.HistoryGetParams{
			Output: []string{"clock", "value"}, ItemIDs: []string{id}, History: &history,
			TimeFrom: &from, TimeTill: &till,
			SortField: []string{"clock"}, SortOrder: []string{"DESC"}, Limit: 12001,
		})
	})
	if err != nil {
		return nil, false, err
	}
	partial := len(rows) > 12000
	// The probe is an observed sample too: keep it when calculating the
	// reported peak, while explicitly marking the range as partial.
	if len(rows) > 12001 {
		rows = rows[:12001]
	}
	result := make([]Sample, 0, len(rows))
	for _, r := range rows {
		clock, err := strconv.ParseInt(r.Clock, 10, 64)
		if err != nil || clock < 0 {
			return nil, partial, inventory.ZabbixUpstreamErr
		}
		result = append(result, Sample{Clock: clock, Value: r.Value})
	}
	return result, partial, nil
}

// Trends provides the trends operation within the caller's authorized network scope.
func (p *Client) Trends(ctx context.Context, id string, from, till int64) ([]Trend, bool, error) {
	if !zabbixIdentifier.MatchString(id) {
		return nil, false, inventory.CircuitBindingErr
	}
	// Zabbix 7.4 does not sort trend.get. The circuit view queries one item over
	// seven days; the extra row detects an incomplete response if that bound is
	// exceeded. The service orders the observed points before presentation.
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zbx.Trend, error) {
		return p.client.Trend().Get(ctx, zbx.TrendGetParams{
			Output: []string{"clock", "num", "value_avg", "value_max"}, ItemIDs: []string{id},
			TimeFrom: &from, TimeTill: &till, Limit: 201,
		})
	})
	if err != nil {
		return nil, false, err
	}
	partial := len(rows) > 200
	// Keep the observed probe when calculating peaks, just as history does.
	if len(rows) > 201 {
		rows = rows[:201]
	}
	result := make([]Trend, 0, len(rows))
	for _, r := range rows {
		clock, err := strconv.ParseInt(r.Clock, 10, 64)
		count, e := strconv.ParseInt(r.Num, 10, 64)
		if err != nil || e != nil || clock < 0 || count < 0 {
			return nil, partial, inventory.ZabbixUpstreamErr
		}
		result = append(result, Trend{Clock: clock, Avg: r.ValueAvg, Max: r.ValueMax, Count: count})
	}
	return result, partial, nil
}
