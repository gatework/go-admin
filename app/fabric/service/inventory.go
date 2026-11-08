package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	networkmodels "go-admin/app/network/models"
	"gorm.io/gorm"
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrInvalidInput = &Error{400, "Fabric 参数无效"}
	ErrNotFound     = &Error{404, "Fabric 或设备不存在或无权查看"}
	ErrConflict     = &Error{409, "Fabric 数据已更新，请重新加载"}
	ErrScope        = &Error{403, "此操作需要完整 Fabric 数据权限"}
	ErrUnavailable  = &Error{503, "Fabric 设备连接尚未配置"}
)

type Fabric struct{ base.Service }

func deviceScope(db *gorm.DB, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return db.Model(&networkmodels.Device{}).Scopes(actions.Permission(networkmodels.Device{}.TableName(), p))
}
func (s *Fabric) visibleDevices(ctx context.Context, p *actions.DataPermission) ([]networkmodels.Device, error) {
	rows := []networkmodels.Device{}
	err := deviceScope(s.Orm.WithContext(ctx), p).Order("id").Find(&rows).Error
	return rows, err
}
func (s *Fabric) reconcile(ctx context.Context, devices []networkmodels.Device) error {
	rules, err := s.Rules(ctx)
	if err != nil {
		return err
	}
	return s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, device := range devices {
			var m models.Membership
			err := tx.First(&m, "device_id = ?", device.Id).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			next := resolveDeviceMembership(device, m, rules.Rules, rules.Version)
			if err == nil && m.RuleVersion == next.RuleVersion && m.NameSnapshot == next.NameSnapshot && m.Status == next.Status && sameFabric(m.FabricID, next.FabricID) {
				continue
			}
			next.DeviceID = device.Id
			next.Version = m.Version + 1
			next.UpdatedAt = time.Now().UTC()
			if err == nil {
				result := tx.Model(&models.Membership{}).Where("device_id = ? AND version = ?", m.DeviceID, m.Version).Select("fabric_id", "mode", "status", "version", "rule_version", "name_snapshot", "updated_at").Updates(next)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrConflict
				}
			} else if err := tx.Create(&next).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// VisibleMembers anchors every Fabric query to the inventory's permission scope.
// complete is based on the actual membership set, never the caller's role label.
func (s *Fabric) VisibleMembers(ctx context.Context, id int, p *actions.DataPermission) ([]networkmodels.Device, bool, error) {
	if id <= 0 {
		return nil, false, ErrInvalidInput
	}
	var fabric models.Fabric
	if err := s.Orm.WithContext(ctx).First(&fabric, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	// Reconcile the complete identity list so renamed devices cannot retain stale ownership.
	var identities []networkmodels.Device
	if err := s.Orm.WithContext(ctx).Select("id", "name", "device_role").Find(&identities).Error; err != nil {
		return nil, false, err
	}
	if err := s.reconcile(ctx, identities); err != nil {
		return nil, false, err
	}
	ids := s.Orm.WithContext(ctx).Model(&models.Membership{}).Select("device_id").Where("fabric_id = ? AND status = ?", id, "assigned")
	rows := []networkmodels.Device{}
	if err := deviceScope(s.Orm.WithContext(ctx), p).Where("id IN (?)", ids).Order("id").Find(&rows).Error; err != nil {
		return nil, false, err
	}
	var total int64
	if err := s.Orm.WithContext(ctx).Model(&networkmodels.Device{}).Where("id IN (?)", ids).Count(&total).Error; err != nil {
		return nil, false, err
	}
	if total > 0 && len(rows) == 0 {
		return nil, false, ErrNotFound
	}
	return rows, int64(len(rows)) == total, nil
}
func (s *Fabric) RequireFullScope(ctx context.Context, id int, p *actions.DataPermission) ([]networkmodels.Device, error) {
	rows, full, err := s.VisibleMembers(ctx, id, p)
	if err != nil {
		return nil, err
	}
	if !full {
		return nil, ErrScope
	}
	return rows, nil
}
func (s *Fabric) Page(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Summary, int64, error) {
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	var fabrics []models.Fabric
	if err := s.Orm.WithContext(ctx).Order("id").Find(&fabrics).Error; err != nil {
		return nil, 0, err
	}
	out := []dto.Summary{}
	for _, f := range fabrics {
		if r.Keyword != "" && !strings.Contains(strings.ToLower(f.Code+" "+f.Name), strings.ToLower(r.Keyword)) {
			continue
		}
		d, err := s.Detail(ctx, f.ID, p)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		if r.Status == "" || d.State == r.Status {
			out = append(out, d.Summary)
		}
	}
	count := int64(len(out))
	start, end := pageBounds(len(out), r)
	return out[start:end], count, nil
}
func pageBounds(n int, r *dto.PageReq) (int, int) {
	start := (r.PageIndex - 1) * r.PageSize
	if start > n {
		start = n
	}
	end := start + r.PageSize
	if end > n {
		end = n
	}
	return start, end
}
func (s *Fabric) Settings(ctx context.Context, id int, r *dto.SettingsReq, p *actions.DataPermission) (models.Fabric, error) {
	var f models.Fabric
	if r.Version <= 0 || r.RuntimeMinutes < 1 || r.RuntimeMinutes > 1440 || r.ConfigMinutes < 1 || r.ConfigMinutes > 1440 {
		return f, ErrInvalidInput
	}
	if _, err := s.RequireFullScope(ctx, id, p); err != nil {
		return f, err
	}
	res := s.Orm.WithContext(ctx).Model(&f).Where("id = ? AND version = ?", id, r.Version).Updates(map[string]any{"collection_enabled": r.CollectionEnabled, "runtime_minutes": r.RuntimeMinutes, "config_minutes": r.ConfigMinutes, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return f, res.Error
	}
	if res.RowsAffected != 1 {
		return f, ErrConflict
	}
	return f, s.Orm.WithContext(ctx).First(&f, id).Error
}
func (s *Fabric) Members(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Member, int64, error) {
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	devices, err := s.visibleDevices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	if err := s.reconcile(ctx, devices); err != nil {
		return nil, 0, err
	}
	rules, err := s.Rules(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := []dto.Member{}
	for _, d := range devices {
		if r.Keyword != "" && !strings.Contains(strings.ToLower(d.Name+" "+d.IP), strings.ToLower(r.Keyword)) {
			continue
		}
		var m models.Membership
		if err := s.Orm.WithContext(ctx).First(&m, "device_id = ?", d.Id).Error; err != nil {
			return nil, 0, err
		}
		if r.Status != "" && r.Status != m.Status || r.FabricID > 0 && (m.FabricID == nil || *m.FabricID != r.FabricID) {
			continue
		}
		out = append(out, dto.Member{Membership: m, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Candidates: ruleCandidates(d.Name, rules.Rules)})
	}
	count := int64(len(out))
	start, end := pageBounds(len(out), r)
	return out[start:end], count, nil
}
func (s *Fabric) UpdateMember(ctx context.Context, id int, r *dto.MemberReq, actor int, p *actions.DataPermission) (dto.Member, error) {
	var out dto.Member
	if actor <= 0 || id <= 0 || (r.Mode != "auto" && r.Mode != "manual") || r.FabricID != nil && *r.FabricID <= 0 {
		return out, ErrInvalidInput
	}
	var d networkmodels.Device
	if err := deviceScope(s.Orm.WithContext(ctx), p).First(&d, id).Error; err != nil {
		return out, ErrNotFound
	}
	if r.FabricID != nil {
		if _, err := s.RequireFullScope(ctx, *r.FabricID, p); err != nil {
			return out, err
		}
	}
	if err := s.reconcile(ctx, []networkmodels.Device{d}); err != nil {
		return out, err
	}
	rules, err := s.Rules(ctx)
	if err != nil {
		return out, err
	}
	var m models.Membership
	if err := s.Orm.WithContext(ctx).First(&m, "device_id = ?", id).Error; err != nil {
		return out, err
	}
	m.Mode = r.Mode
	m.FabricID = r.FabricID
	m = resolveDeviceMembership(d, m, rules.Rules, rules.Version)
	res := s.Orm.WithContext(ctx).Model(&models.Membership{}).Where("device_id = ? AND version = ?", id, m.Version).Updates(map[string]any{"fabric_id": m.FabricID, "mode": m.Mode, "status": m.Status, "rule_version": m.RuleVersion, "name_snapshot": m.NameSnapshot, "version": gorm.Expr("version + 1"), "update_by": actor, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return out, res.Error
	}
	if res.RowsAffected != 1 {
		return out, ErrConflict
	}
	m.Version++
	return dto.Member{Membership: m, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Candidates: ruleCandidates(d.Name, rules.Rules)}, nil
}
func validText(s string, max int) bool {
	return utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, 0)
}
