package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gatework/motor/common/worker"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor/app/net/models/grouping"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/errkind"
)

// syncTimeout bounds one sync, fetch and write together. It runs in the
// background, so it is sized for a large Netdisco, not for an HTTP client.
const syncTimeout = 5 * time.Minute

// syncLease outlives syncTimeout, so a live run never loses its claim.
const syncLease = syncTimeout + time.Minute

// SyncStatus describes sync status for network operations.
type SyncStatus struct {
	inventory.SyncState
	Configured bool `json:"configured"`
}

// SyncResult describes sync result for network operations.
type SyncResult struct {
	Imported int       `json:"imported"`
	Updated  int       `json:"updated"`
	Total    int       `json:"total"`
	SyncedAt time.Time `json:"syncedAt"`
}

// SyncStatus provides the sync status operation within the caller's authorized network scope.
func (s *Device) SyncStatus(ctx context.Context) (SyncStatus, error) {
	result := SyncStatus{Configured: s.source().Configured(), Status: "idle"}
	now := time.Now().UTC()
	if err := s.DB.WithContext(ctx).Model(&inventory.SyncState{}).Where("id = 1 AND status = ? AND (lease_expires_at <= ? OR lease_expires_at IS NULL)", "running", now).Updates(map[string]any{"status": "failed", "finished_at": now, "token": "", "lease_expires_at": nil, "message": inventory.InterruptedErr.Code()}).Error; err != nil {
		return result, err
	}
	err := s.DB.WithContext(ctx).First(&result.SyncState, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	if !result.Configured {
		result.Message = inventory.NotConfiguredErr.Code()
	}
	return result, err
}

// RequestSync queues a sync for the inventory sync worker and returns the
// state with it queued. Only one sync is queued or running at a time.
func (s *Device) RequestSync(ctx context.Context, actor int, p *actions.DataPermission, admin bool) (SyncStatus, error) {
	if actor <= 0 {
		return SyncStatus{}, inventory.InvalidErr
	}
	if !admin && !actions.FullScope(p) {
		return SyncStatus{}, inventory.SyncScopeErr
	}
	if !s.source().Configured() {
		return SyncStatus{}, inventory.NotConfiguredErr
	}
	if err := s.ensureState(ctx); err != nil {
		return SyncStatus{}, err
	}
	now := time.Now().UTC()
	queued := s.DB.WithContext(ctx).Model(&inventory.SyncState{}).
		Where("id = 1 AND (status NOT IN ? OR (status = ? AND (lease_expires_at <= ? OR lease_expires_at IS NULL)))", []string{"queued", "running"}, "running", now).
		Updates(map[string]any{"status": "queued", "requested_by": actor, "token": "", "lease_expires_at": nil, "message": ""})
	if queued.Error != nil {
		return SyncStatus{}, queued.Error
	}
	if queued.RowsAffected != 1 {
		return SyncStatus{}, inventory.RunningErr
	}
	return s.SyncStatus(ctx)
}

// RunQueuedSync claims a queued sync and runs it: fetch from Netdisco, then
// write every device in one transaction. ran is false when nothing was queued.
func (s *Device) RunQueuedSync(ctx context.Context) (result SyncResult, ran bool, err error) {
	token, actor, err := s.claim(ctx)
	if err != nil || token == "" {
		return result, false, err
	}
	result, err = s.run(ctx, token, actor)
	return result, true, err
}

func (s *Device) run(ctx context.Context, token string, actor int) (SyncResult, error) {
	result := SyncResult{}
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	facts, err := s.source().Fetch(ctx)
	if err != nil {
		if ctx.Err() != nil {
			err = inventory.TimeoutErr
		}
		s.fail(ctx, token, err)
		return result, err
	}
	syncedAt := time.Now().UTC()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		// Fence before writing; this update locks the state row until all facts
		// and the successful state commit together.
		guard := tx.Model(&inventory.SyncState{}).Where("id = 1 AND token = ? AND status = ? AND lease_expires_at > ?", token, "running", syncedAt).Updates(map[string]any{"status": "succeeded", "last_success_at": syncedAt, "finished_at": syncedAt, "device_count": len(facts), "token": "", "lease_expires_at": nil, "message": ""})
		if guard.Error != nil {
			return guard.Error
		}
		if guard.RowsAffected != 1 {
			return inventory.RunningErr
		}
		groups, err := grouping.Load(tx)
		if err != nil {
			return err
		}
		if err := syncDeviceBatches(tx, facts, groups, syncedAt, actor, &result); err != nil {
			return err
		}
		return tx.Model(&inventory.SyncState{}).Where("id = 1").Updates(map[string]any{"imported": result.Imported, "updated": result.Updated}).Error
	})
	if err != nil {
		if ctx.Err() != nil {
			err = inventory.TimeoutErr
		}
		s.fail(ctx, token, err)
		if kind, _ := errkind.Of(err); kind == errkind.Unknown {
			err = fmt.Errorf("%w: %w", inventory.PersistenceErr, err)
		}
		return SyncResult{}, err
	}
	result.Total = len(facts)
	result.SyncedAt = syncedAt
	return result, nil
}

