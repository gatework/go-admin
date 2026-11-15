package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider/zabbix"
)

func trafficCircuit(t *testing.T) (*Circuit, *circuitSource, inventory.Circuit, time.Time) {
	t.Helper()
	s := newCircuitService(t)
	source := knownCircuitSource()
	s.Source = source
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	draft := circuitDraft("TRAFFIC-1")
	draft.AEndpoint.Interface = "eth0"
	draft.AEndpoint.Monitoring = inventory.CircuitBinding{HostID: "1", InItemID: "10", OutItemID: "11"}
	row, err := s.Create(context.Background(), draft, 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	return s, source, row, now
}
func TestCircuitTrafficPreservesPeakUnitsMissingAndStaleCurrent(t *testing.T) {
	s, source, row, now := trafficCircuit(t)
	in := source.items["10"]
	in.LastValue = "7.5"
	in.LastClock = now.Unix() - 30
	source.items["10"] = in
	out := source.items["11"]
	out.LastValue = "2"
	out.LastClock = now.Unix() - 301
	source.items["11"] = out
	source.history["10"] = []zabbix.Sample{{Clock: now.Unix() - 10, Value: "10"}, {Clock: now.Unix() - 11, Value: "1000"}, {Clock: now.Unix() - 12, Value: "0"}}
	source.history["11"] = []zabbix.Sample{{Clock: now.Unix() - 10, Value: "1.25"}}
	result, err := s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || len(result.Series) != 2 {
		t.Fatalf("traffic: %+v %v", result, err)
	}
	a, b := result.Series[0], result.Series[1]
	if a.State != "ok" || a.CurrentBps == nil || *a.CurrentBps != 7.5 || a.PeakBps == nil || *a.PeakBps != 1000 || a.Mode != "history" {
		t.Fatalf("history lost current/peak: %+v", a)
	}
	if b.State != "stale" || b.CurrentBps != nil || b.LastSampleAt == nil || *b.LastSampleAt != (now.Unix()-301)*1000 || b.PeakBps == nil || *b.PeakBps != 10 {
		t.Fatalf("stale/byte conversion lost: %+v", b)
	}
	if result.TimeFrom != now.Add(-time.Hour).UnixMilli() || result.TimeTill != now.UnixMilli() {
		t.Fatalf("wrong ms range: %+v", result)
	}
	source.history["10"] = nil
	in.LastClock = 0
	in.LastValue = ""
	source.items["10"] = in
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	a = result.Series[0]
	if err != nil || a.State != "empty" || a.CurrentBps != nil || a.PeakBps != nil || len(a.Points) != 0 {
		t.Fatalf("empty invented zero: %+v %v", a, err)
	}
}
func TestCircuitTrafficTrendsUseMaxAndIndependentLastValue(t *testing.T) {
	s, source, row, now := trafficCircuit(t)
	item := source.items["10"]
	item.LastClock = now.Unix() - 5
	item.LastValue = "42"
	source.items["10"] = item
	source.trends["10"] = []zabbix.Trend{{Clock: now.Add(-time.Hour).Unix(), Avg: "20", Max: "900", Count: 60}, {Clock: now.Add(-3 * time.Hour).Unix(), Avg: "10", Max: "500", Count: 60}}
	result, err := s.Traffic(context.Background(), row.ID, "7d", fullScope())
	if err != nil {
		t.Fatal(err)
	}
	a := result.Series[0]
	if a.Mode != "trend" || a.PeakBps == nil || *a.PeakBps != 900 || a.CurrentBps == nil || *a.CurrentBps != 42 {
		t.Fatalf("trend statistics mixed: %+v", a)
	}
	gap := false
	for _, p := range a.Points {
		if p[0] == float64(now.Add(-2*time.Hour).UnixMilli()) && p[1] == nil {
			gap = true
		}
	}
	if !gap {
		t.Fatalf("missing trend hour connected: %+v", a.Points)
	}
	source.trends["10"] = nil
	result, err = s.Traffic(context.Background(), row.ID, "7d", fullScope())
	if err != nil || result.Series[0].State != "empty" || len(result.Series[0].Points) != 0 {
		t.Fatalf("missing trend fabricated: %+v %v", result, err)
	}
}
func TestCircuitTrafficRevalidatesBindingsAndLimitsSeriesFailures(t *testing.T) {
	s, source, row, _ := trafficCircuit(t)
	item := source.items["10"]
	item.HostID = "2"
	source.items["10"] = item
	result, err := s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || result.Series[0].State != "unavailable" || result.Series[1].State != "empty" {
		t.Fatalf("ownership not revalidated: %+v %v", result, err)
	}
	item = source.items["11"]
	item.Units = ""
	source.items["11"] = item
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || result.Series[1].State != "unavailable" {
		t.Fatalf("unknown units claimed rate: %+v %v", result, err)
	}
	source.err = errors.New("private-url token=secret")
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || result.Series[0].State != "error" {
		t.Fatalf("series error escalated: %+v %v", result, err)
	}
	source.configured = false
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || result.Configured || len(result.Series) != 0 {
		t.Fatalf("missing config: %+v %v", result, err)
	}
	if _, err = s.Traffic(context.Background(), row.ID, "bogus", fullScope()); !errors.Is(err, inventory.InvalidErr) {
		t.Fatalf("bad period: %v", err)
	}
	if _, err = s.Traffic(context.Background(), row.ID, "1h", nil); !errors.Is(err, inventory.CircuitNotFoundErr) {
		t.Fatalf("nil scope traffic leaked: %v", err)
	}
}
func TestCircuitHistoryBucketsPreserveGapsAndPartialEvidence(t *testing.T) {
	s, source, row, now := trafficCircuit(t)
	for i := range 12000 {
		source.history["10"] = append(source.history["10"], zabbix.Sample{Clock: now.Unix() - int64(i), Value: "1"})
	}
	source.history["10"] = append(source.history["10"], zabbix.Sample{Clock: now.Unix() - 12000, Value: "99"})
	source.partial = true
	result, err := s.Traffic(context.Background(), row.ID, "6h", fullScope())
	a := result.Series[0]
	if err != nil || !a.Partial || len(a.Points) > 720 || a.PeakBps == nil || *a.PeakBps != 99 {
		t.Fatalf("bounded partial evidence: %d %+v %v", len(a.Points), a, err)
	}
	source.partial = false
	source.history["10"] = nil
	for _, seconds := range []int64{0, 180, 360, 540, 2400, 2580, 2760} {
		source.history["10"] = append(source.history["10"], zabbix.Sample{Clock: now.Unix() - seconds, Value: "1"})
	}
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil {
		t.Fatal(err)
	}
	nonNull, nulls := 0, 0
	for _, point := range result.Series[0].Points {
		if point[1] == nil {
			nulls++
		} else {
			nonNull++
		}
	}
	if nonNull != 7 || nulls != 2 {
		t.Fatalf("normal cadence fragmented or long gap joined: %+v", result.Series[0].Points)
	}
}

func TestCircuitTrafficZeroInvalidAndLargeRatesRemainJSONSafe(t *testing.T) {
	s, source, row, now := trafficCircuit(t)
	item := source.items["10"]
	item.LastClock, item.LastValue = now.Unix(), "0"
	source.items["10"] = item
	source.history["10"] = []zabbix.Sample{{Clock: now.Unix(), Value: "0"}}
	result, err := s.Traffic(context.Background(), row.ID, "1h", fullScope())
	a := result.Series[0]
	if err != nil || a.CurrentBps == nil || *a.CurrentBps != 0 || a.PeakBps == nil || *a.PeakBps != 0 || a.State != "ok" {
		t.Fatalf("observed zero lost: %+v %v", a, err)
	}
	item.LastValue = "NaN"
	source.items["10"] = item
	source.history["10"] = []zabbix.Sample{{Clock: now.Unix(), Value: "1e308"}, {Clock: now.Unix() - 1, Value: "1e308"}}
	result, err = s.Traffic(context.Background(), row.ID, "1h", fullScope())
	if err != nil || result.Series[0].CurrentBps != nil {
		t.Fatalf("invalid current invented data: %+v %v", result, err)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("bucket arithmetic broke JSON: %v", err)
	}
}
