package service

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"github.com/google/uuid"
	"go-admin/app/network"
	"go-admin/app/network/grouping"
	"go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const syncTimeout = 25 * time.Second
const syncLease = 45 * time.Second

type SyncStatus struct {
	Configured bool `json:"configured"`
	models.SyncState
}
type SyncResult struct {
	Imported int       `json:"imported"`
	Updated  int       `json:"updated"`
	Total    int       `json:"total"`
	SyncedAt time.Time `json:"syncedAt"`
}

func (s *Device) SyncStatus(ctx context.Context) (SyncStatus, error) {
	result := SyncStatus{Configured: s.source().Configured(), SyncState: models.SyncState{Status: "idle"}}
	now := time.Now().UTC()
	if err := s.Orm.WithContext(ctx).Model(&models.SyncState{}).Where("id = 1 AND status = ? AND (lease_expires_at <= ? OR lease_expires_at IS NULL)", "running", now).Updates(map[string]any{"status": "failed", "finished_at": now, "token": "", "lease_expires_at": nil, "message": "上次同步已中断，请重新同步"}).Error; err != nil {
		return result, err
	}
	err := s.Orm.WithContext(ctx).First(&result.SyncState, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	if !result.Configured {
		result.Message = network.ErrNotConfigured.Message
	}
	return result, err
}
func (s *Device) acquire(ctx context.Context) (string, error) {
	state := models.SyncState{ID: 1, Status: "idle"}
	if err := s.Orm.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return "", err
	}
	now := time.Now().UTC()
	token := uuid.NewString()
	acquired := s.Orm.WithContext(ctx).Model(&models.SyncState{}).Where("id = 1 AND (status <> ? OR lease_expires_at <= ? OR lease_expires_at IS NULL)", "running", now).Updates(map[string]any{"status": "running", "token": token, "lease_expires_at": now.Add(syncLease), "started_at": now, "finished_at": nil, "message": ""})
	if acquired.Error != nil {
		return "", acquired.Error
	}
	if acquired.RowsAffected != 1 {
		return "", network.ErrRunning
	}
	return token, nil
}
func (s *Device) fail(ctx context.Context, token string, cause error) {
	// A disconnected request must not leave a live run forever. The conditional
	// token prevents an expired writer from clearing its successor's lease.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	message := network.ErrPersistence.Message
	var safe *network.Error
	if errors.As(cause, &safe) {
		message = safe.Message
	}
	err := s.Orm.WithContext(cleanup).Model(&models.SyncState{}).Where("id = 1 AND token = ? AND status = ?", token, "running").Updates(map[string]any{"status": "failed", "finished_at": time.Now().UTC(), "token": "", "lease_expires_at": nil, "message": message}).Error
	if err != nil && s.Log != nil {
		s.Log.Error("could not persist network inventory sync failure; lease recovery remains available")
	}
}
func (s *Device) Sync(ctx context.Context, actor int, p *actions.DataPermission, admin bool) (SyncResult, error) {
	result := SyncResult{}
	if actor <= 0 {
		return result, network.ErrInvalidInput
	}
	if config.ApplicationConfig.EnableDP && !admin && (p == nil || p.DataScope != actions.DataScopeAll) {
		return result, network.ErrSyncScope
	}
	source := s.source()
	if !source.Configured() {
		return result, network.ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	token, err := s.acquire(ctx)
	if err != nil {
		return result, err
	}
	facts, err := source.Fetch(ctx)
	if err != nil {
		s.fail(ctx, token, err)
		return result, err
	}
	syncedAt := time.Now().UTC()
	err = s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		// Fence before writing; this update locks the lease row until all facts
		// and the successful state commit together.
		guard := tx.Model(&models.SyncState{}).Where("id = 1 AND token = ? AND status = ? AND lease_expires_at > ?", token, "running", syncedAt).Updates(map[string]any{"status": "succeeded", "last_success_at": syncedAt, "finished_at": syncedAt, "device_count": len(facts), "token": "", "lease_expires_at": nil, "message": ""})
		if guard.Error != nil {
			return guard.Error
		}
		if guard.RowsAffected != 1 {
			return network.ErrRunning
		}
		groups, err := grouping.Load(tx)
		if err != nil {
			return err
		}
		for start := 0; start < len(facts); start += 200 {
			end := min(start+200, len(facts))
			batch := make([]models.Device, 0, end-start)
			ips := make([]string, 0, end-start)
			for _, facts := range facts[start:end] {
				ips = append(ips, facts.IP)
			}
			var existing []models.Device
			if err := tx.Unscoped().Where("ip IN ?", ips).Find(&existing).Error; err != nil {
				return err
			}
			byIP := map[string]models.Device{}
			for _, row := range existing {
				byIP[row.IP] = row
			}
			for _, facts := range facts[start:end] {
				row := models.Device{SourceFacts: facts, LastSyncedAt: syncedAt, Version: 1, Status: "pending", Tags: []string{}, GroupSource: "auto", ClassificationSource: "auto"}
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
			assignments = append(assignments, clause.Assignment{Column: clause.Column{Name: "version"}, Value: gorm.Expr(models.Device{}.TableName() + ".version + 1")})
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "ip"}}, DoUpdates: assignments}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			err = network.ErrTimeout
		}
		s.fail(ctx, token, err)
		var safe *network.Error
		if !errors.As(err, &safe) {
			err = network.ErrPersistence
		}
		return SyncResult{}, err
	}
	result.Total = len(facts)
	result.SyncedAt = syncedAt
	return result, nil
}
