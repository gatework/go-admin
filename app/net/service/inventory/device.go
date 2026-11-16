package inventory

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/grouping"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider/netdisco"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/csvsafe"
	"github.com/gatework/motor/common/database"
)

// Source describes inventory source for network operations.
type Source interface {
	Configured() bool
	Fetch(ctx context.Context) ([]inventory.SourceFacts, error)
}

// Device describes device for network operations.
type Device struct {
	base.Service
	Source Source
}

// Page provides the page operation within the caller's authorized network scope.
func (s *Device) Page(ctx context.Context, r *dto.DevicePageReq, p *actions.DataPermission) ([]inventory.Device, int64, error) {
	rows := make([]inventory.Device, 0)
	var count int64
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	groupFilter, err := grouping.DeviceScope(s.DB.WithContext(ctx), r.GroupID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.scoped(ctx, p).Scopes(filters(r), groupFilter).Count(&count).Error; err != nil {
		return nil, 0, err
	}
	err = s.scoped(ctx, p).Scopes(filters(r), groupFilter).Order("updated_at DESC, id DESC").Scopes(coredto.Paginate(r.PageSize, r.PageIndex)).Find(&rows).Error
	if err == nil {
		err = populateGroupNames(s.DB.WithContext(ctx), rows)
	}
	return rows, count, err
}

// Get loads the selected record within the caller's authorized scope.
func (s *Device) Get(ctx context.Context, id int, p *actions.DataPermission) (inventory.Device, error) {
	var row inventory.Device
	if id <= 0 {
		return row, inventory.InvalidErr
	}
	err := s.scoped(ctx, p).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, inventory.NotFoundErr
	}
	if err == nil {
		rows := []inventory.Device{row}
		err = populateGroupNames(s.DB.WithContext(ctx), rows)
		row = rows[0]
	}
	return row, err
}
func filters(r *dto.DevicePageReq) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if r.Absent {
			db = db.Where("last_synced_at < (SELECT last_success_at FROM " + inventory.SyncState{}.TableName() + " WHERE id = 1)")
		}
		if r.FirewallOnly {
			db = db.Where("(device_type = ? OR device_role = ?)", "firewall", "firewall")
		}
		if keyword := strings.ToLower(strings.TrimSpace(r.Keyword)); keyword != "" {
			pattern := database.LikeContains(keyword)
			clauses := make([]string, 0, 7)
			values := make([]any, 0, 7)
			for _, column := range []string{"name", "hostname", "ip", "serial", "vendor", "model", "location"} {
				clauses = append(clauses, "LOWER("+column+") LIKE ? "+database.LikeEscape)
				values = append(values, pattern)
			}
			db = db.Where("("+strings.Join(clauses, " OR ")+")", values...)
		}
		for _, filter := range []struct {
			column, value string
			empty         bool
		}{{"vendor", r.Vendor, r.VendorEmpty}, {"model", r.Model, false}, {"status", r.Status, false}, {"device_type", r.DeviceType, r.DeviceTypeEmpty}, {"device_role", r.DeviceRole, r.DeviceRoleEmpty}} {
			if filter.empty {
				db = db.Where("COALESCE(" + filter.column + ", '') = ''")
			} else if value := strings.TrimSpace(filter.value); value != "" {
				db = db.Where(filter.column+" = ?", value)
			}
		}
		if tag := strings.TrimSpace(r.Tag); tag != "" {
			encoded, _ := json.Marshal(tag)
			db = db.Where("tags LIKE ? "+database.LikeEscape, database.LikeContains(string(encoded)))
		}
		return db
	}
}

