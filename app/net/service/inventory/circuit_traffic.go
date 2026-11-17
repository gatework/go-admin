package inventory

import (
	"context"
	"errors"
	"time"

	zbx "github.com/gatework/go-zabbix"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
)

// Traffic provides the traffic operation within the caller's authorized network scope.
func (s *Circuit) Traffic(ctx context.Context, id int, period string, p *actions.DataPermission) (dto.CircuitTraffic, error) {
	result := dto.CircuitTraffic{Period: period, Series: []dto.CircuitTrafficSeries{}}
	duration, ok := map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}[period]
	if !ok {
		return result, inventory.InvalidErr
	}
	row, err := s.Get(ctx, id, p)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}
	now = now.Truncate(time.Second)
	from, till := now.Add(-duration).Unix(), now.Unix()
	result.TimeFrom = from * 1000
	result.TimeTill = till * 1000
	source := s.source()
	result.Configured = source.Configured()
	if !result.Configured {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, side := range []struct {
		name     string
		endpoint inventory.CircuitEndpoint
	}{{"a", row.AEndpoint}, {"z", row.ZEndpoint}} {
		b := side.endpoint.Monitoring
		for _, direction := range []struct{ name, id, label string }{{"in", b.InItemID, b.InItemName}, {"out", b.OutItemID, b.OutItemName}} {
			if direction.id == "" {
				continue
			}
			series := dto.CircuitTrafficSeries{Side: side.name, Direction: direction.name, HostID: b.HostID, HostName: b.HostName, ItemID: direction.id, Name: direction.label, Interface: side.endpoint.Interface, State: "empty", Mode: "history", Points: []dto.TrafficPoint{}}
			if period == "7d" {
				series.Mode = "trend"
			}
			s.populateTrafficSeries(ctx, source, &series, from, till)
			result.Series = append(result.Series, series)
		}
	}
	return result, nil
}

// populateTrafficSeries reads one bound item through go-zabbix's ReadTraffic,
// which revalidates that the item still belongs to the host, reads history or
// hourly trends with their truncation, converts units to bits per second and
// marks missing evidence as nil rather than zero. This keeps the HTTP shape:
// milliseconds where the SDK speaks seconds.
func (s *Circuit) populateTrafficSeries(ctx context.Context, source CircuitSource, series *dto.CircuitTrafficSeries, from, till int64) {
	traffic, err := zbx.ReadTraffic(ctx, source, zbx.TrafficQuery{
		HostID: series.HostID, ItemID: series.ItemID,
		From: from, Till: till, UseTrends: series.Mode == "trend",
	})
	if traffic.Item.ID != "" {
		series.HostName, series.Name = traffic.Host.Name, traffic.Item.Name
		series.CurrentBps, series.PeakBps, series.Partial = traffic.CurrentBPS, traffic.PeakBPS, traffic.Partial
		if traffic.LastSampleAt != nil {
			value := *traffic.LastSampleAt * 1000
			series.LastSampleAt = &value
		}
		for _, point := range traffic.Points {
			p := dto.TrafficPoint{float64(point.Clock * 1000), nil}
			if point.BPS != nil {
				p[1] = *point.BPS
			}
			series.Points = append(series.Points, p)
		}
	}
	if err != nil {
		series.State = trafficErrorState(err)
		return
	}
	if traffic.SampleCount == 0 {
		series.State = "empty"
		return
	}
	series.State = "ok"
	if traffic.Stale {
		series.State = "stale"
	}
}

// trafficErrorState: a binding that no longer resolves is "unavailable", any
// other failure is "error". The provider maps the SDK's binding error onto
// CircuitBindingErr; a source that returns the SDK error directly counts too.
func trafficErrorState(err error) string {
	if errors.Is(err, inventory.CircuitBindingErr) || errors.Is(err, zbx.ErrMonitoringBinding) {
		return "unavailable"
	}
	return "error"
}
