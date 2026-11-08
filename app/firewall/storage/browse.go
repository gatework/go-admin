package storage

import (
	"context"
	"net/netip"
	"strconv"
	"strings"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"go-admin/app/firewall/models"
	"gorm.io/gorm"
)

type Filter struct {
	Keyword, Address, Protocol, Port, Zone, State string
	PageIndex, PageSize                           int
}
type Object struct {
	ID          int                 `json:"id"`
	Kind        string              `json:"kind"`
	NativeID    string              `json:"nativeId"`
	Name        string              `json:"name"`
	Scope       string              `json:"scope"`
	Position    int                 `json:"position"`
	State       string              `json:"state"`
	Summary     string              `json:"summary"`
	Record      any                 `json:"record"`
	Diagnostics []models.Diagnostic `json:"diagnostics"`
}
type Page struct {
	List      []Object `json:"list"`
	Count     int64    `json:"count"`
	PageIndex int      `json:"pageIndex"`
	PageSize  int      `json:"pageSize"`
}
type References struct {
	Outgoing []models.Reference `json:"outgoing"`
	Incoming []models.Reference `json:"incoming"`
}

func table(kind string) (string, error) {
	switch kind {
	case "interface":
		return (models.Interface{}).TableName(), nil
	case "route":
		return (models.Route{}).TableName(), nil
	case "zone":
		return (models.Zone{}).TableName(), nil
	case "address":
		return (models.Address{}).TableName(), nil
	case "address_group":
		return (models.AddressGroup{}).TableName(), nil
	case "service":
		return (models.Service{}).TableName(), nil
	case "service_group":
		return (models.ServiceGroup{}).TableName(), nil
	case "schedule":
		return (models.Schedule{}).TableName(), nil
	case "rule":
		return (models.Rule{}).TableName(), nil
	case "rule_binding":
		return (models.RuleBinding{}).TableName(), nil
	case "static_nat":
		return (models.StaticNAT{}).TableName(), nil
	case "dynamic_nat":
		return (models.DynamicNAT{}).TableName(), nil
	case "nat_pool":
		return (models.NATPool{}).TableName(), nil
	}
	return "", ErrInvalid
}
func (s *Store) Objects(ctx context.Context, snapshotID int, kind string, f Filter) (Page, error) {
	page := Page{List: []Object{}, PageIndex: f.PageIndex, PageSize: f.PageSize}
	if page.PageIndex <= 0 {
		page.PageIndex = 1
	}
	if page.PageSize <= 0 {
		page.PageSize = 20
	}
	if page.PageSize > 200 {
		page.PageSize = 200
	}
	tbl, err := table(kind)
	if err != nil {
		return page, err
	}
	var snap models.Snapshot
	if err = s.db.WithContext(ctx).First(&snap, snapshotID).Error; err != nil {
		return page, err
	}
	if snap.ProjectionState != "ready" {
		return page, ErrInvalid
	}
	db := s.db.WithContext(ctx).Table(tbl).Where("snapshot_id = ?", snapshotID)
	if f.Keyword != "" {
		pattern := "%" + strings.ToLower(f.Keyword) + "%"
		db = db.Where("(LOWER(name) LIKE ? OR LOWER(native_id) LIKE ? OR LOWER(scope) LIKE ?)", pattern, pattern, pattern)
	}
	if f.State != "" {
		db = db.Where("state = ?", f.State)
	}
	for _, v := range []struct{ value, typ string }{{f.Address, "address"}, {f.Protocol + f.Port, "service"}, {f.Zone, "zone"}} {
		if v.value != "" {
			ids, e := s.filterIDs(ctx, snapshotID, kind, v.typ, f)
			if e != nil {
				return page, e
			}
			if len(ids) == 0 {
				db = db.Where("1 = 0")
			} else {
				db = db.Where("id IN ?", ids)
			}
		}
	}
	if err = db.Count(&page.Count).Error; err != nil {
		return page, err
	}
	db = db.Order("position ASC, id ASC").Offset((page.PageIndex - 1) * page.PageSize).Limit(page.PageSize)
	page.List, err = readObjects(db, kind)
	return page, err
}
func (s *Store) Object(ctx context.Context, snapshotID int, kind string, id int) (Object, error) {
	tbl, err := table(kind)
	if err != nil {
		return Object{}, err
	}
	var snap models.Snapshot
	if err = s.db.WithContext(ctx).First(&snap, snapshotID).Error; err != nil {
		return Object{}, err
	}
	rows, err := readObjects(s.db.WithContext(ctx).Table(tbl).Where("snapshot_id = ? AND id = ?", snapshotID, id), kind)
	if err != nil {
		return Object{}, err
	}
	if len(rows) == 0 {
		return Object{}, gorm.ErrRecordNotFound
	}
	return rows[0], nil
}
func (s *Store) References(ctx context.Context, snapshotID int, kind string, id int) (References, error) {
	out := References{Outgoing: []models.Reference{}, Incoming: []models.Reference{}}
	if _, err := s.Object(ctx, snapshotID, kind, id); err != nil {
		return out, err
	}
	db := s.db.WithContext(ctx).Where("snapshot_id = ?", snapshotID)
	if err := db.Where("source_kind = ? AND source_id = ?", kind, id).Order("position ASC, id ASC").Find(&out.Outgoing).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Where("snapshot_id = ?", snapshotID).Where("target_kind = ? AND target_id = ? AND resolved = ?", kind, id, true).Order("position ASC, id ASC").Find(&out.Incoming).Error; err != nil {
		return out, err
	}
	return out, nil
}
func view(kind string, n models.Native, record any) Object {
	return Object{ID: n.ID, Kind: kind, NativeID: n.NativeID, Name: n.Name, Scope: n.Scope, Position: n.Position, State: n.State, Summary: n.Name, Record: record, Diagnostics: n.Diagnostics}
}
func readObjects(db *gorm.DB, kind string) ([]Object, error) {
	out := []Object{}
	switch kind {
	case "interface":
		var rows []models.Interface
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "route":
		var rows []models.Route
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "zone":
		var rows []models.Zone
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "address":
		var rows []models.Address
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "address_group":
		var rows []models.AddressGroup
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "service":
		var rows []models.Service
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "service_group":
		var rows []models.ServiceGroup
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "schedule":
		var rows []models.Schedule
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "rule":
		var rows []models.Rule
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "rule_binding":
		var rows []models.RuleBinding
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "static_nat":
		var rows []models.StaticNAT
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "dynamic_nat":
		var rows []models.DynamicNAT
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	case "nat_pool":
		var rows []models.NATPool
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, view(kind, r.Native, r.Record))
		}
	default:
		return nil, ErrInvalid
	}
	return out, nil
}

