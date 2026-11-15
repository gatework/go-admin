package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/go-net-connector/connector"
	base "github.com/gatework/motor-core/sdk/service"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/delivery"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/paging"
)

// Workflow coordinates authorized delivery changes and their durable state.
type Workflow struct {
	base.Service
	Config Config
}

// Tables returns the persistent records required by the delivery module.
func Tables() []any {
	return []any{&models.Connection{}, &models.Order{}, &models.Target{}, &models.DeviceLease{}, &models.Receipt{}, &models.Audit{}}
}

// Get loads the selected record within the caller's authorized scope.
func (s *Workflow) Get(ctx context.Context, id int, p *actions.DataPermission) (models.Order, error) {
	var o models.Order
	e := s.scoped(ctx, p).Preload("Targets", func(db *gorm.DB) *gorm.DB { return db.Omit("output").Order("id") }).First(&o, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = NotFoundErr
	}
	decorate(&o, s.Config.Enabled)
	return o, e
}

// Target provides the target operation within the caller's authorized delivery scope.
func (s *Workflow) Target(ctx context.Context, id, target int, p *actions.DataPermission) (models.Target, error) {
	var t models.Target
	if _, e := s.Get(ctx, id, p); e != nil {
		return t, e
	}
	e := s.db(ctx).Where("id = ? AND order_id = ?", target, id).First(&t).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = NotFoundErr
	}
	t.Retryable = t.Status == "failed" && !t.Attempted
	if t.Device.Profile == "" {
		t.Device.Profile = t.Connection.Profile
	}
	return t, e
}

// Create creates the requested record within the caller's authorized scope.
func (s *Workflow) Create(ctx context.Context, r dto.Create, actor int, name string, p *actions.DataPermission) (models.Order, error) {
	return s.create(ctx, r, actor, name, p, nil)
}
func digest(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }
func validate(title string, targets []dto.InputTarget) error {
	if strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 200 || len(targets) == 0 || len(targets) > MaxTargets {
		return InvalidErr
	}
	seen := map[int]bool{}
	total := 0
	for _, t := range targets {
		if t.DeviceID <= 0 || seen[t.DeviceID] || len(t.Script) > MaxScriptBytes || !utf8.ValidString(t.Script) {
			return InvalidErr
		}
		seen[t.DeviceID] = true
		total += len(t.Script)
		script, e := connector.ParseScript(t.Script, "delivery")
		if e != nil || len(script.Commands) == 0 || total > MaxTotalBytes {
			return InvalidErr
		}
	}
	return nil
}

// Edit provides the edit operation within the caller's authorized delivery scope.
func (s *Workflow) Edit(ctx context.Context, id int, r dto.Edit, actor int, p *actions.DataPermission) (models.Order, error) {
	o, e := s.Get(ctx, id, p)
	if e != nil {
		return o, e
	}
	if e = validate(r.Title, r.Targets); e != nil {
		return o, e
	}
	if o.Kind == "fabric" || o.Status != "scheduled" || o.Version != r.Version {
		return o, ConflictErr
	}
	targets, e := s.targets(ctx, r.Targets, p)
	if e != nil {
		return o, e
	}
	due := time.Now().UTC().Add(s.Config.Grace)
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Order{}).Where("id = ? AND version = ? AND status = ?", id, r.Version, "scheduled").Updates(map[string]any{"title": r.Title, "device_count": len(targets), "scheduled_at": due, "version": gorm.Expr("version + 1")})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if e := tx.Where("order_id = ?", id).Delete(&models.Target{}).Error; e != nil {
			return e
		}
		for i := range targets {
			targets[i].OrderID = id
		}
		if e := tx.Create(&targets).Error; e != nil {
			return e
		}
		return tx.Create(&models.Audit{OrderID: id, ActorID: actor, Action: "edit"}).Error
	})
	if e != nil {
		return o, e
	}
	return s.Get(ctx, id, p)
}
func decorate(o *models.Order, enabled bool) {
	o.Editable = o.Status == "scheduled" && o.Kind != "fabric"
	o.Executable = o.Editable && enabled
	o.Cancellable = (o.Status == "scheduled" || o.Status == "queued" || o.Status == "running") && !o.CancelRequested
	o.Retryable = false
	if o.Kind == "fabric" {
		o.Cancellable = false
		return
	}
	for i := range o.Targets {
		t := &o.Targets[i]
		if t.Device.Profile == "" {
			t.Device.Profile = t.Connection.Profile
		}
		t.Retryable = t.Status == "failed" && !t.Attempted
		if t.Retryable {
			o.Retryable = enabled
		}
	}
	if o.Status == "scheduled" || o.Status == "queued" || o.Status == "running" {
		o.Retryable = false
	}
}

