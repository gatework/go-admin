package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/config"
	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/database"
)

// CircuitSource is the read-only upstream boundary. No write operation exists
// here, and deterministic tests need neither credentials nor a live Zabbix.
type CircuitSource interface {
	Configured() bool
	Hosts(ctx context.Context, query string) (provider.ZabbixHostList, error)
	Items(ctx context.Context, hostID string, query string) (provider.ZabbixItemList, error)
	Host(ctx context.Context, id string) (provider.ZabbixHost, error)
	Item(ctx context.Context, id string) (provider.ZabbixItem, error)
	History(ctx context.Context, id string, valueType string, from int64, to int64) ([]provider.ZabbixSample, bool, error)
	Trends(ctx context.Context, id string, from int64, to int64) ([]provider.ZabbixTrend, bool, error)
}

// Circuit describes circuit for network operations.
type Circuit struct {
	base.Service
	Source CircuitSource
	Now    func() time.Time
}

// Page provides the page operation within the caller's authorized network scope.
func (s *Circuit) Page(ctx context.Context, r *dto.CircuitPageReq, p *actions.DataPermission) ([]inventory.Circuit, int64, error) {
	rows := []inventory.Circuit{}
	var count int64
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	if err := s.scoped(ctx, p).Scopes(circuitFilters(r)).Count(&count).Error; err != nil {
		return nil, 0, inventory.PersistenceErr
	}
	err := s.scoped(ctx, p).Scopes(circuitFilters(r)).Order("updated_at DESC, id DESC").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error
	if err != nil {
		return nil, 0, inventory.PersistenceErr
	}
	return rows, count, nil
}
func circuitScopeAllowed(p *actions.DataPermission) bool {
	return p != nil && (!config.Current().Settings.Application.EnableDP || (actions.IsValidDataScope(p.DataScope) && (p.DataScope != actions.DataScopeSelf || p.UserID > 0)))
}
func circuitPermission(table string, p *actions.DataPermission) func(*gorm.DB) *gorm.DB {
	if !circuitScopeAllowed(p) {
		return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }
	}
	return actions.Permission(table, p)
}

// Get loads the selected record within the caller's authorized scope.
func (s *Circuit) Get(ctx context.Context, id int, p *actions.DataPermission) (inventory.Circuit, error) {
	var row inventory.Circuit
	if id <= 0 {
		return row, inventory.InvalidErr
	}
	err := s.scoped(ctx, p).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, inventory.CircuitNotFoundErr
	}
	if err != nil {
		return row, inventory.PersistenceErr
	}
	return row, nil
}
func circuitFilters(r *dto.CircuitPageReq) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if keyword := strings.ToLower(r.Keyword); keyword != "" {
			pattern := database.LikeContains(keyword)
			clauses := []string{}
			values := []any{}
			for _, column := range []string{"circuit_no", "name", "contract_no", "provider", "region", "organization"} {
				clauses = append(clauses, "LOWER("+column+") LIKE ? "+database.LikeEscape)
				values = append(values, pattern)
			}
			db = db.Where("("+strings.Join(clauses, " OR ")+")", values...)
		}
		for _, f := range []struct{ column, value string }{{"status", r.Status}, {"category", r.Category}, {"provider", r.Provider}, {"region", r.Region}, {"organization", r.Organization}} {
			if f.value != "" {
				db = db.Where(f.column+" = ?", f.value)
			}
		}
		monitored := "(COALESCE(a_monitoring_in_item_id, '') <> '' OR COALESCE(a_monitoring_out_item_id, '') <> '' OR COALESCE(z_monitoring_in_item_id, '') <> '' OR COALESCE(z_monitoring_out_item_id, '') <> '')"
		switch r.Monitored {
		case "true":
			db = db.Where(monitored)
		case "false":
			db = db.Where("NOT " + monitored)
		}
		return db
	}
}

// Create creates the requested record within the caller's authorized scope.
func (s *Circuit) Create(ctx context.Context, r *dto.CircuitControlReq, actor int, p *actions.DataPermission) (inventory.Circuit, error) {
	var row inventory.Circuit
	if actor <= 0 {
		return row, inventory.InvalidErr
	}
	if !circuitScopeAllowed(p) {
		return row, inventory.CircuitScopeErr
	}
	if err := r.Validate(false); err != nil {
		return row, err
	}
	if err := s.validateDevices(ctx, r, p); err != nil {
		return row, err
	}
	if err := s.validateBindings(ctx, r, row); err != nil {
		return row, err
	}
	row.CircuitFields = inventory.CircuitFields(*r)
	row.Version = 1
	row.SetCreateBy(actor)
	row.SetUpdateBy(actor)
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return inventory.Circuit{}, circuitPersistenceError(err)
	}
	return row, nil
}

