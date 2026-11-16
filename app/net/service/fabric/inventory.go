package fabric

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	base "github.com/gatework/motor-core/sdk/service"
	models "github.com/gatework/motor/app/net/models/fabric"
	networkmodels "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/errs"
)

// Fabric failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is a request the service refuses as malformed.
	InvalidErr = errs.New(errs.KindInvalid, "fabric.invalid_input", "")
	// NotFoundErr is a fabric or device that does not exist or is out of scope.
	NotFoundErr = errs.New(errs.KindNotFound, "fabric.not_found", "")
	// ConflictErr is a write against fabric data that has since changed.
	ConflictErr = errs.New(errs.KindConflict, "fabric.version_conflict", "")
	// ScopeErr is an operation that needs the full fabric data scope.
	ScopeErr = errs.New(errs.KindForbidden, "fabric.full_scope_required", "")
	// UnavailableErr is a fabric device whose connection is not configured.
	UnavailableErr = errs.New(errs.KindUnavailable, "fabric.connection_not_configured", "")
	// UnsupportedPlatformErr is a device platform fabric changes cannot drive.
	UnsupportedPlatformErr = errs.New(errs.KindInvalid, "fabric.unsupported_platform", "")
)

//revive:enable:error-naming

// Fabric applies access scopes to fabric inventory, evidence and change operations.
type Fabric struct{ base.Service }

func deviceScope(db *gorm.DB, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return db.Model(&networkmodels.Device{}).Scopes(actions.Permission(networkmodels.Device{}.TableName(), p))
}

// VisibleMembers anchors every Fabric query to the inventory's permission scope.
// complete is based on the actual membership set, never the caller's role label.
func (s *Fabric) VisibleMembers(ctx context.Context, id int, p *actions.DataPermission) ([]networkmodels.Device, bool, error) {
	if id <= 0 {
		return nil, false, InvalidErr
	}
	var fabric models.Fabric
	if err := s.DB.WithContext(ctx).First(&fabric, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, NotFoundErr
		}
		return nil, false, err
	}
	// Reconcile the complete identity list so renamed devices cannot retain stale ownership.
	var identities []networkmodels.Device
	if err := s.DB.WithContext(ctx).Select("id", "name", "device_role").Find(&identities).Error; err != nil {
		return nil, false, err
	}
	if err := s.reconcile(ctx, identities); err != nil {
		return nil, false, err
	}
	ids := s.DB.WithContext(ctx).Model(&models.Membership{}).Select("device_id").Where("fabric_id = ? AND status = ?", id, "assigned")
	rows := []networkmodels.Device{}
	if err := deviceScope(s.DB.WithContext(ctx), p).Where("id IN (?)", ids).Order("id").Find(&rows).Error; err != nil {
		return nil, false, err
	}
	var total int64
	if err := s.DB.WithContext(ctx).Model(&networkmodels.Device{}).Where("id IN (?)", ids).Count(&total).Error; err != nil {
		return nil, false, err
	}
	if total > 0 && len(rows) == 0 {
		return nil, false, NotFoundErr
	}
	return rows, int64(len(rows)) == total, nil
}

// RequireFullScope provides the require full scope operation within the caller's authorized fabric scope.
func (s *Fabric) RequireFullScope(ctx context.Context, id int, p *actions.DataPermission) ([]networkmodels.Device, error) {
	rows, full, err := s.VisibleMembers(ctx, id, p)
	if err != nil {
		return nil, err
	}
	if !full {
		return nil, ScopeErr
	}
	return rows, nil
}