type node struct {
	kind string
	id   int
}

func (s *Store) filterIDs(ctx context.Context, snapshotID int, kind, filterType string, f Filter) ([]int, error) {
	selected := map[node]bool{}
	db := s.db.WithContext(ctx)
	switch filterType {
	case "address":
		var r flow.AddressRange
		if p, e := netip.ParsePrefix(strings.TrimSpace(f.Address)); e == nil {
			r, _ = flow.PrefixRange(p)
		} else if a, e := netip.ParseAddr(strings.TrimSpace(f.Address)); e == nil {
			r = flow.AddressRange{First: a, Last: a}
		} else {
			parts := strings.Split(f.Address, "-")
			if len(parts) != 2 {
				return nil, ErrInvalid
			}
			first, e1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
			last, e2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
			if e1 != nil || e2 != nil {
				return nil, ErrInvalid
			}
			r = flow.AddressRange{First: first, Last: last}
		}
		if !r.Valid() {
			return nil, ErrInvalid
		}
		var rows []models.AddressRange
		if err := db.Where("snapshot_id = ? AND family = ? AND first <= ? AND last >= ?", snapshotID, r.First.BitLen(), addressKey(r.Last), addressKey(r.First)).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			selected[node{r.ObjectKind, r.ObjectID}] = true
		}
	case "service":
		query := db.Where("snapshot_id = ?", snapshotID)
		if f.Protocol != "" {
			protocol, ok := map[string]int{"tcp": 6, "udp": 17, "icmp": 1, "icmpv6": 58, "sctp": 132}[strings.ToLower(f.Protocol)]
			if !ok {
				var e error
				protocol, e = strconv.Atoi(f.Protocol)
				if e != nil || protocol < 0 || protocol > 255 {
					return nil, ErrInvalid
				}
			}
			query = query.Where("protocol_low <= ? AND protocol_high >= ?", protocol, protocol)
		}
		if f.Port != "" {
			parts := strings.Split(f.Port, "-")
			if len(parts) > 2 {
				return nil, ErrInvalid
			}
			lo, e := strconv.Atoi(parts[0])
			if e != nil || lo < 0 || lo > 65535 {
				return nil, ErrInvalid
			}
			hi := lo
			if len(parts) == 2 {
				hi, e = strconv.Atoi(parts[1])
				if e != nil || hi < lo || hi > 65535 {
					return nil, ErrInvalid
				}
			}
			query = query.Where("destination_low <= ? AND destination_high >= ?", hi, lo)
		}
		var rows []models.ServiceTerm
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			selected[node{"service", r.ObjectID}] = true
		}
	case "zone":
		var rows []models.Zone
		if err := db.Where("snapshot_id = ? AND (native_id = ? OR name = ?)", snapshotID, f.Zone, f.Zone).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			selected[node{"zone", r.ID}] = true
		}
	}
	if filterType == "address" || filterType == "service" {
		for _, nativeKind := range []string{"rule", "static_nat", "dynamic_nat"} {
			if kind != nativeKind && kind != "rule_binding" {
				continue
			}
			tbl, _ := table(nativeKind)
			rows, err := readObjects(s.db.WithContext(ctx).Table(tbl).Where("snapshot_id = ?", snapshotID), nativeKind)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				var match fm.Match
				switch record := row.Record.(type) {
				case fm.RuleRecord:
					match = record.Match
				case fm.StaticNATRecord:
					match = record.Match
				case fm.DynamicNATRecord:
					match = record.Match
				}
				if filterType == "address" && (match.AnySource || match.AnyDestination) || filterType == "service" && match.AnyService {
					selected[node{nativeKind, row.ID}] = true
				}
			}
		}
	}
	// Follow reverse edges to include groups and rules that use matching leaf evidence.
	// Set membership terminates cycles without enumerating network hosts or ports.
	var refs []models.Reference
	if err := db.Where("snapshot_id = ? AND resolved = ?", snapshotID, true).Find(&refs).Error; err != nil {
		return nil, err
	}
	for changed := true; changed; {
		changed = false
		for _, r := range refs {
			source, target := node{r.SourceKind, r.SourceID}, node{r.TargetKind, r.TargetID}
			if selected[target] && !selected[source] {
				selected[source] = true
				changed = true
			}
		}
	}
	ids := []int{}
	for n := range selected {
		if n.kind == kind {
			ids = append(ids, n.id)
		}
	}
	return ids, nil
}
