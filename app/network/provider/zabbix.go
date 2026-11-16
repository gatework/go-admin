package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gatework/go-zabbix"

	"go-admin/app/network"
)

type ZabbixConfig struct{ URL, Token string }
type Zabbix struct {
	config  ZabbixConfig
	client  *zabbix.Client
	initErr error
}

// Requests own their SDK client/token but share the standard transport's
// connection pool and finite idle timeout. Repeated traffic refreshes must
// not leave a new, indefinitely idle SDK transport behind each time.
var zabbixHTTPTransport = http.DefaultTransport.(*http.Transport).Clone()

func ZabbixFromEnvironment() *Zabbix {
	return NewZabbix(ZabbixConfig{URL: os.Getenv("ZABBIX_API_URL"), Token: os.Getenv("ZABBIX_API_TOKEN")})
}
func NewZabbix(config ZabbixConfig) *Zabbix {
	config.URL = strings.TrimSpace(config.URL)
	p := &Zabbix{config: config}
	if p.Configured() {
		httpClient := &http.Client{Transport: zabbixHTTPTransport, Timeout: 20 * time.Second}
		client, err := zabbix.New(config.URL,
			zabbix.WithHTTPClient(httpClient),
			zabbix.WithAPIToken(config.Token),
			zabbix.WithMaxResponseBytes(4<<20),
		)
		if err != nil {
			p.initErr = err
			return p
		}
		p.client = client
	}
	return p
}
func (p *Zabbix) Configured() bool {
	u, err := url.Parse(p.config.URL)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && !strings.ContainsRune(p.config.URL, '#') && strings.TrimSpace(p.config.Token) != ""
}
func queryZabbix[T any](ctx context.Context, p *Zabbix, query func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, network.ErrInvalidInput
	}
	if !p.Configured() {
		return zero, network.ErrZabbixNotConfigured
	}
	if p.initErr != nil || p.client == nil {
		return zero, network.ErrZabbixUpstream
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := query(ctx)
	if err == nil {
		return result, nil
	}
	switch {
	case ctx.Err() != nil:
		return result, network.ErrZabbixTimeout
	case errors.Is(err, zabbix.ErrMonitoringInput):
		return result, network.ErrInvalidInput
	case errors.Is(err, zabbix.ErrMonitoringBinding):
		return result, network.ErrCircuitBinding
	default:
		return result, network.ErrZabbixUpstream
	}
}
func (p *Zabbix) Hosts(ctx context.Context, keyword string) (zabbix.MonitoredHostList, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zabbix.MonitoredHostList, error) {
		return p.client.Monitoring().Hosts(ctx, keyword)
	})
}
func (p *Zabbix) Items(ctx context.Context, hostID, keyword string) (zabbix.MonitoredItemList, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zabbix.MonitoredItemList, error) {
		return p.client.Monitoring().Items(ctx, hostID, keyword)
	})
}
func (p *Zabbix) Host(ctx context.Context, id string) (zabbix.MonitoredHost, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zabbix.MonitoredHost, error) { return p.client.Monitoring().Host(ctx, id) })
}
func (p *Zabbix) Item(ctx context.Context, id string) (zabbix.MonitoredItem, error) {
	return queryZabbix(ctx, p, func(ctx context.Context) (zabbix.MonitoredItem, error) { return p.client.Monitoring().Item(ctx, id) })
}

type metricResult[T any] struct {
	rows    []T
	partial bool
}

func (p *Zabbix) History(ctx context.Context, id, valueType string, from, till int64) ([]zabbix.MetricSample, bool, error) {
	result, err := queryZabbix(ctx, p, func(ctx context.Context) (metricResult[zabbix.MetricSample], error) {
		rows, partial, err := p.client.Monitoring().History(ctx, id, valueType, from, till)
		return metricResult[zabbix.MetricSample]{rows: rows, partial: partial}, err
	})
	return result.rows, result.partial, err
}
func (p *Zabbix) Trends(ctx context.Context, id string, from, till int64) ([]zabbix.MetricTrend, bool, error) {
	result, err := queryZabbix(ctx, p, func(ctx context.Context) (metricResult[zabbix.MetricTrend], error) {
		rows, partial, err := p.client.Monitoring().Trends(ctx, id, from, till)
		return metricResult[zabbix.MetricTrend]{rows: rows, partial: partial}, err
	})
	return result.rows, result.partial, err
}
