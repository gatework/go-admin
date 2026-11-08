package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/network"
	"go-admin/app/network/models"
	"go-admin/app/network/provider"
	"go-admin/app/network/service/dto"
)

func (s *Circuit) Traffic(ctx context.Context, id int, period string, p *actions.DataPermission) (dto.CircuitTraffic, error) {
	result := dto.CircuitTraffic{Period: period, Series: []dto.CircuitTrafficSeries{}}
	duration, ok := map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}[period]
	if !ok {
		return result, network.ErrInvalidInput
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
		endpoint models.CircuitEndpoint
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
func (s *Circuit) populateTrafficSeries(ctx context.Context, source CircuitSource, series *dto.CircuitTrafficSeries, from, till int64) {
	host, err := source.Host(ctx, series.HostID)
	if err != nil {
		series.State = trafficErrorState(err)
		return
	}
	if host.ID != series.HostID {
		series.State = "unavailable"
		return
	}
	item, err := source.Item(ctx, series.ItemID)
	if err != nil {
		series.State = trafficErrorState(err)
		return
	}
	if item.ID != series.ItemID || !item.IsRateFor(series.HostID) || item.State == "1" {
		series.State = "unavailable"
		return
	}
	series.HostName = host.Name
	series.Name = item.Name
	multiplier := 1.0
	if item.Units == "Bps" {
		multiplier = 8
	}
	if item.LastClock > 0 && item.LastClock <= till {
		last := item.LastClock * 1000
		series.LastSampleAt = &last
		if till-item.LastClock <= 300 {
			series.CurrentBps = rateValue(item.LastValue, multiplier)
		}
	}
	var count int
	if series.Mode == "trend" {
		var rows []provider.ZabbixTrend
		rows, series.Partial, err = source.Trends(ctx, item.ID, from, till)
		if err == nil {
			series.Points, series.PeakBps, count = trendPoints(rows, from, till, multiplier)
		}
	} else {
		var rows []provider.ZabbixSample
		rows, series.Partial, err = source.History(ctx, item.ID, item.ValueType, from, till)
		if err == nil {
			series.Points, series.PeakBps, count = historyPoints(rows, from, till, multiplier)
		}
	}
	if err != nil {
		series.State = trafficErrorState(err)
		return
	}
	if count == 0 {
		series.State = "empty"
		return
	}
	series.State = "ok"
	if item.LastClock > 0 && till-item.LastClock > 300 {
		series.State = "stale"
	}
}
func trafficErrorState(err error) string {
	if errors.Is(err, network.ErrCircuitBinding) {
		return "unavailable"
	}
	return "error"
}
func rateValue(value string, multiplier float64) *float64 {
	v, err := strconv.ParseFloat(value, 64)
	v *= multiplier
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	return &v
}
func recordPeak(peak **float64, value *float64) {
	if value != nil && (*peak == nil || **peak < *value) {
		copy := *value
		*peak = &copy
	}
}
func trafficPoint(clock int64, value *float64) dto.TrafficPoint {
	p := dto.TrafficPoint{float64(clock * 1000), nil}
	if value != nil {
		p[1] = *value
	}
	return p
}

// Buckets average only actual samples. Missing normal polling intervals are
// omitted; a gap larger than max(5 minutes, 3 x median observed interval) gets
// a null marker. Each marker consumes an otherwise empty bucket, so even with
// gaps the final chart contains at most 720 points.
func historyPoints(rows []provider.ZabbixSample, from, till int64, multiplier float64) ([]dto.TrafficPoint, *float64, int) {
	points := []dto.TrafficPoint{}
	var peak *float64
	valid := 0
	filtered := make([]provider.ZabbixSample, 0, len(rows))
	for _, row := range rows {
		if row.Clock >= from && row.Clock <= till {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		return points, nil, 0
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Clock < filtered[j].Clock })
	intervals := []int64{}
	for i := 1; i < len(filtered); i++ {
		if delta := filtered[i].Clock - filtered[i-1].Clock; delta > 0 {
			intervals = append(intervals, delta)
		}
	}
	gap := int64(300)
	if len(intervals) > 0 {
		sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
		gap = max(gap, 3*intervals[len(intervals)/2])
	}
	width := max(int64(1), (till-from+719)/720)
	type bucket struct {
		mean        float64
		count       int
		invalid     bool
		first, last int64
		seen        bool
	}
	buckets := make([]bucket, 720)
	for _, row := range filtered {
		index := min(int((row.Clock-from)/width), 719)
		b := &buckets[index]
		if !b.seen {
			b.first = row.Clock
			b.seen = true
		}
		b.last = row.Clock
		value := rateValue(row.Value, multiplier)
		if value == nil {
			b.invalid = true
			continue
		}
		b.count++
		b.mean += (*value - b.mean) / float64(b.count)
		valid++
		recordPeak(&peak, value)
	}
	lastClock, lastIndex := from, -1
	for index, b := range buckets {
		if !b.seen {
			continue
		}
		if b.first-lastClock > gap && index > lastIndex+1 {
			points = append(points, trafficPoint(from+int64(lastIndex+1)*width, nil))
		}
		var value *float64
		if b.count > 0 && !b.invalid {
			v := b.mean
			value = &v
		}
		points = append(points, trafficPoint(b.first, value))
		lastClock = b.last
		lastIndex = index
	}
	if till-lastClock > gap && lastIndex < 719 {
		points = append(points, trafficPoint(till, nil))
	}
	return points, peak, valid
}
func trendPoints(rows []provider.ZabbixTrend, from, till int64, multiplier float64) ([]dto.TrafficPoint, *float64, int) {
	points := []dto.TrafficPoint{}
	var peak *float64
	valid := 0
	values := map[int64]*float64{}
	for _, row := range rows {
		if row.Clock < from || row.Clock > till || row.Count <= 0 {
			continue
		}
		avg := rateValue(row.Avg, multiplier)
		maximum := rateValue(row.Max, multiplier)
		values[row.Clock] = avg
		if avg != nil {
			valid++
		}
		recordPeak(&peak, maximum)
	}
	if len(values) == 0 {
		return points, nil, 0
	}
	first := (from + 3599) / 3600 * 3600
	for clock := first; clock <= till; clock += 3600 {
		points = append(points, trafficPoint(clock, values[clock]))
	}
	return points, peak, valid
}
