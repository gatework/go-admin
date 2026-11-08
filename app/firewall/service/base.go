package service

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

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/vendors"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/storage"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
)

var (
	ErrInvalid     = errors.New("firewall.invalid_input")
	ErrNotFound    = errors.New("firewall.not_found")
	ErrConflict    = errors.New("firewall.version_conflict")
	ErrUnavailable = errors.New("firewall.evidence_unavailable")
)

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
func (s *Firewall) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	if s.Orm == nil {
		return "", ErrUnavailable
	}
	pool, err := s.Orm.DB()
	if err != nil {
		return "", ErrUnavailable
	}
	if v, ok := roots.Load(pool); ok {
		return v.(string), nil
	}
	return "", ErrUnavailable
}
func (s *Firewall) store() (*storage.Store, error) {
	root, e := s.root()
	if e != nil {
		return nil, e
	}
	return storage.New(s.Orm, root)
}
func (s *Firewall) db(ctx context.Context) *gorm.DB { return s.Orm.WithContext(ctx) }
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
		return ErrNotFound
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

type Page struct {
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Keyword   string `form:"keyword"`
	GroupID   *int   `form:"groupId"`
	DomainID  int    `form:"domainId"`
}

func (p *Page) Validate() error {
	if p.PageIndex == 0 {
		p.PageIndex = 1
	}
	if p.PageSize == 0 {
		p.PageSize = 20
	}
	if p.PageIndex < 1 || p.PageSize < 1 || p.PageSize > 200 || p.PageIndex > 100000 || len(p.Keyword) > 512 || p.DomainID < 0 || p.GroupID != nil && *p.GroupID < 0 {
		return ErrInvalid
	}
	return nil
}
func pageQuery(db *gorm.DB, p Page) *gorm.DB {
	return db.Offset((p.PageIndex - 1) * p.PageSize).Limit(p.PageSize)
}
func escapeLike(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(strings.TrimSpace(s)))
}
func keyword(db *gorm.DB, column, value string) *gorm.DB {
	if value != "" {
		return db.Where("LOWER("+column+") LIKE ? ESCAPE '!'", "%"+escapeLike(value)+"%")
	}
	return db
}

type InstanceInput struct {
	DeviceID      int    `json:"deviceId"`
	DomainID      int    `json:"domainId"`
	Vendor        string `json:"vendor"`
	NativeContext string `json:"nativeContext"`
	Mode          string `json:"mode"`
}

func (s *Firewall) Instance(ctx context.Context, id int, p *actions.DataPermission) (models.Instance, error) {
	var row models.Instance
	if id <= 0 {
		return row, ErrInvalid
	}
	e := scopedInstances(s.db(ctx), p).First(&row, id).Error
	return row, safeError(e)
}
func (s *Firewall) CreateInstance(ctx context.Context, in InstanceInput, p *actions.DataPermission) (models.Instance, error) {
	var out models.Instance
	if s.Actor <= 0 || in.DeviceID <= 0 || in.DomainID <= 0 || len(in.NativeContext) > 128 || (in.Mode != "routed" && in.Mode != "transparent") {
		return out, ErrInvalid
	}
	var d network.Device
	var e error
	if e = deviceScope(s.db(ctx), p).First(&d, in.DeviceID).Error; e != nil {
		return out, safeError(e)
	}
	if d.DeviceType != "firewall" && d.DeviceRole != "firewall" {
		return out, ErrInvalid
	}
	if strings.TrimSpace(in.Vendor) == "" {
		in.Vendor = inferFirewallVendor(d)
	}
	meta, e := vendors.Normalize(fm.Metadata{Vendor: strings.ToLower(strings.TrimSpace(in.Vendor)), Context: strings.TrimSpace(in.NativeContext)})
	if e != nil {
		return out, ErrInvalid
	}
	if e = s.domain(ctx, in.DomainID); e != nil {
		return out, e
	}
	out = models.Instance{DeviceID: d.Id, Name: d.Name, Vendor: meta.Vendor, NativeContext: meta.Context, Mode: in.Mode, DomainID: in.DomainID, Version: 1, SettingsVersion: 1}
	out.Settings.Version = 1
	if out.Name == "" {
		out.Name = d.IP
	}
	if e = s.db(ctx).Create(&out).Error; e != nil {
		return models.Instance{}, ErrConflict
	}
	return out, nil
}
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
					if g.ParentID != nil && *g.ParentID == ids[i] && !seen[g.Id] {
						seen[g.Id] = true
						ids = append(ids, g.Id)
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
func inDomain(q, db *gorm.DB, domainID int) *gorm.DB {
	return q.Where("domain_id = ? OR id IN (?) OR id IN (?)", domainID,
		db.Model(&models.ScopeBinding{}).Select("instance_id").Where("domain_id = ?", domainID),
		db.Model(&models.ZoneBinding{}).Select("instance_id").Where("domain_id = ?", domainID))
}
func pValue(p *Page) Page { return *p }
func (s *Firewall) domain(ctx context.Context, id int) error {
	var d models.Domain
	if id <= 0 {
		return ErrInvalid
	}
	if e := s.db(ctx).First(&d, id).Error; e != nil {
		return safeError(e)
	}
	return nil
}
func (s *Firewall) Domains(ctx context.Context) ([]models.Domain, error) {
	out := []models.Domain{}
	e := s.db(ctx).Order("id").Find(&out).Error
	return out, e
}

type DomainInput struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

func (s *Firewall) SaveDomain(ctx context.Context, id int, in DomainInput) (models.Domain, error) {
	var d models.Domain
	if !validKey(in.Code) || len(in.Code) > 64 || !validTitle(in.Name) || utf8.RuneCountInString(in.Name) > 128 {
		return d, ErrInvalid
	}
	if id == 0 {
		d = models.Domain{Code: in.Code, Name: in.Name, Version: 1}
		e := s.db(ctx).Create(&d).Error
		if e != nil {
			return d, ErrConflict
		}
		return d, nil
	}
	q := s.db(ctx).Model(&models.Domain{}).Where("id = ? AND version = ?", id, in.Version).Updates(map[string]any{"name": in.Name, "code": in.Code, "version": gorm.Expr("version + 1")})
	if q.Error != nil {
		return d, q.Error
	}
	if q.RowsAffected != 1 {
		return d, ErrConflict
	}
	return d, s.db(ctx).First(&d, id).Error
}
