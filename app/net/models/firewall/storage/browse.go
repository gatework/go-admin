package storage

import (
	"context"
	"net/netip"
	"strconv"
	"strings"

	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	models "github.com/gatework/motor/app/net/models/firewall"
)

// Filter carries filter state for this module.
type Filter struct {
	Keyword, Address, Protocol, Port, Zone, State string
	PageIndex, PageSize                           int
}

// Object carries object state for this module.
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

// Page carries page state for this module.
type Page struct {
	List      []Object `json:"list"`
	Count     int64    `json:"count"`
	PageIndex int      `json:"pageIndex"`
	PageSize  int      `json:"pageSize"`
}

// References carries references state for this module.
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
	return "", InvalidErr
}

// Objects provides objects for this module.
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
		return page, InvalidErr
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

// Object provides object for this module.
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

// References provides references for this module.
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
	switch kind {
	case "interface":
		return readObjectRows(db, func(r models.Interface) Object { return view(kind, r.Native, r.Record) })
	case "route":
		return readObjectRows(db, func(r models.Route) Object { return view(kind, r.Native, r.Record) })
	case "zone":
		return readObjectRows(db, func(r models.Zone) Object { return view(kind, r.Native, r.Record) })
	case "address":
		return readObjectRows(db, func(r models.Address) Object { return view(kind, r.Native, r.Record) })
	case "address_group":
		return readObjectRows(db, func(r models.AddressGroup) Object { return view(kind, r.Native, r.Record) })
	case "service":
		return readObjectRows(db, func(r models.Service) Object { return view(kind, r.Native, r.Record) })
	case "service_group":
		return readObjectRows(db, func(r models.ServiceGroup) Object { return view(kind, r.Native, r.Record) })
	case "schedule":
		return readObjectRows(db, func(r models.Schedule) Object { return view(kind, r.Native, r.Record) })
	case "rule":
		return readObjectRows(db, func(r models.Rule) Object { return view(kind, r.Native, r.Record) })
	case "rule_binding":
		return readObjectRows(db, func(r models.RuleBinding) Object { return view(kind, r.Native, r.Record) })
	case "static_nat":
		return readObjectRows(db, func(r models.StaticNAT) Object { return view(kind, r.Native, r.Record) })
	case "dynamic_nat":
		return readObjectRows(db, func(r models.DynamicNAT) Object { return view(kind, r.Native, r.Record) })
	case "nat_pool":
		return readObjectRows(db, func(r models.NATPool) Object { return view(kind, r.Native, r.Record) })
	default:
		return nil, InvalidErr
	}
}

func readObjectRows[T any](db *gorm.DB, convert func(T) Object) ([]Object, error) {
	var rows []T
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
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
		if err := selectAddressNodes(db, snapshotID, f, selected); err != nil {
			return nil, err
		}
	case "service":
		if err := selectServiceNodes(db, snapshotID, f, selected); err != nil {
			return nil, err
		}
	case "zone":
		if err := selectZoneNodes(db, snapshotID, f, selected); err != nil {
			return nil, err
		}
	}
	if err := s.selectWildcardNodes(ctx, snapshotID, kind, filterType, selected); err != nil {
		return nil, err
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

func selectAddressNodes(db *gorm.DB, snapshotID int, f Filter, selected map[node]bool) error {
	var r flow.AddressRange
	if p, e := netip.ParsePrefix(strings.TrimSpace(f.Address)); e == nil {
		r, _ = flow.PrefixRange(p)
	} else if a, e := netip.ParseAddr(strings.TrimSpace(f.Address)); e == nil {
		r = flow.AddressRange{First: a, Last: a}
	} else {
		parts := strings.Split(f.Address, "-")
		if len(parts) != 2 {
			return InvalidErr
		}
		first, e1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
		last, e2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
		if e1 != nil || e2 != nil {
			return InvalidErr
		}
		r = flow.AddressRange{First: first, Last: last}
	}
	if !r.Valid() {
		return InvalidErr
	}
	var rows []models.AddressRange
	if err := db.Where("snapshot_id = ? AND family = ? AND first <= ? AND last >= ?", snapshotID, r.First.BitLen(), addressKey(r.Last), addressKey(r.First)).Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		selected[node{r.ObjectKind, r.ObjectID}] = true
	}
	return nil
}

func selectServiceNodes(db *gorm.DB, snapshotID int, f Filter, selected map[node]bool) error {
	query := db.Where("snapshot_id = ?", snapshotID)
	if f.Protocol != "" {
		protocol, ok := map[string]int{"tcp": 6, "udp": 17, "icmp": 1, "icmpv6": 58, "sctp": 132}[strings.ToLower(f.Protocol)]
		if !ok {
			var e error
			protocol, e = strconv.Atoi(f.Protocol)
			if e != nil || protocol < 0 || protocol > 255 {
				return InvalidErr
			}
		}
		query = query.Where("protocol_low <= ? AND protocol_high >= ?", protocol, protocol)
	}
	if f.Port != "" {
		parts := strings.Split(f.Port, "-")
		if len(parts) > 2 {
			return InvalidErr
		}
		lo, e := strconv.Atoi(parts[0])
		if e != nil || lo < 0 || lo > 65535 {
			return InvalidErr
		}
		hi := lo
		if len(parts) == 2 {
			hi, e = strconv.Atoi(parts[1])
			if e != nil || hi < lo || hi > 65535 {
				return InvalidErr
			}
		}
		query = query.Where("destination_low <= ? AND destination_high >= ?", hi, lo)
	}
	var rows []models.ServiceTerm
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		selected[node{"service", r.ObjectID}] = true
	}
	return nil
}

func selectZoneNodes(db *gorm.DB, snapshotID int, f Filter, selected map[node]bool) error {
	var rows []models.Zone
	if err := db.Where("snapshot_id = ? AND (native_id = ? OR name = ?)", snapshotID, f.Zone, f.Zone).Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		selected[node{"zone", r.ID}] = true
	}
	return nil
}

func (s *Store) selectWildcardNodes(ctx context.Context, snapshotID int, kind, filterType string, selected map[node]bool) error {
	if filterType == "address" || filterType == "service" {
		for _, nativeKind := range []string{"rule", "static_nat", "dynamic_nat"} {
			if kind != nativeKind && kind != "rule_binding" {
				continue
			}
			tbl, _ := table(nativeKind)
			rows, err := readObjects(s.db.WithContext(ctx).Table(tbl).Where("snapshot_id = ?", snapshotID), nativeKind)
			if err != nil {
				return err
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
	return nil
}
