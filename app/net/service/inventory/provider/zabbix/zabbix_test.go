package zabbix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	zbx "github.com/gatework/go-zabbix"
	"github.com/gatework/motor/app/net/models/inventory"
)

type zabbixRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

func zabbixFixture(t *testing.T, response map[string]any) (*Client, <-chan zabbixRequest) {
	t.Helper()
	requests := make(chan zabbixRequest, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("SDK request did not preserve POST/token authentication")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request zabbixRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.JSONRPC != "2.0" {
			t.Errorf("JSON-RPC version = %q", request.JSONRPC)
		}
		select {
		case requests <- request:
		default:
			t.Error("unexpected extra SDK request")
		}
		body := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		maps.Copy(body, response)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	httpClient := server.Client()
	httpClient.Timeout = 20 * time.Second
	client, err := zbx.New(server.URL,
		zbx.WithHTTPClient(httpClient),
		zbx.WithAPIToken("fixture-secret"),
		zbx.WithMaxResponseBytes(4<<20),
	)
	if err != nil {
		t.Fatal(err)
	}
	p := New(Config{URL: server.URL, Token: "fixture-secret"})
	p.client = client
	return p, requests
}

func assertZabbixRequest(t *testing.T, requests <-chan zabbixRequest, method, params string) {
	t.Helper()
	var want map[string]any
	if err := json.Unmarshal([]byte(params), &want); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if request.Method != method || !reflect.DeepEqual(request.Params, want) {
			t.Fatalf("encoded query = %s %#v; want %s %#v", request.Method, request.Params, method, want)
		}
	default:
		t.Fatal("SDK did not send a request")
	}
}

func TestZabbixHostAndItemLookupsAreBoundedAndRejectWrongOwnership(t *testing.T) {
	hostRows := make([]map[string]string, 51)
	for i := range hostRows {
		hostRows[i] = map[string]string{"hostid": fmt.Sprint(i + 1), "name": "Router", "host": "router"}
	}
	p, requests := zabbixFixture(t, map[string]any{"result": hostRows})
	hosts, err := p.Hosts(context.Background(), " router ")
	if err != nil || len(hosts.Items) != 50 || !hosts.HasMore || hosts.Items[49].ID != "50" {
		t.Fatalf("host bound: %+v %v", hosts, err)
	}
	assertZabbixRequest(t, requests, "host.get", `{
		"output":["hostid","name","host"],"search":{"name":"router","host":"router"},
		"searchByAny":true,"sortfield":["name"],"limit":51
	}`)
	p, requests = zabbixFixture(t, map[string]any{"result": []map[string]string{
		{"itemid": "1", "hostid": "1", "name": "Receive", "key_": "net.if.in", "units": "bps", "value_type": "3", "status": "0"},
		{"itemid": "2", "hostid": "2", "name": "Other host", "units": "bps", "value_type": "3", "status": "0"},
		{"itemid": "3", "hostid": "1", "units": "B", "value_type": "3", "status": "0"},
		{"itemid": "4", "hostid": "1", "units": "bps", "value_type": "1", "status": "0"},
		{"itemid": "5", "hostid": "1", "units": "Bps", "value_type": "0", "status": "1"},
	}})
	items, err := p.Items(context.Background(), "1", "")
	if err != nil || len(items.Items) != 1 || items.Items[0].ID != "1" || items.HasMore {
		t.Fatalf("unsafe items: %+v %v", items, err)
	}
	assertZabbixRequest(t, requests, "item.get", `{
		"output":["itemid","hostid","name","key_","units","value_type","status","state","lastvalue","lastclock"],
		"hostids":["1"],"filter":{"value_type":["0","3"],"status":"0","units":["bps","Bps"]},
		"sortfield":["name"],"limit":201
	}`)
	for _, keyword := range []string{"", strings.Repeat("a", 129), "a\x00b"} {
		if _, err = p.Hosts(context.Background(), keyword); !errors.Is(err, inventory.InvalidErr) {
			t.Fatalf("bad host search: %v", err)
		}
	}
}

func TestZabbixItemSearchLimitPrecedesBusinessFiltering(t *testing.T) {
	rows := make([]map[string]string, 201)
	for i := range rows {
		rows[i] = map[string]string{
			"itemid": fmt.Sprint(i + 1), "hostid": "2", "name": "Receive", "key_": "net.if.in",
			"units": "Bps", "value_type": "0", "status": "0",
		}
	}
	rows[200]["hostid"] = "1"
	p, requests := zabbixFixture(t, map[string]any{"result": rows})
	items, err := p.Items(context.Background(), "1", " Receive ")
	if err != nil || len(items.Items) != 0 || !items.HasMore {
		t.Fatalf("item bound must precede filtering: %+v %v", items, err)
	}
	assertZabbixRequest(t, requests, "item.get", `{
		"output":["itemid","hostid","name","key_","units","value_type","status","state","lastvalue","lastclock"],
		"hostids":["1"],"filter":{"value_type":["0","3"],"status":"0","units":["bps","Bps"]},
		"search":{"name":"Receive","key_":"Receive"},"searchByAny":true,"sortfield":["name"],"limit":201
	}`)
}