// Action provides the action operation within the caller's authorized delivery scope.
func (s *Workflow) Action(ctx context.Context, id int, version int64, action string, actor int, p *actions.DataPermission) (models.Order, error) {
	o, e := s.Get(ctx, id, p)
	if e != nil {
		return o, e
	}
	if o.Kind == "fabric" || o.Version != version {
		return o, ConflictErr
	}
	if action == "execute" && !s.Config.Enabled {
		return o, DisabledErr
	}
	if action == "execute" && !o.Editable || action == "cancel" && !o.Cancellable {
		return o, ConflictErr
	}
	if action != "execute" && action != "cancel" {
		return o, InvalidErr
	}
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		values := map[string]any{"version": gorm.Expr("version + 1")}
		if action == "execute" {
			values["status"] = "queued"
		} else {
			values["cancel_requested"] = true
			if o.Status != "running" {
				values["status"] = "cancelled"
				values["finished_at"] = time.Now().UTC()
			}
		}
		q := tx.Model(&models.Order{}).Where("id = ? AND version = ? AND status = ?", id, version, o.Status).Updates(values)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if action == "cancel" {
			q = tx.Model(&models.Target{}).Where("order_id = ? AND status = ?", id, "pending").Updates(map[string]any{"status": "cancelled", "finished_at": time.Now().UTC()})
			if q.Error != nil {
				return q.Error
			}
			if e := tx.Model(&models.Order{}).Where("id = ?", id).Update("cancelled_count", gorm.Expr("cancelled_count + ?", q.RowsAffected)).Error; e != nil {
				return e
			}
		}
		return tx.Create(&models.Audit{OrderID: id, ActorID: actor, Action: action}).Error
	})
	if e != nil {
		return o, e
	}
	return s.Get(ctx, id, p)
}

// Retry provides the retry operation within the caller's authorized delivery scope.
func (s *Workflow) Retry(ctx context.Context, id int, r dto.Action, actor int, name string, p *actions.DataPermission) (models.Order, error) {
	o, e := s.Get(ctx, id, p)
	if e != nil {
		return o, e
	}
	if o.Kind == "fabric" || o.Version != r.Version {
		return o, ConflictErr
	}
	if !o.Retryable {
		return o, ConflictErr
	}
	inputs := []dto.InputTarget{}
	for _, t := range o.Targets {
		if t.Retryable {
			profile := t.Connection.Profile
			inputs = append(inputs, dto.InputTarget{DeviceID: t.DeviceID, Script: t.Script, ExpectedProfile: &profile, ExpectedDeviceRole: t.Device.DeviceRole, ExpectedDeviceType: t.Device.DeviceType})
		}
	}
	return s.create(ctx, dto.Create{RequestKey: r.RequestKey, Title: o.Title, Targets: inputs}, actor, name, p, &o)
}

// Page provides the page operation within the caller's authorized delivery scope.
func (s *Workflow) Page(ctx context.Context, r dto.Page, p *actions.DataPermission) ([]models.Order, int64, error) {
	if !paging.Normalize(&r.PageIndex, &r.PageSize) {
		return nil, 0, InvalidErr
	}
	q := s.scoped(ctx, p)
	if r.Keyword != "" {
		like, pattern := " LIKE ? "+database.LikeEscape, database.LikeContains(r.Keyword)
		q = q.Where("title"+like+" OR request_no"+like, pattern, pattern)
	}
	if r.Status != "" {
		q = q.Where("status = ?", r.Status)
	}
	var count int64
	if e := q.Count(&count).Error; e != nil {
		return nil, 0, e
	}
	rows := []models.Order{}
	e := q.Order("id DESC").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error
	for i := range rows {
		decorate(&rows[i], s.Config.Enabled)
		rows[i].Retryable = rows[i].Kind != "fabric" && s.Config.Enabled && rows[i].FailedCount > 0 && rows[i].FinishedAt != nil
	}
	return rows, count, e
}

// Silence SQL for this module: SQL errors/traces can otherwise disclose scripts and output.
func (s *Workflow) db(ctx context.Context) *gorm.DB {
	return s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}
