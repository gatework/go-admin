package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gatework/go-zabbix"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"

	"go-admin/app/network"
	"go-admin/app/network/models"
	"go-admin/app/network/provider"
	"go-admin/app/network/service/dto"
)

// CircuitSource is the read-only upstream boundary. No write operation exists
// here, and deterministic tests need neither credentials nor a live Zabbix.
type CircuitSource interface {
	zabbix.TrafficSource
	Configured() bool
	Hosts(context.Context, string) (zabbix.MonitoredHostList, error)
	Items(context.Context, string, string) (zabbix.MonitoredItemList, error)
}
type Circuit struct {
	base.Service
	Source CircuitSource
	Now    func() time.Time
}

func (s *Circuit) source() CircuitSource {
	if s.Source != nil {
		return s.Source
	}
	return provider.ZabbixFromEnvironment()
}
func circuitScopeAllowed(p *actions.DataPermission) bool {
	return p != nil && (!config.ApplicationConfig.EnableDP || (actions.IsValidDataScope(p.DataScope) && (p.DataScope != actions.DataScopeSelf || p.UserId > 0)))
}
func circuitPermission(table string, p *actions.DataPermission) func(*gorm.DB) *gorm.DB {
	if !circuitScopeAllowed(p) {
		return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }
	}
	return actions.Permission(table, p)
}
func (s *Circuit) scoped(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	return s.Orm.WithContext(ctx).Model(&models.Circuit{}).Scopes(circuitPermission(models.Circuit{}.TableName(), p))
}
func circuitFilters(r *dto.CircuitPageReq) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if keyword := strings.ToLower(r.Keyword); keyword != "" {
			pattern := "%" + escapeLike(keyword) + "%"
			clauses := []string{}
			values := []any{}
			for _, column := range []string{"circuit_no", "name", "contract_no", "provider", "region", "organization"} {
				clauses = append(clauses, "LOWER("+column+") LIKE ? ESCAPE '!'")
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
		if r.Monitored == "true" {
			db = db.Where(monitored)
		} else if r.Monitored == "false" {
			db = db.Where("NOT " + monitored)
		}
		return db
	}
}
func (s *Circuit) Page(ctx context.Context, r *dto.CircuitPageReq, p *actions.DataPermission) ([]models.Circuit, int64, error) {
	rows := []models.Circuit{}
	var count int64
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	if err := s.scoped(ctx, p).Scopes(circuitFilters(r)).Count(&count).Error; err != nil {
		return nil, 0, network.ErrPersistence
	}
	err := s.scoped(ctx, p).Scopes(circuitFilters(r)).Order("updated_at DESC, id DESC").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error
	if err != nil {
		return nil, 0, network.ErrPersistence
	}
	return rows, count, nil
}
func (s *Circuit) Get(ctx context.Context, id int, p *actions.DataPermission) (models.Circuit, error) {
	var row models.Circuit
	if id <= 0 {
		return row, network.ErrInvalidInput
	}
	err := s.scoped(ctx, p).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, network.ErrCircuitNotFound
	}
	if err != nil {
		return row, network.ErrPersistence
	}
	return row, nil
}
func (s *Circuit) validateDevices(ctx context.Context, r *dto.CircuitControlReq, p *actions.DataPermission) error {
	for _, e := range []models.CircuitEndpoint{r.AEndpoint, r.ZEndpoint} {
		if e.DeviceID == nil {
			continue
		}
		var count int64
		err := s.Orm.WithContext(ctx).Model(&models.Device{}).Scopes(circuitPermission(models.Device{}.TableName(), p)).Where("id = ?", *e.DeviceID).Count(&count).Error
		if err != nil {
			return network.ErrPersistence
		}
		if count != 1 {
			return network.ErrCircuitDevice
		}
	}
	return nil
}
func sameBinding(a, b models.CircuitBinding) bool {
	return a.HostID == b.HostID && a.InItemID == b.InItemID && a.OutItemID == b.OutItemID
}
func (s *Circuit) validateBindings(ctx context.Context, r *dto.CircuitControlReq, old models.Circuit) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	source := s.source()
	for _, pair := range []struct {
		next *models.CircuitBinding
		old  models.CircuitBinding
	}{{&r.AEndpoint.Monitoring, old.AEndpoint.Monitoring}, {&r.ZEndpoint.Monitoring, old.ZEndpoint.Monitoring}} {
		b := pair.next
		if sameBinding(*b, pair.old) {
			*b = pair.old
			continue
		}
		if b.HostID == "" {
			*b = models.CircuitBinding{}
			continue
		}
		if !source.Configured() {
			return network.ErrZabbixNotConfigured
		}
		host, err := source.Host(ctx, b.HostID)
		if err != nil {
			return safeCircuitSourceError(err)
		}
		if host.ID != b.HostID {
			return network.ErrCircuitBinding
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
				return network.ErrCircuitBinding
			}
			*direction.name = item.Name
		}
	}
	return nil
}
func safeCircuitSourceError(err error) error {
	for _, safe := range []*network.Error{network.ErrInvalidInput, network.ErrCircuitBinding, network.ErrZabbixNotConfigured, network.ErrZabbixTimeout} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return network.ErrZabbixUpstream
}
func circuitPersistenceError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "ux_network_circuit_no") || strings.Contains(err.Error(), "UNIQUE constraint failed: sys_network_circuit.circuit_no") {
		return network.ErrCircuitDuplicate
	}
	return network.ErrPersistence
}
func (s *Circuit) Create(ctx context.Context, r *dto.CircuitControlReq, actor int, p *actions.DataPermission) (models.Circuit, error) {
	var row models.Circuit
	if actor <= 0 {
		return row, network.ErrInvalidInput
	}
	if !circuitScopeAllowed(p) {
		return row, network.ErrCircuitScope
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
	row.CircuitFields = models.CircuitFields(*r)
	row.Version = 1
	row.SetCreateBy(actor)
	row.SetUpdateBy(actor)
	if err := s.Orm.WithContext(ctx).Create(&row).Error; err != nil {
		return models.Circuit{}, circuitPersistenceError(err)
	}
	return row, nil
}
func (s *Circuit) Update(ctx context.Context, id int, r *dto.CircuitControlReq, actor int, p *actions.DataPermission) (models.Circuit, error) {
	var result models.Circuit
	if actor <= 0 {
		return result, network.ErrInvalidInput
	}
	if err := r.Validate(true); err != nil {
		return result, err
	}
	old, err := s.Get(ctx, id, p)
	if err != nil {
		return result, err
	}
	if old.Version != r.Version {
		return result, network.ErrConflict
	}
	if old.Status == "terminated" && r.Status != "terminated" {
		return result, network.ErrCircuitLifecycle
	}
	if err = s.validateDevices(ctx, r, p); err != nil {
		return result, err
	}
	if err = s.validateBindings(ctx, r, old); err != nil {
		return result, err
	}
	err = s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		local := *s
		local.Orm = tx
		updated := local.scoped(ctx, p).Where("id = ? AND version = ?", id, r.Version).Updates(circuitUpdates(r, actor))
		if updated.Error != nil {
			return circuitPersistenceError(updated.Error)
		}
		if updated.RowsAffected == 0 {
			if _, err := local.Get(ctx, id, p); err != nil {
				return err
			}
			return network.ErrConflict
		}
		var err error
		result, err = local.Get(ctx, id, p)
		return err
	})
	if err != nil {
		return models.Circuit{}, err
	}
	return result, nil
}
func circuitUpdates(r *dto.CircuitControlReq, actor int) map[string]any {
	update := map[string]any{"circuit_no": r.CircuitNo, "name": r.Name, "provider": r.Provider, "category": r.Category, "purpose": r.Purpose, "status": r.Status, "bandwidth_mbps": r.BandwidthMbps, "region": r.Region, "organization": r.Organization, "contract_no": r.ContractNo, "installed_on": r.InstalledOn, "expires_on": r.ExpiresOn, "payer": r.Payer, "fee": r.Fee, "currency": r.Currency, "billing_cycle": r.BillingCycle, "route_notes": r.RouteNotes, "notes": r.Notes, "update_by": actor, "version": gorm.Expr("version + 1")}
	for _, side := range []struct {
		prefix string
		e      models.CircuitEndpoint
	}{{"a_", r.AEndpoint}, {"z_", r.ZEndpoint}} {
		e := side.e
		b := e.Monitoring
		for key, value := range map[string]any{"device_id": e.DeviceID, "name": e.Name, "address": e.Address, "interface": e.Interface, "ip": e.IP, "contact_name": e.ContactName, "contact_phone": e.ContactPhone, "monitoring_host_id": b.HostID, "monitoring_host_name": b.HostName, "monitoring_in_item_id": b.InItemID, "monitoring_in_item_name": b.InItemName, "monitoring_out_item_id": b.OutItemID, "monitoring_out_item_name": b.OutItemName} {
			update[side.prefix+key] = value
		}
	}
	return update
}
func (s *Circuit) Delete(ctx context.Context, id int, version int64, actor int, p *actions.DataPermission) error {
	if version <= 0 || actor <= 0 {
		return network.ErrInvalidInput
	}
	row, err := s.Get(ctx, id, p)
	if err != nil {
		return err
	}
	if row.Version != version {
		return network.ErrConflict
	}
	if row.Status != "planned" {
		return network.ErrCircuitLifecycle
	}
	result := s.scoped(ctx, p).Where("id = ? AND version = ? AND status = ?", id, version, "planned").Updates(map[string]any{"deleted_at": time.Now().UnixMilli(), "version": gorm.Expr("version + 1"), "update_by": actor})
	if result.Error != nil {
		return network.ErrPersistence
	}
	if result.RowsAffected == 0 {
		if _, err = s.Get(ctx, id, p); err != nil {
			return err
		}
		return network.ErrConflict
	}
	return nil
}
func (s *Circuit) Options(ctx context.Context, p *actions.DataPermission) (dto.CircuitOptions, error) {
	result := dto.CircuitOptions{Providers: []string{}, Regions: []string{}, Organizations: []string{}, ZabbixConfigured: s.source().Configured()}
	for _, option := range []struct {
		column string
		values *[]string
	}{{"provider", &result.Providers}, {"region", &result.Regions}, {"organization", &result.Organizations}} {
		if err := s.scoped(ctx, p).Distinct(option.column).Where(option.column+" <> ?", "").Order(option.column).Pluck(option.column, option.values).Error; err != nil {
			return result, network.ErrPersistence
		}
	}
	return result, nil
}
func (s *Circuit) Hosts(ctx context.Context, keyword string) (zabbix.MonitoredHostList, error) {
	result, err := s.source().Hosts(ctx, keyword)
	if err != nil {
		return result, safeCircuitSourceError(err)
	}
	return result, nil
}
func (s *Circuit) Items(ctx context.Context, hostID, keyword string) (zabbix.MonitoredItemList, error) {
	result, err := s.source().Items(ctx, hostID, keyword)
	if err != nil {
		return result, safeCircuitSourceError(err)
	}
	return result, nil
}