// Page provides the page operation within the caller's authorized fabric scope.
func (s *Fabric) Page(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Summary, int64, error) {
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	var fabrics []models.Fabric
	if err := s.DB.WithContext(ctx).Order("id").Find(&fabrics).Error; err != nil {
		return nil, 0, err
	}
	out := []dto.Summary{}
	for _, f := range fabrics {
		if r.Keyword != "" && !strings.Contains(strings.ToLower(f.Code+" "+f.Name), strings.ToLower(r.Keyword)) {
			continue
		}
		d, err := s.Detail(ctx, f.ID, p)
		if errors.Is(err, NotFoundErr) {
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

// Settings persists the selected tings settings.
func (s *Fabric) Settings(ctx context.Context, id int, r *dto.SettingsReq, p *actions.DataPermission) (models.Fabric, error) {
	var f models.Fabric
	if r.Version <= 0 || r.RuntimeMinutes < 1 || r.RuntimeMinutes > 1440 || r.ConfigMinutes < 1 || r.ConfigMinutes > 1440 {
		return f, InvalidErr
	}
	if _, err := s.RequireFullScope(ctx, id, p); err != nil {
		return f, err
	}
	res := s.DB.WithContext(ctx).Model(&f).Where("id = ? AND version = ?", id, r.Version).Updates(map[string]any{"collection_enabled": r.CollectionEnabled, "runtime_minutes": r.RuntimeMinutes, "config_minutes": r.ConfigMinutes, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return f, res.Error
	}
	if res.RowsAffected != 1 {
		return f, ConflictErr
	}
	return f, s.DB.WithContext(ctx).First(&f, id).Error
}

// Members provides the members operation within the caller's authorized fabric scope.
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
		if err := s.DB.WithContext(ctx).First(&m, "device_id = ?", d.ID).Error; err != nil {
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
func pageBounds(n int, r *dto.PageReq) (int, int) {
	start := min((r.PageIndex-1)*r.PageSize, n)
	end := min(start+r.PageSize, n)
	return start, end
}

// UpdateMember updates member after checking ownership and version constraints.
func (s *Fabric) UpdateMember(ctx context.Context, id int, r *dto.MemberReq, actor int, p *actions.DataPermission) (dto.Member, error) {
	var out dto.Member
	if actor <= 0 || id <= 0 || (r.Mode != "auto" && r.Mode != "manual") || r.FabricID != nil && *r.FabricID <= 0 {
		return out, InvalidErr
	}
	var d networkmodels.Device
	if err := deviceScope(s.DB.WithContext(ctx), p).First(&d, id).Error; err != nil {
		return out, NotFoundErr
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
	if err := s.DB.WithContext(ctx).First(&m, "device_id = ?", id).Error; err != nil {
		return out, err
	}
	m.Mode = r.Mode
	m.FabricID = r.FabricID
	m = resolveDeviceMembership(d, m, rules.Rules, rules.Version)
	res := s.DB.WithContext(ctx).Model(&models.Membership{}).Where("device_id = ? AND version = ?", id, m.Version).Updates(map[string]any{"fabric_id": m.FabricID, "mode": m.Mode, "status": m.Status, "rule_version": m.RuleVersion, "name_snapshot": m.NameSnapshot, "version": gorm.Expr("version + 1"), "update_by": actor, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return out, res.Error
	}
	if res.RowsAffected != 1 {
		return out, ConflictErr
	}
	m.Version++
	return dto.Member{Membership: m, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Candidates: ruleCandidates(d.Name, rules.Rules)}, nil
}
func (s *Fabric) visibleDevices(ctx context.Context, p *actions.DataPermission) ([]networkmodels.Device, error) {
	rows := []networkmodels.Device{}
	err := deviceScope(s.DB.WithContext(ctx), p).Order("id").Find(&rows).Error
	return rows, err
}
func (s *Fabric) reconcile(ctx context.Context, devices []networkmodels.Device) error {
	rules, err := s.Rules(ctx)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, device := range devices {
			var m models.Membership
			err := tx.First(&m, "device_id = ?", device.ID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			next := resolveDeviceMembership(device, m, rules.Rules, rules.Version)
			if err == nil && m.RuleVersion == next.RuleVersion && m.NameSnapshot == next.NameSnapshot && m.Status == next.Status && sameFabric(m.FabricID, next.FabricID) {
				continue
			}
			next.DeviceID = device.ID
			next.Version = m.Version + 1
			next.UpdatedAt = time.Now().UTC()
			if err == nil {
				result := tx.Model(&models.Membership{}).Where("device_id = ? AND version = ?", m.DeviceID, m.Version).Select("fabric_id", "mode", "status", "version", "rule_version", "name_snapshot", "updated_at").Updates(next)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ConflictErr
				}
			} else if err := tx.Create(&next).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func validText(s string, limit int) bool {
	return utf8.RuneCountInString(s) <= limit && !strings.ContainsRune(s, 0)
}
