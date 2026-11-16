package wireless

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	wireless "github.com/gatework/go-wireless"
	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/delivery"
	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/errors"
	"github.com/gatework/motor/common/paging"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is a request the service refuses as malformed.
	InvalidErr = errors.New(errors.KindInvalid, "wireless.invalid_input", "")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = errors.New(errors.KindConflict, "wireless.version_conflict", "")
	// ScopeErr is a resource that does not exist or that the caller's data
	// scope does not reach; the two are deliberately indistinguishable.
	ScopeErr = errors.New(errors.KindNotFound, "wireless.not_found_or_forbidden", "")
)

//revive:enable:error-naming

// Freshness defines the freshness used by wireless operations.
const Freshness = 36 * time.Hour

// Wireless serves scoped wireless inventory, evidence and collection operations.
type Wireless struct {
	base.Service
	Actor int
}

// Tables returns the persistent records required by the wireless module.
func Tables() []any {
	return []any{&models.Dimension{}, &models.Resource{}, &models.Collection{}, &models.NeighborState{}, &models.Audit{}}
}

// Management provides the management operation within the caller's authorized wireless scope.
func (s *Wireless) Management(ctx context.Context, controller, id int, r dto.ManagementReq, p *actions.DataPermission) (dto.Resource, error) {
	var result dto.Resource
	var row models.Resource
	if !validManagement(r) {
		return result, InvalidErr
	}
	if _, e := s.controller(ctx, controller, p); e != nil {
		return result, e
	}
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Resource{}).Where("id = ? AND controller_id = ? AND kind = ? AND version = ?", id, controller, "access_points", r.Version).Updates(map[string]any{"local_alias": r.LocalAlias, "installation_location": r.InstallationLocation, "owner": r.Owner, "notes": r.Notes, "version": gorm.Expr("version + 1"), "updated_at": time.Now().UTC()})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if e := tx.First(&row, id).Error; e != nil {
			return e
		}
		return s.audit(tx, controller, id, "management", row.Version)
	})
	if err != nil {
		return result, err
	}
	rows, err := s.resourceViews(ctx, []models.Resource{row}, p)
	if err != nil {
		return result, err
	}
	return rows[0], nil
}
func hash(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func (s *Wireless) db(ctx context.Context) *gorm.DB {
	return s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}
func (s *Wireless) devices(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	return s.db(ctx).Model(&network.Device{}).Scopes(actions.Permission((network.Device{}).TableName(), p))
}
func (s *Wireless) controller(ctx context.Context, id int, p *actions.DataPermission) (network.Device, error) {
	var d network.Device
	e := s.devices(ctx, p).Where("id = ? AND device_type = ?", id, "wireless_controller").First(&d).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ScopeErr
	}
	return d, e
}
func source(d network.Device, c delivery.Connection) string {
	return hash([]any{d.ID, d.IP, d.Name, d.Hostname, d.Vendor, d.HardwareModel, d.OS, d.DeviceType, d.Status, d.Version, c.Profile, c.Protocol, c.Port, c.Version})
}
func platform(c delivery.Connection) string {
	switch c.Profile {
	case "huawei_vrp", "huawei":
		return "huawei"
	default:
		return c.Profile
	}
}
func (s *Wireless) connection(ctx context.Context, d network.Device) (delivery.Connection, error) {
	return deviceaccess.FromEnvironment().Resolve(ctx, s.DB, d, deviceaccess.Read)
}
func validManagement(r dto.ManagementReq) bool {
	return r.Version > 0 && utf8.ValidString(r.LocalAlias+r.InstallationLocation+r.Owner+r.Notes) && utf8.RuneCountInString(r.LocalAlias) <= 255 && utf8.RuneCountInString(r.InstallationLocation) <= 255 && utf8.RuneCountInString(r.Owner) <= 255 && utf8.RuneCountInString(r.Notes) <= 4000
}
func (s *Wireless) dimensions(p string, requested []wireless.Dimension) ([]wireless.Dimension, error) {
	profile, e := wireless.NewProfile(p, "")
	if e != nil {
		return nil, InvalidErr
	}
	if requested == nil {
		out := []wireless.Dimension{}
		for _, d := range profile.Dimensions() {
			if d != wireless.LLDP {
				out = append(out, d)
			}
		}
		return out, nil
	}
	if len(requested) == 0 {
		return nil, InvalidErr
	}
	valid := map[wireless.Dimension]bool{}
	for _, d := range wireless.SupportedDimensions() {
		valid[d] = true
	}
	seen := map[wireless.Dimension]bool{}
	out := []wireless.Dimension{}
	for _, d := range requested {
		if !valid[d] || seen[d] {
			return nil, InvalidErr
		}
		seen[d] = true
		out = append(out, d)
	}
	if seen[wireless.LLDP] && !seen[wireless.AccessPoints] {
		out = append(out, wireless.AccessPoints)
	}
	if (seen[wireless.APGroups] || seen[wireless.ServiceTemplates]) && !seen[wireless.Configuration] {
		out = append(out, wireless.Configuration)
	}
	slices.Sort(out)
	return out, nil
}
func (s *Wireless) audit(tx *gorm.DB, controller, id int, action string, version int64) error {
	return tx.Create(&models.Audit{ActorID: s.Actor, ControllerID: controller, ResourceID: id, Action: action, Version: version, CreatedAt: time.Now().UTC()}).Error
}

func page(r *dto.PageReq) error {
	if r == nil {
		return InvalidErr
	}
	if !paging.Normalize(&r.PageIndex, &r.PageSize) || r.GroupID != nil && *r.GroupID < 0 || len(r.Keyword) > 512 {
		return InvalidErr
	}
	if r.Direction != "" && r.Direction != "asc" && r.Direction != "desc" {
		return InvalidErr
	}
	return nil
}

// inTx is s with its database bound to tx, for work inside a transaction.
func (s *Wireless) inTx(tx *gorm.DB) *Wireless {
	return &Wireless{DB: tx, Logger: s.Logger, Cache: s.Cache, Actor: s.Actor}
}