// Update updates the requested record after checking ownership and version constraints.
func (s *Device) Update(ctx context.Context, id int, r *dto.DeviceUpdateReq, actor int, p *actions.DataPermission) (inventory.Device, error) {
	var row inventory.Device
	if id <= 0 || actor <= 0 {
		return row, inventory.InvalidErr
	}
	if err := r.Validate(); err != nil {
		return row, err
	}
	// A map includes intentionally cleared values; the DTO is the only field
	// allowlist and the SQL version predicate is the concurrency boundary.
	tags, err := json.Marshal(r.Tags)
	if err != nil {
		return row, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		scoped := Device{DB: tx}
		current, err := scoped.Get(ctx, id, p)
		if err != nil {
			return err
		}
		if current.Version != r.Version {
			return inventory.ConflictErr
		}
		groups, err := grouping.Load(tx)
		if err != nil {
			return err
		}
		groupID, err := resolveMembership(groups, current, r.GroupID, r.GroupSource)
		if err != nil {
			return err
		}
		role, kind := r.DeviceRole, r.DeviceType
		if r.ClassificationSource == "auto" {
			role, kind = grouping.Classification(current.Name, current.Serial)
		}
		update := scoped.scoped(ctx, p).Where("id = ? AND version = ?", id, r.Version).Updates(map[string]any{
			"group_id": groupID, "group_source": r.GroupSource, "classification_source": r.ClassificationSource,
			"status": r.Status, "device_type": kind, "device_role": role, "owner": r.Owner, "contact_email": r.ContactEmail, "contact_phone": r.ContactPhone, "tags": string(tags), "notes": r.Notes, "update_by": actor, "version": gorm.Expr("version + 1"),
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return inventory.ConflictErr
		}
		return nil
	})
	if err != nil {
		return row, err
	}
	return s.Get(ctx, id, p)
}

// Options provides the options operation within the caller's authorized network scope.
func (s *Device) Options(ctx context.Context, p *actions.DataPermission) (dto.DeviceOptions, error) {
	result := dto.DeviceOptions{Vendors: []string{}, Models: []string{}}
	for _, option := range []struct {
		column string
		values *[]string
	}{{"vendor", &result.Vendors}, {"model", &result.Models}} {
		if err := s.scoped(ctx, p).Distinct(option.column).Where(option.column+" <> ?", "").Order(option.column).Pluck(option.column, option.values).Error; err != nil {
			return result, err
		}
	}
	return result, nil
}

// Export provides the export operation within the caller's authorized network scope.
func (s *Device) Export(ctx context.Context, r *dto.DevicePageReq, p *actions.DataPermission) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	var rows []inventory.Device
	groupFilter, err := grouping.DeviceScope(s.DB.WithContext(ctx), r.GroupID)
	if err != nil {
		return nil, err
	}
	if err := s.scoped(ctx, p).Scopes(filters(r), groupFilter).Order("updated_at DESC, id DESC").Limit(maxExportRows + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > maxExportRows {
		return nil, inventory.ExportLimitErr
	}
	if err := populateGroupNames(s.DB.WithContext(ctx), rows); err != nil {
		return nil, err
	}
	buffer := bytes.NewBufferString("\ufeff")
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"ID", "管理 IP", "设备名称", "主机名", "序列号", "厂商", "型号", "系统", "系统版本", "位置", "来源联系信息", "描述", "MAC 地址", "来源标签", "分组", "台账状态", "设备类型", "设备角色", "负责人", "联系邮箱", "联系电话", "标签", "备注", "来源创建时间", "最近发现时间", "最近同步时间"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		cells := []string{strconv.Itoa(row.ID), row.IP, row.Name, row.Hostname, row.Serial, row.Vendor, row.HardwareModel, row.OS, row.OSVersion, row.Location, row.Contact, row.Description, row.MACAddress, strings.Join(row.SourceTags, "; "), row.Group, row.Status, row.DeviceType, row.DeviceRole, row.Owner, row.ContactEmail, row.ContactPhone, strings.Join(row.Tags, "; "), row.Notes, row.SourceCreatedAt, row.LastDiscoveredAt, row.LastSyncedAt.UTC().Format(time.RFC3339)}
		for i := range cells {
			cells[i] = csvsafe.Cell(cells[i])
		}
		if err := writer.Write(cells); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}
func (s *Device) source() Source {
	if s.Source != nil {
		return s.Source
	}
	return netdisco.FromEnvironment()
}
func (s *Device) scoped(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return s.DB.WithContext(ctx).Model(&inventory.Device{}).Scopes(actions.Permission(inventory.Device{}.TableName(), p))
}
