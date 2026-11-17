package zabbix

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

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
//
// Host and item lookup, history and trend reads with their truncation, and
// identifier validation are delegated to go-zabbix's Monitoring service. This
// type keeps what is the module's own: configuration, the HTTPS policy, the
// request budget and the mapping of SDK errors onto inventory's errors.
type Client struct {
	config  Config
	client  *zbx.Client
	initErr error
}

var _ zbx.TrafficSource = (*Client)(nil)

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

// queryZabbix runs one SDK call under the 20-second upstream budget and maps
// its failure onto inventory's errors: an invalid query is the caller's input,
// a missing or mismatched host/item is a broken circuit binding, and anything
// else is the upstream being unavailable.
func queryZabbix[T any](ctx context.Context, p *Client, query func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, inventory.InvalidErr
	}
	if !p.Configured() {
		return zero, inventory.ZabbixNotConfiguredErr
	}
	if p.initErr != nil || p.client == nil {
		return zero, inventory.ZabbixUpstreamErr
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := query(ctx)
	if err == nil {
		return result, nil
	}
	switch {
	case ctx.Err() != nil:
		return result, inventory.ZabbixTimeoutErr
	case stderrors.Is(err, zbx.ErrMonitoringInput):
		return result, inventory.InvalidErr
	case stderrors.Is(err, zbx.ErrMonitoringBinding):
		return result, inventory.CircuitBindingErr
	default:
		return result, inventory.ZabbixUpstreamErr
	}
}

// Hosts provides the hosts operation within the caller's authorized network scope.
func (p *Client) Hosts(ctx context.Context, keyword string) (zbx.MonitoredHostList, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zbx.MonitoredHostList, error) {
		return p.client.Monitoring().Hosts(ctx, keyword)
	})
}

// Items provides the items operation within the caller's authorized network scope.
func (p *Client) Items(ctx context.Context, hostID, keyword string) (zbx.MonitoredItemList, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zbx.MonitoredItemList, error) {
		return p.client.Monitoring().Items(ctx, hostID, keyword)
	})
}

// Host provides the host operation within the caller's authorized network scope.
func (p *Client) Host(ctx context.Context, id string) (zbx.MonitoredHost, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zbx.MonitoredHost, error) {
		return p.client.Monitoring().Host(ctx, id)
	})
}

// Item provides the item operation within the caller's authorized network scope.
func (p *Client) Item(ctx context.Context, id string) (zbx.MonitoredItem, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zbx.MonitoredItem, error) {
		return p.client.Monitoring().Item(ctx, id)
	})
}

type metricResult[T any] struct {
	rows    []T
	partial bool
}

// History provides the history operation within the caller's authorized network scope.
func (p *Client) History(ctx context.Context, id, valueType string, from, till int64) ([]zbx.MetricSample, bool, error) {
	result, err := queryZabbix(ctx, p, func(ctx context.Context) (metricResult[zbx.MetricSample], error) {
		rows, partial, err := p.client.Monitoring().History(ctx, id, valueType, from, till)
		return metricResult[zbx.MetricSample]{rows: rows, partial: partial}, err
	})
	return result.rows, result.partial, err
}

// Trends provides the trends operation within the caller's authorized network scope.
func (p *Client) Trends(ctx context.Context, id string, from, till int64) ([]zbx.MetricTrend, bool, error) {
	result, err := queryZabbix(ctx, p, func(ctx context.Context) (metricResult[zbx.MetricTrend], error) {
		rows, partial, err := p.client.Monitoring().Trends(ctx, id, from, till)
		return metricResult[zbx.MetricTrend]{rows: rows, partial: partial}, err
	})
	return result.rows, result.partial, err
}