func (s *Device) ensureState(ctx context.Context) error {
	state := inventory.SyncState{ID: 1, Status: "idle"}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error
}

// claim moves a queued sync to running under a fresh token and returns the
// token and the user who asked for it; an empty token means nothing was queued.
func (s *Device) claim(ctx context.Context) (string, int, error) {
	now := time.Now().UTC()
	token := uuid.NewString()
	claimed := s.DB.WithContext(ctx).Model(&inventory.SyncState{}).Where("id = 1 AND status = ?", "queued").Updates(map[string]any{"status": "running", "token": token, "lease_expires_at": now.Add(syncLease), "started_at": now, "finished_at": nil, "message": "", "imported": 0, "updated": 0})
	if claimed.Error != nil || claimed.RowsAffected != 1 {
		return "", 0, claimed.Error
	}
	var state inventory.SyncState
	if err := s.DB.WithContext(ctx).Select("requested_by").First(&state, 1).Error; err != nil {
		return "", 0, err
	}
	return token, state.RequestedBy, nil
}

func (s *Device) fail(ctx context.Context, token string, cause error) {
	// A cancelled run must not leave a live claim forever. The conditional
	// token prevents an expired writer from clearing its successor's lease.
	cleanup, cancel := worker.Cleanup(ctx, 3*time.Second)
	defer cancel()
	message := inventory.PersistenceErr.Code()
	if _, code := errkind.Of(cause); code != "" {
		message = code
	}

	err := s.DB.WithContext(cleanup).Model(&inventory.SyncState{}).Where("id = 1 AND token = ? AND status = ?", token, "running").Updates(map[string]any{"status": "failed", "finished_at": time.Now().UTC(), "token": "", "lease_expires_at": nil, "message": message}).Error
	if err != nil && s.Logger != nil {
		s.Logger.Error("could not persist network inventory sync failure; lease recovery remains available")
	}
}

func syncDeviceBatches(tx *gorm.DB, facts []inventory.SourceFacts, groups grouping.Directory, syncedAt time.Time, actor int, result *SyncResult) error {
	for start := 0; start < len(facts); start += 200 {
		end := min(start+200, len(facts))
		batch := make([]inventory.Device, 0, end-start)
		ips := make([]string, 0, end-start)
		for _, facts := range facts[start:end] {
			ips = append(ips, facts.IP)
		}
		var existing []inventory.Device
		if err := tx.Unscoped().Where("ip IN ?", ips).Find(&existing).Error; err != nil {
			return err
		}
		byIP := map[string]inventory.Device{}
		for _, row := range existing {
			byIP[row.IP] = row
		}
		for _, facts := range facts[start:end] {
			row := inventory.Device{SourceFacts: facts, LastSyncedAt: syncedAt, Version: 1, Status: "pending", Tags: []string{}, GroupSource: "auto", ClassificationSource: "auto"}
			if current, exists := byIP[facts.IP]; exists {
				row.GroupID = current.GroupID
				row.GroupSource = current.GroupSource
				row.DeviceType = current.DeviceType
				row.DeviceRole = current.DeviceRole
				row.ClassificationSource = current.ClassificationSource
				result.Updated++
			} else {
				result.Imported++
			}
			if row.GroupSource == "auto" {
				row.GroupID = groups.Resolve(facts.Name, facts.Serial, row.GroupID)
			}
			if row.ClassificationSource == "auto" {
				row.DeviceRole, row.DeviceType = grouping.Classification(facts.Name, facts.Serial)
			}
			row.SetCreateBy(actor)
			row.SetUpdateBy(actor)
			batch = append(batch, row)
		}
		assignments := clause.AssignmentColumns([]string{"name", "hostname", "serial", "vendor", "model", "os", "os_version", "location", "contact", "description", "mac_address", "source_tags", "source_created_at", "last_discovered_at", "last_synced_at", "updated_at"})
		assignments = append(assignments, clause.AssignmentColumns([]string{"group_id", "group_source", "classification_source", "device_type", "device_role"})...)
		assignments = append(assignments, clause.Assignment{Column: clause.Column{Name: "version"}, Value: gorm.Expr(inventory.Device{}.TableName() + ".version + 1")})
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "ip"}}, DoUpdates: assignments}).Create(&batch).Error; err != nil {
			return err
		}
	}
	return nil
}