// Update updates the requested record after checking ownership and version constraints.
func (s *Circuit) Update(ctx context.Context, id int, r *dto.CircuitControlReq, actor int, p *actions.DataPermission) (inventory.Circuit, error) {
	var result inventory.Circuit
	if actor <= 0 {
		return result, inventory.InvalidErr
	}
	if err := r.Validate(true); err != nil {
		return result, err
	}
	old, err := s.Get(ctx, id, p)
	if err != nil {
		return result, err
	}
	if old.Version != r.Version {
		return result, inventory.ConflictErr
	}
	if old.Status == "terminated" && r.Status != "terminated" {
		return result, inventory.CircuitLifecycleErr
	}
	if err = s.validateDevices(ctx, r, p); err != nil {
		return result, err
	}
	if err = s.validateBindings(ctx, r, old); err != nil {
		return result, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		local := *s
		local.DB = tx
		updated := local.scoped(ctx, p).Where("id = ? AND version = ?", id, r.Version).Updates(circuitUpdates(r, actor))
		if updated.Error != nil {
			return circuitPersistenceError(updated.Error)
		}
		if updated.RowsAffected == 0 {
			if _, err := local.Get(ctx, id, p); err != nil {
				return err
			}
			return inventory.ConflictErr
		}
		var err error
		result, err = local.Get(ctx, id, p)
		return err
	})
	if err != nil {
		return inventory.Circuit{}, err
	}
	return result, nil
}

// Delete removes the selected records within the caller's authorized scope.
func (s *Circuit) Delete(ctx context.Context, id int, version int64, actor int, p *actions.DataPermission) error {
	if version <= 0 || actor <= 0 {
		return inventory.InvalidErr
	}
	row, err := s.Get(ctx, id, p)
	if err != nil {
		return err
	}
	if row.Version != version {
		return inventory.ConflictErr
	}
	if row.Status != "planned" {
		return inventory.CircuitLifecycleErr
	}
	result := s.scoped(ctx, p).Where("id = ? AND version = ? AND status = ?", id, version, "planned").Updates(map[string]any{"deleted_at": time.Now().UnixMilli(), "version": gorm.Expr("version + 1"), "update_by": actor})
	if result.Error != nil {
		return inventory.PersistenceErr
	}
	if result.RowsAffected == 0 {
		if _, err = s.Get(ctx, id, p); err != nil {
			return err
		}
		return inventory.ConflictErr
	}
	return nil
}
func sameBinding(a, b inventory.CircuitBinding) bool {
	return a.HostID == b.HostID && a.InItemID == b.InItemID && a.OutItemID == b.OutItemID
}

// Options provides the options operation within the caller's authorized network scope.
func (s *Circuit) Options(ctx context.Context, p *actions.DataPermission) (dto.CircuitOptions, error) {
	result := dto.CircuitOptions{Providers: []string{}, Regions: []string{}, Organizations: []string{}, ZabbixConfigured: s.source().Configured()}
	for _, option := range []struct {
		column string
		values *[]string
	}{{"provider", &result.Providers}, {"region", &result.Regions}, {"organization", &result.Organizations}} {
		if err := s.scoped(ctx, p).Distinct(option.column).Where(option.column+" <> ?", "").Order(option.column).Pluck(option.column, option.values).Error; err != nil {
			return result, inventory.PersistenceErr
		}
	}
	return result, nil
}
func safeCircuitSourceError(err error) error {
	for _, safe := range []error{inventory.InvalidErr, inventory.CircuitBindingErr, inventory.ZabbixNotConfiguredErr, inventory.ZabbixTimeoutErr} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return inventory.ZabbixUpstreamErr
}
func circuitPersistenceError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "ux_network_circuit_no") || strings.Contains(err.Error(), "UNIQUE constraint failed: sys_network_circuit.circuit_no") {
		return inventory.CircuitDuplicateErr
	}
	return inventory.PersistenceErr
}

// Hosts provides the hosts operation within the caller's authorized network scope.
func (s *Circuit) Hosts(ctx context.Context, keyword string) (provider.ZabbixHostList, error) {
	result, err := s.source().Hosts(ctx, keyword)
	if err != nil {
		return result, safeCircuitSourceError(err)
	}
	return result, nil
}

