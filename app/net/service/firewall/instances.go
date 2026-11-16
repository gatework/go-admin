package firewall

import (
	"context"
	"strings"

	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/vendors"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/grouping"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/actions"
)

// Instance provides instance for this module.
func (s *Firewall) Instance(ctx context.Context, id int, p *actions.DataPermission) (models.Instance, error) {
	var row models.Instance
	if id <= 0 {
		return row, InvalidErr
	}
	e := scopedInstances(s.db(ctx), p).First(&row, id).Error
	return row, safeError(e)
}

// CreateInstance provides create instance for this module.
func (s *Firewall) CreateInstance(ctx context.Context, in InstanceInput, p *actions.DataPermission) (models.Instance, error) {
	var out models.Instance
	if s.Actor <= 0 || in.DeviceID <= 0 || in.DomainID <= 0 || len(in.NativeContext) > 128 || (in.Mode != "routed" && in.Mode != "transparent") {
		return out, InvalidErr
	}
	var d network.Device
	var e error
	if e = deviceScope(s.db(ctx), p).First(&d, in.DeviceID).Error; e != nil {
		return out, safeError(e)
	}
	if d.DeviceType != "firewall" && d.DeviceRole != "firewall" {
		return out, InvalidErr
	}
	if strings.TrimSpace(in.Vendor) == "" {
		in.Vendor = inferFirewallVendor(d)
	}
	meta, e := vendors.Normalize(fm.Metadata{Vendor: strings.ToLower(strings.TrimSpace(in.Vendor)), Context: strings.TrimSpace(in.NativeContext)})
	if e != nil {
		return out, InvalidErr
	}
	if e = s.domain(ctx, in.DomainID); e != nil {
		return out, e
	}
	out = models.Instance{DeviceID: d.ID, Name: d.Name, Vendor: meta.Vendor, NativeContext: meta.Context, Mode: in.Mode, DomainID: in.DomainID, Version: 1, SettingsVersion: 1}
	out.Settings.Version = 1
	if out.Name == "" {
		out.Name = d.IP
	}
	if e = s.db(ctx).Create(&out).Error; e != nil {
		return models.Instance{}, conflictOnDuplicate(e)
	}
	return out, nil
}

// Instances provides instances for this module.
func (s *Firewall) Instances(ctx context.Context, p *Page, permission *actions.DataPermission) ([]models.Instance, int64, error) {
	rows := []models.Instance{}
	var count int64
	if e := p.Validate(); e != nil {
		return rows, 0, e
	}
	q := scopedInstances(s.db(ctx), permission)
	q = keyword(q, "name", p.Keyword)
	if p.DomainID > 0 {
		q = inDomain(q, s.db(ctx), p.DomainID)
	}
	if p.GroupID != nil {
		inGroup, e := grouping.DeviceScope(s.db(ctx), p.GroupID)
		if e != nil {
			return rows, 0, e
		}
		q = q.Where("device_id IN (?)", deviceScope(s.db(ctx), permission).Scopes(inGroup).Select("id"))
	}
	if e := q.Count(&count).Error; e != nil {
		return rows, 0, e
	}
	e := pageQuery(q, pValue(p)).Order("id DESC").Find(&rows).Error
	return rows, count, e
}

func deviceScope(db *gorm.DB, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return db.Model(&network.Device{}).Scopes(actions.Permission(network.Device{}.TableName(), p))
}

func scopedInstances(db *gorm.DB, p *actions.DataPermission) *gorm.DB {
	return db.Model(&models.Instance{}).Where("device_id IN (?)", deviceScope(db, p).Select("id"))
}

// InstanceInput carries instance input state for this module.
type InstanceInput struct {
	DeviceID      int    `json:"deviceId"`
	DomainID      int    `json:"domainId"`
	Vendor        string `json:"vendor"`
	NativeContext string `json:"nativeContext"`
	Mode          string `json:"mode"`
}

func inDomain(q, db *gorm.DB, domainID int) *gorm.DB {
	return q.Where("domain_id = ? OR id IN (?) OR id IN (?)", domainID,
		db.Model(&models.ScopeBinding{}).Select("instance_id").Where("domain_id = ?", domainID),
		db.Model(&models.ZoneBinding{}).Select("instance_id").Where("domain_id = ?", domainID))
}