func (s *Workflow) scoped(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return s.db(ctx).Model(&models.Order{}).Scopes(actions.Permission(models.Order{}.TableName(), p))
}
func (s *Workflow) inventory(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	if p == nil {
		p = &actions.DataPermission{}
	}
	return s.db(ctx).Model(&inventory.Device{}).Scopes(actions.Permission(inventory.Device{}.TableName(), p))
}
func (s *Workflow) targets(ctx context.Context, inputs []dto.InputTarget, p *actions.DataPermission) ([]models.Target, error) {
	out := make([]models.Target, 0, len(inputs))
	for _, in := range inputs {
		var d inventory.Device
		if e := s.inventory(ctx, p).Where("id = ?", in.DeviceID).First(&d).Error; e != nil {
			return nil, NotFoundErr
		}
		if d.Status != "active" || net.ParseIP(d.IP) == nil {
			return nil, ConnectionErr
		}
		c, e := s.connection(ctx, d)
		if e != nil {
			return nil, e
		}
		if in.ExpectedProfile != nil && *in.ExpectedProfile != c.Profile || in.ExpectedDeviceRole != nil && *in.ExpectedDeviceRole != d.DeviceRole || in.ExpectedDeviceType != nil && *in.ExpectedDeviceType != d.DeviceType {
			return nil, ConflictErr
		}
		if !c.Ready {
			return nil, ConnectionErr
		}
		script, _ := connector.ParseScript(in.Script, "delivery")
		out = append(out, models.Target{DeviceID: d.ID, Device: deviceSnapshot(d, c.Profile), Connection: c, ConnectionVersion: c.Version, Script: in.Script, ScriptSHA256: digest(in.Script), ScriptLineCount: len(script.Commands), Status: "pending"})
	}
	return out, nil
}
func (s *Workflow) create(ctx context.Context, r dto.Create, actor int, name string, p *actions.DataPermission, retry *models.Order) (models.Order, error) {
	var o models.Order
	if actor <= 0 || len(r.RequestKey) < 8 || len(r.RequestKey) > 100 {
		return o, InvalidErr
	}
	if e := validate(r.Title, r.Targets); e != nil {
		return o, e
	}
	retryID, hash := creationDigest(r, retry)
	e := s.db(ctx).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&o).Error
	if e == nil {
		if o.PayloadHash != hash {
			return models.Order{}, ConflictErr
		}
		return s.Get(ctx, o.ID, p)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return o, e
	}
	if !s.Config.Enabled {
		return o, DisabledErr
	}
	retrySources, e := retrySnapshots(retry)
	if e != nil {
		return o, e
	}
	targets, e := s.targets(ctx, r.Targets, p)
	if e != nil {
		return o, e
	}
	if retry != nil {
		for _, t := range targets {
			source, ok := retrySources[t.DeviceID]
			if !ok || !matchesSnapshot(t.Device, source) {
				return o, ConflictErr
			}
		}
	}
	due := time.Now().UTC().Add(s.Config.Grace)
	o = models.Order{RequestNo: "DL-" + uuid.NewString(), RequestKey: r.RequestKey, PayloadHash: hash, Title: r.Title, Status: "scheduled", Version: 1, DeviceCount: len(targets), CreateBy: actor, CreatedByName: name, RetryOfID: retryID, ScheduledAt: &due, Targets: targets}
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&o).Error; e != nil {
			return e
		}
		return tx.Create(&models.Audit{OrderID: o.ID, ActorID: actor, Action: "create"}).Error
	})
	if e != nil {
		var existing models.Order
		if find := s.db(ctx).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&existing).Error; find == nil {
			if existing.PayloadHash != hash {
				return models.Order{}, ConflictErr
			}
			return s.Get(ctx, existing.ID, p)
		}
		return models.Order{}, e
	}
	return s.Get(ctx, o.ID, p)
}

func retrySnapshots(retry *models.Order) (map[int]models.DeviceSnapshot, error) {
	retrySources := map[int]models.DeviceSnapshot{}
	if retry != nil {
		for _, t := range retry.Targets {
			if t.Retryable {
				if t.Device.DeviceRole == nil || t.Device.DeviceType == nil || t.Device.Profile == "" {
					return nil, ConflictErr
				}
				retrySources[t.DeviceID] = t.Device
			}
		}
	}
	return retrySources, nil
}

func creationDigest(r dto.Create, retry *models.Order) (*int, string) {
	var retryID *int
	hashTargets := r.Targets
	if retry != nil {
		retryID = &retry.ID
		// Retry expectations are derived from its immutable source, not request
		// parameters. Keep the original digest format for lost-response recovery.
		hashTargets = make([]dto.InputTarget, len(r.Targets))
		for i, in := range r.Targets {
			hashTargets[i] = dto.InputTarget{DeviceID: in.DeviceID, Script: in.Script}
		}
	}
	payload, _ := json.Marshal(struct {
		Title   string
		Targets []dto.InputTarget
		Retry   *int
	}{r.Title, hashTargets, retryID})
	hash := digest(string(payload))
	return retryID, hash
}