func TestZabbixHostAndItemBindings(t *testing.T) {
	p, requests := zabbixFixture(t, map[string]any{"result": []map[string]string{
		{"hostid": "1", "name": "Router", "host": "router"},
	}})
	host, err := p.Host(context.Background(), "1")
	if err != nil || host != (Host{ID: "1", Name: "Router", Host: "router"}) {
		t.Fatalf("host = %+v, %v", host, err)
	}
	assertZabbixRequest(t, requests, "host.get", `{
		"output":["hostid","name","host"],"hostids":["1"],"limit":2
	}`)
	p, requests = zabbixFixture(t, map[string]any{"result": []map[string]string{
		{
			"itemid": "10", "hostid": "1", "name": "Receive", "key_": "net.if.in", "units": "Bps",
			"value_type": "0", "status": "0", "state": "1", "lastvalue": "12.5", "lastclock": "1234",
		},
	}})
	item, err := p.Item(context.Background(), "10")
	want := Item{
		ID: "10", HostID: "1", Name: "Receive", Key: "net.if.in", Units: "Bps",
		ValueType: "0", Status: "0", State: "1", LastValue: "12.5", LastClock: 1234,
	}
	if err != nil || item != want || !item.IsRateFor("1") {
		t.Fatalf("item = %+v, %v; want %+v", item, err, want)
	}
	assertZabbixRequest(t, requests, "item.get", `{
		"output":["itemid","hostid","name","key_","units","value_type","status","state","lastvalue","lastclock"],
		"itemids":["10"],"limit":2
	}`)
	for _, tc := range []struct {
		name   string
		result string
	}{
		{name: "missing", result: `[]`},
		{name: "wrong identity", result: `[{"hostid":"2","itemid":"2"}]`},
		{name: "multiple", result: `[{"hostid":"1","itemid":"1"},{"hostid":"1","itemid":"1"}]`},
		{name: "invalid text", result: `[{"hostid":"1","itemid":"1","name":"bad\u0000name"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, requests := zabbixFixture(t, map[string]any{"result": json.RawMessage(tc.result)})
			if _, err := p.Host(context.Background(), "1"); !errors.Is(err, inventory.CircuitBindingErr) {
				t.Fatalf("host binding error = %v", err)
			}
			<-requests
			if _, err := p.Item(context.Background(), "1"); !errors.Is(err, inventory.CircuitBindingErr) {
				t.Fatalf("item binding error = %v", err)
			}
		})
	}
}

func TestZabbixItemsMalformedList(t *testing.T) {
	p, _ := zabbixFixture(t, map[string]any{"result": json.RawMessage(`[null]`)})
	items, err := p.Items(context.Background(), "1", "")
	if !errors.Is(err, inventory.ZabbixUpstreamErr) || len(items.Items) != 0 || items.HasMore {
		t.Fatalf("malformed list = %+v, %v; want upstream error", items, err)
	}
}

func TestZabbixHistoryCapValueTypesAndTrendQuery(t *testing.T) {
	for _, valueType := range []string{"0", "3"} {
		t.Run(valueType, func(t *testing.T) {
			result := make([]map[string]string, 12002)
			for i := range result {
				result[i] = map[string]string{"clock": fmt.Sprint(20000 - i), "value": "1.25"}
			}
			result[12000]["value"] = "900"
			p, requests := zabbixFixture(t, map[string]any{"result": result})
			rows, partial, err := p.History(context.Background(), "10", valueType, 0, 20000)
			if err != nil || len(rows) != 12001 || !partial || rows[12000].Value != "900" {
				t.Fatalf("history cap: %d %v %v", len(rows), partial, err)
			}
			assertZabbixRequest(t, requests, "history.get", fmt.Sprintf(`{
				"output":["clock","value"],"itemids":["10"],"history":%s,"time_from":0,"time_till":20000,
				"sortfield":["clock"],"sortorder":["DESC"],"limit":12001
			}`, valueType))
		})
	}
	p, requests := zabbixFixture(t, map[string]any{"result": []map[string]string{
		{"clock": "1000", "num": "3", "value_avg": "12.5", "value_max": "900"},
		{"clock": "0", "num": "1", "value_avg": "2", "value_max": "3"},
	}})
	rows, partial, err := p.Trends(context.Background(), "10", 0, 2000)
	if err != nil || partial || len(rows) != 2 || rows[0] != (Trend{Clock: 1000, Count: 3, Avg: "12.5", Max: "900"}) || rows[1].Clock != 0 {
		t.Fatalf("trend observations changed: %+v %v %v", rows, partial, err)
	}
	assertZabbixRequest(t, requests, "trend.get", `{
		"output":["clock","num","value_avg","value_max"],"itemids":["10"],"time_from":0,"time_till":2000,"limit":201
	}`)
}

func TestZabbixTrendProbeRemainsAvailableForPeak(t *testing.T) {
	result := make([]map[string]string, 202)
	for i := range result {
		result[i] = map[string]string{"clock": fmt.Sprint(i * 3600), "num": "1", "value_avg": "1", "value_max": "1"}
	}
	result[200]["value_max"] = "900"
	p, _ := zabbixFixture(t, map[string]any{"result": result})
	rows, partial, err := p.Trends(context.Background(), "10", 0, 200*3600)
	if err != nil || !partial || len(rows) != 201 || rows[200].Max != "900" {
		t.Fatalf("trend probe lost: length=%d partial=%v error=%v", len(rows), partial, err)
	}
}

func TestZabbixEmptySeriesPreserveZeroTimeBounds(t *testing.T) {
	p, requests := zabbixFixture(t, map[string]any{"result": json.RawMessage(`[]`)})
	samples, partial, err := p.History(context.Background(), "10", "0", 0, 0)
	if err != nil || partial || samples == nil || len(samples) != 0 {
		t.Fatalf("empty history = %+v, %v, %v", samples, partial, err)
	}
	assertZabbixRequest(t, requests, "history.get", `{
		"output":["clock","value"],"itemids":["10"],"history":0,"time_from":0,"time_till":0,
		"sortfield":["clock"],"sortorder":["DESC"],"limit":12001
	}`)
	trends, partial, err := p.Trends(context.Background(), "10", 0, 0)
	if err != nil || partial || trends == nil || len(trends) != 0 {
		t.Fatalf("empty trends = %+v, %v, %v", trends, partial, err)
	}
	assertZabbixRequest(t, requests, "trend.get", `{
		"output":["clock","num","value_avg","value_max"],"itemids":["10"],"time_from":0,"time_till":0,"limit":201
	}`)
}

func TestZabbixSampleTimesAndCountsRejectMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result string
		trend  bool
	}{
		{name: "history invalid clock", result: `[{"clock":"invalid","value":"1"}]`},
		{name: "history negative clock", result: `[{"clock":"-1","value":"1"}]`},
		{name: "trend negative clock", result: `[{"clock":"-1","num":"1"}]`, trend: true},
		{name: "trend invalid count", result: `[{"clock":"0","num":"invalid"}]`, trend: true},
		{name: "trend negative count", result: `[{"clock":"0","num":"-1"}]`, trend: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := zabbixFixture(t, map[string]any{"result": json.RawMessage(tc.result)})
			var err error
			if tc.trend {
				_, _, err = p.Trends(context.Background(), "10", 0, 1000)
			} else {
				_, _, err = p.History(context.Background(), "10", "0", 0, 1000)
			}
			if !errors.Is(err, inventory.ZabbixUpstreamErr) {
				t.Fatalf("malformed observation error = %v", err)
			}
		})
	}
}

func TestZabbixConfigurationContextAndSafeError(t *testing.T) {
	invalidToken := New(Config{URL: "https://example.invalid/api", Token: "fixture\nsecret"})
	_, initErr := invalidToken.Host(context.Background(), "1")
	if !errors.Is(initErr, inventory.ZabbixUpstreamErr) || strings.Contains(initErr.Error(), "secret") {
		t.Fatalf("SDK initialization failure must be reported safely: %v", initErr)
	}
	for _, url := range []string{
		"http://example.invalid/api", "https://user:password@example.invalid/api",
		"https://example.invalid/api?token=x", "https://example.invalid/api?", "https://example.invalid/api#secret",
	} {
		p := New(Config{URL: url, Token: "x"})
		if p.Configured() {
			t.Fatalf("unsafe config accepted: %s", url)
		}
		if _, err := p.Host(context.Background(), "1"); !errors.Is(err, inventory.ZabbixNotConfiguredErr) {
			t.Fatalf("unconfigured error = %v", err)
		}
	}
	p, _ := zabbixFixture(t, map[string]any{"error": map[string]any{
		"code": -32602, "message": "https://private.invalid token=fixture-secret", "data": "fixture-secret",
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Host(ctx, "1")
	if !errors.Is(err, inventory.ZabbixTimeoutErr) || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("unsafe cancelled error: %v", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = p.Host(ctx, "1")
	if !errors.Is(err, inventory.ZabbixTimeoutErr) {
		t.Fatalf("caller deadline dropped: %v", err)
	}
	_, err = p.Host(context.Background(), "1")
	if !errors.Is(err, inventory.ZabbixUpstreamErr) || strings.Contains(err.Error(), "private.invalid") || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}