// Items provides the items operation within the caller's authorized network scope.
func (s *Circuit) Items(ctx context.Context, hostID, keyword string) (provider.ZabbixItemList, error) {
	result, err := s.source().Items(ctx, hostID, keyword)
	if err != nil {
		return result, safeCircuitSourceError(err)
	}
	return result, nil
}
func circuitUpdates(r *dto.CircuitControlReq, actor int) map[string]any {
	update := map[string]any{"circuit_no": r.CircuitNo, "name": r.Name, "provider": r.Provider, "category": r.Category, "purpose": r.Purpose, "status": r.Status, "bandwidth_mbps": r.BandwidthMbps, "region": r.Region, "organization": r.Organization, "contract_no": r.ContractNo, "installed_on": r.InstalledOn, "expires_on": r.ExpiresOn, "payer": r.Payer, "fee": r.Fee, "currency": r.Currency, "billing_cycle": r.BillingCycle, "route_notes": r.RouteNotes, "notes": r.Notes, "update_by": actor, "version": gorm.Expr("version + 1")}
	for _, side := range []struct {
		prefix string
		e      inventory.CircuitEndpoint
	}{{"a_", r.AEndpoint}, {"z_", r.ZEndpoint}} {
		e := side.e
		b := e.Monitoring
		for key, value := range map[string]any{"device_id": e.DeviceID, "name": e.Name, "address": e.Address, "interface": e.Interface, "ip": e.IP, "contact_name": e.ContactName, "contact_phone": e.ContactPhone, "monitoring_host_id": b.HostID, "monitoring_host_name": b.HostName, "monitoring_in_item_id": b.InItemID, "monitoring_in_item_name": b.InItemName, "monitoring_out_item_id": b.OutItemID, "monitoring_out_item_name": b.OutItemName} {
			update[side.prefix+key] = value
		}
	}
	return update
}
func (s *Circuit) source() CircuitSource {
	if s.Source != nil {
		return s.Source
	}
	return provider.ZabbixFromEnvironment()
}
func (s *Circuit) scoped(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	return s.DB.WithContext(ctx).Model(&inventory.Circuit{}).Scopes(circuitPermission(inventory.Circuit{}.TableName(), p))
}
func (s *Circuit) validateDevices(ctx context.Context, r *dto.CircuitControlReq, p *actions.DataPermission) error {
	for _, e := range []inventory.CircuitEndpoint{r.AEndpoint, r.ZEndpoint} {
		if e.DeviceID == nil {
			continue
		}
		var count int64
		err := s.DB.WithContext(ctx).Model(&inventory.Device{}).Scopes(circuitPermission(inventory.Device{}.TableName(), p)).Where("id = ?", *e.DeviceID).Count(&count).Error
		if err != nil {
			return inventory.PersistenceErr
		}
		if count != 1 {
			return inventory.CircuitDeviceErr
		}
	}
	return nil
}
func (s *Circuit) validateBindings(ctx context.Context, r *dto.CircuitControlReq, old inventory.Circuit) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	source := s.source()
	for _, pair := range []struct {
		next *inventory.CircuitBinding
		old  inventory.CircuitBinding
	}{{&r.AEndpoint.Monitoring, old.AEndpoint.Monitoring}, {&r.ZEndpoint.Monitoring, old.ZEndpoint.Monitoring}} {
		b := pair.next
		if sameBinding(*b, pair.old) {
			*b = pair.old
			continue
		}
		if b.HostID == "" {
			*b = inventory.CircuitBinding{}
			continue
		}
		if !source.Configured() {
			return inventory.ZabbixNotConfiguredErr
		}
		host, err := source.Host(ctx, b.HostID)
		if err != nil {
			return safeCircuitSourceError(err)
		}
		if host.ID != b.HostID {
			return inventory.CircuitBindingErr
		}
		b.HostName = host.Name
		b.InItemName = ""
		b.OutItemName = ""
		for _, direction := range []struct {
			id   string
			name *string
		}{{b.InItemID, &b.InItemName}, {b.OutItemID, &b.OutItemName}} {
			if direction.id == "" {
				continue
			}
			item, err := source.Item(ctx, direction.id)
			if err != nil {
				return safeCircuitSourceError(err)
			}
			if item.ID != direction.id || !item.IsRateFor(b.HostID) {
				return inventory.CircuitBindingErr
			}
			*direction.name = item.Name
		}
	}
	return nil
}
