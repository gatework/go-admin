package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gatework/motor/common/errkind"

	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/vendors"
	base "github.com/gatework/motor-core/sdk/service"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/paging"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr reports invalid.
	InvalidErr = errkind.New(errkind.Invalid, "firewall.invalid_input")
	// NotFoundErr describes the err not found used by this module.
	NotFoundErr = errkind.New(errkind.NotFound, "firewall.not_found")
	// ConflictErr reports a concurrent firewall update.
	ConflictErr = errkind.New(errkind.Conflict, "firewall.version_conflict")
	// UnavailableErr reports missing firewall evidence.
	UnavailableErr = errkind.New(errkind.Unavailable, "firewall.evidence_unavailable")
)

//revive:enable:error-naming

// Firewall carries firewall state for this module.
type Firewall struct {
	base.Service
	Actor int
	Root  string
}

var roots sync.Map

// RegisterRoot associates an existing tenant database with a stable private store.
func RegisterRoot(db *gorm.DB, tenant string) string {
	root := os.Getenv("FIREWALL_SNAPSHOT_DIR")
	if root == "" {
		root = filepath.Join("data", "firewall")
	}
	sum := sha256.Sum256([]byte(tenant))
	root = filepath.Join(root, hex.EncodeToString(sum[:16]))
	// Request middleware uses WithContext, producing a different *gorm.DB per
	// request. Its underlying tenant connection pool remains stable.
	if pool, err := db.DB(); err == nil {
		roots.Store(pool, root)
	}
	return root
}

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
		return models.Instance{}, ConflictErr
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
		devices := deviceScope(s.db(ctx), permission)
		if *p.GroupID == 0 {
			devices = devices.Where("group_id IS NULL")
		} else {
			var groups []network.DeviceGroup
			if e := s.db(ctx).Find(&groups).Error; e != nil {
				return rows, 0, e
			}
			ids := []int{*p.GroupID}
			seen := map[int]bool{*p.GroupID: true}
			for i := 0; i < len(ids); i++ {
				for _, g := range groups {
					if g.ParentID != nil && *g.ParentID == ids[i] && !seen[g.ID] {
						seen[g.ID] = true
						ids = append(ids, g.ID)
					}
				}
			}
			devices = devices.Where("group_id IN ?", ids)
		}
		q = q.Where("device_id IN (?)", devices.Select("id"))
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
func safeError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return NotFoundErr
	}
	return e
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func validKey(s string) bool {
	return len(s) > 0 && len(s) <= 128 && regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(s)
}
func validTitle(s string) bool { return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 255 }

// Page carries page state for this module.
type Page struct {
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Keyword   string `form:"keyword"`
	GroupID   *int   `form:"groupId"`
	DomainID  int    `form:"domainId"`
}

// Validate provides validate for this module.
func (p *Page) Validate() error {
	if !paging.Normalize(&p.PageIndex, &p.PageSize) || len(p.Keyword) > 512 || p.DomainID < 0 || p.GroupID != nil && *p.GroupID < 0 {
		return InvalidErr
	}
	return nil
}
func pageQuery(db *gorm.DB, p Page) *gorm.DB {
	return db.Offset((p.PageIndex - 1) * p.PageSize).Limit(p.PageSize)
}
func keyword(db *gorm.DB, column, value string) *gorm.DB {
	if value != "" {
		return db.Where("LOWER("+column+") LIKE ? "+database.LikeEscape, database.LikeContains(strings.ToLower(strings.TrimSpace(value))))
	}
	return db
}

// InstanceInput carries instance input state for this module.
type InstanceInput struct {
	DeviceID      int    `json:"deviceId"`
	DomainID      int    `json:"domainId"`
	Vendor        string `json:"vendor"`
	NativeContext string `json:"nativeContext"`
	Mode          string `json:"mode"`
}

// Domains provides domains for this module.
func (s *Firewall) Domains(ctx context.Context) ([]models.Domain, error) {
	out := []models.Domain{}
	e := s.db(ctx).Order("id").Find(&out).Error
	return out, e
}

// SaveDomain provides save domain for this module.
func (s *Firewall) SaveDomain(ctx context.Context, id int, in DomainInput) (models.Domain, error) {
	var d models.Domain
	if !validKey(in.Code) || len(in.Code) > 64 || !validTitle(in.Name) || utf8.RuneCountInString(in.Name) > 128 {
		return d, InvalidErr
	}
	if id == 0 {
		d = models.Domain{Code: in.Code, Name: in.Name, Version: 1}
		e := s.db(ctx).Create(&d).Error
		if e != nil {
			return d, ConflictErr
		}
		return d, nil
	}
	q := s.db(ctx).Model(&models.Domain{}).Where("id = ? AND version = ?", id, in.Version).Updates(map[string]any{"name": in.Name, "code": in.Code, "version": gorm.Expr("version + 1")})
	if q.Error != nil {
		return d, q.Error
	}
	if q.RowsAffected != 1 {
		return d, ConflictErr
	}
	return d, s.db(ctx).First(&d, id).Error
}

func (s *Firewall) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	if s.DB == nil {
		return "", UnavailableErr
	}
	pool, err := s.DB.DB()
	if err != nil {
		return "", UnavailableErr
	}
	if v, ok := roots.Load(pool); ok {
		root, ok := v.(string)
		if !ok {
			return "", UnavailableErr
		}
		return root, nil
	}
	return "", UnavailableErr
}
func inDomain(q, db *gorm.DB, domainID int) *gorm.DB {
	return q.Where("domain_id = ? OR id IN (?) OR id IN (?)", domainID,
		db.Model(&models.ScopeBinding{}).Select("instance_id").Where("domain_id = ?", domainID),
		db.Model(&models.ZoneBinding{}).Select("instance_id").Where("domain_id = ?", domainID))
}
func pValue(p *Page) Page { return *p }
func (s *Firewall) store() (*storage.Store, error) {
	root, e := s.root()
	if e != nil {
		return nil, e
	}
	return storage.New(s.DB, root)
}

func (s *Firewall) db(ctx context.Context) *gorm.DB { return s.DB.WithContext(ctx) }

// DomainInput carries domain input state for this module.
type DomainInput struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

func (s *Firewall) domain(ctx context.Context, id int) error {
	var d models.Domain
	if id <= 0 {
		return InvalidErr
	}
	if e := s.db(ctx).First(&d, id).Error; e != nil {
		return safeError(e)
	}
	return nil
}
