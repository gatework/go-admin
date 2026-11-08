package provider

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
func queryZabbix[T any](ctx context.Context, p *Zabbix, query func(context.Context) ([]T, error)) ([]T, error) {
	if !p.Configured() {
		return nil, network.ErrZabbixNotConfigured
	}
	if p.initErr != nil || p.client == nil {
		return nil, network.ErrZabbixUpstream
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rows, err := query(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, network.ErrZabbixTimeout
		}
		return nil, network.ErrZabbixUpstream
	}
	return rows, nil
}

type ZabbixHost struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}
type ZabbixHostList struct {
	Items   []ZabbixHost `json:"items"`
	HasMore bool         `json:"hasMore"`
}
type ZabbixItem struct {
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

func (i ZabbixItem) IsRateFor(hostID string) bool {
	return i.HostID == hostID && (i.ValueType == "0" || i.ValueType == "3") && i.Status == "0" && (i.Units == "bps" || i.Units == "Bps")
}

type ZabbixItemList struct {
	Items   []ZabbixItem `json:"items"`
	HasMore bool         `json:"hasMore"`
}
type ZabbixSample struct {
	Clock int64
	Value string
}
type ZabbixTrend struct {
	Clock    int64
	Avg, Max string
	Count    int64
}

func zabbixItem(r zabbix.Item) ZabbixItem {
	clock, _ := strconv.ParseInt(r.LastClock, 10, 64)
	return ZabbixItem{
		ID: r.ItemID, HostID: r.HostID, Name: r.Name, Key: r.Key, Units: r.Units,
		ValueType: r.ValueType, Status: r.Status, State: r.State, LastValue: r.LastValue, LastClock: clock,
	}
}

var zabbixIdentifier = regexp.MustCompile(`^[1-9][0-9]{0,31}$`)

func validZabbixText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
}

var zabbixItemOutput = []string{"itemid", "hostid", "name", "key_", "units", "value_type", "status", "state", "lastvalue", "lastclock"}

func (p *Zabbix) Hosts(ctx context.Context, keyword string) (ZabbixHostList, error) {
	result := ZabbixHostList{Items: []ZabbixHost{}}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || !validZabbixText(keyword, 128) {
		return result, network.ErrInvalidInput
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.Host, error) {
		return p.client.Host().Get(ctx, zabbix.HostGetParams{
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
			return result, network.ErrZabbixUpstream
		}
		result.Items = append(result.Items, ZabbixHost{ID: r.HostID, Name: r.Name, Host: r.Host})
	}
	return result, nil
}
func (p *Zabbix) Items(ctx context.Context, hostID, keyword string) (ZabbixItemList, error) {
	result := ZabbixItemList{Items: []ZabbixItem{}}
	keyword = strings.TrimSpace(keyword)
	if !zabbixIdentifier.MatchString(hostID) || !validZabbixText(keyword, 128) {
		return result, network.ErrInvalidInput
	}
	params := zabbix.ItemGetParams{
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
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.Item, error) {
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
func (p *Zabbix) Host(ctx context.Context, id string) (ZabbixHost, error) {
	var result ZabbixHost
	if !zabbixIdentifier.MatchString(id) {
		return result, network.ErrCircuitBinding
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.Host, error) {
		return p.client.Host().Get(ctx, zabbix.HostGetParams{
			Output: []string{"hostid", "name", "host"}, HostIDs: []string{id}, Limit: 2,
		})
	})
	if err != nil {
		return result, err
	}
	if len(rows) != 1 || rows[0].HostID != id || !validZabbixText(rows[0].Name, 512) || !validZabbixText(rows[0].Host, 512) {
		return result, network.ErrCircuitBinding
	}
	return ZabbixHost{ID: id, Name: rows[0].Name, Host: rows[0].Host}, nil
}
func (p *Zabbix) Item(ctx context.Context, id string) (ZabbixItem, error) {
	var result ZabbixItem
	if !zabbixIdentifier.MatchString(id) {
		return result, network.ErrCircuitBinding
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.Item, error) {
		return p.client.Item().Get(ctx, zabbix.ItemGetParams{
			Output: zabbixItemOutput, ItemIDs: []string{id}, Limit: 2,
		})
	})
	if err != nil {
		return result, err
	}
	if len(rows) != 1 || rows[0].ItemID != id || !validZabbixText(rows[0].Name, 512) {
		return result, network.ErrCircuitBinding
	}
	return zabbixItem(rows[0]), nil
}
func (p *Zabbix) History(ctx context.Context, id, valueType string, from, till int64) ([]ZabbixSample, bool, error) {
	if !zabbixIdentifier.MatchString(id) || (valueType != "0" && valueType != "3") {
		return nil, false, network.ErrCircuitBinding
	}
	history := 0
	if valueType == "3" {
		history = 3
	}
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.HistoryValue, error) {
		return p.client.History().Get(ctx, zabbix.HistoryGetParams{
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
	result := make([]ZabbixSample, 0, len(rows))
	for _, r := range rows {
		clock, err := strconv.ParseInt(r.Clock, 10, 64)
		if err != nil || clock < 0 {
			return nil, partial, network.ErrZabbixUpstream
		}
		result = append(result, ZabbixSample{Clock: clock, Value: r.Value})
	}
	return result, partial, nil
}
func (p *Zabbix) Trends(ctx context.Context, id string, from, till int64) ([]ZabbixTrend, bool, error) {
	if !zabbixIdentifier.MatchString(id) {
		return nil, false, network.ErrCircuitBinding
	}
	// Zabbix 7.4 does not sort trend.get. The circuit view queries one item over
	// seven days; the extra row detects an incomplete response if that bound is
	// exceeded. The service orders the observed points before presentation.
	rows, err := queryZabbix(ctx, p, func(ctx context.Context) ([]zabbix.Trend, error) {
		return p.client.Trend().Get(ctx, zabbix.TrendGetParams{
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
	result := make([]ZabbixTrend, 0, len(rows))
	for _, r := range rows {
		clock, err := strconv.ParseInt(r.Clock, 10, 64)
		count, e := strconv.ParseInt(r.Num, 10, 64)
		if err != nil || e != nil || clock < 0 || count < 0 {
			return nil, partial, network.ErrZabbixUpstream
		}
		result = append(result, ZabbixTrend{Clock: clock, Avg: r.ValueAvg, Max: r.ValueMax, Count: count})
	}
	return result, partial, nil
}
