// Package connection owns shared, versioned connection parameters. Callers
// decide vendor capability and admission; credentials never enter these rows.
package connection

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/actions"
)

// InvalidErr identifies invalid failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var InvalidErr = errors.New("connection.invalid_input")

//revive:enable:error-naming

// ConflictErr identifies conflict failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var ConflictErr = errors.New("connection.version_conflict")

//revive:enable:error-naming

// NotFoundErr identifies not found failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var NotFoundErr = errors.New("connection.not_found")

//revive:enable:error-naming

// Settings stores per-device transport configuration with optimistic versioning.
type Settings struct {
	DeviceID        int       `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Profile         string    `gorm:"size:32;not null" json:"profile"`
	Protocol        string    `gorm:"size:10;not null" json:"protocol"`
	Port            int       `json:"port"`
	Version         int64     `json:"version"`
	UpdatedAt       time.Time `json:"-"`
	UpdateBy        int       `json:"-"`
	Ready           bool      `gorm:"-" json:"ready"`
	Reason          string    `gorm:"-" json:"reason"`
	ProfileSource   string    `gorm:"-" json:"profileSource"`
	InferredProfile string    `gorm:"-" json:"inferredProfile"`
}

// TableName returns the device access settings table.
// TableName returns the stable database table for Settings.
func (Settings) TableName() string { return "sys_delivery_connection" }

// Input describes input for network operations.
type Input struct {
	Version  int64  `json:"version"`
	Profile  string `json:"profile"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// Load loads persisted  state.
func Load(ctx context.Context, db *gorm.DB, id int) (Settings, error) {
	out := Settings{DeviceID: id, Protocol: "ssh", Port: 22}
	err := db.WithContext(ctx).First(&out, "device_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return out, err
}

// Save persists validated  state.
func Save(ctx context.Context, db *gorm.DB, id int, in Input, actor int, permission *actions.DataPermission) error {
	if id <= 0 || actor <= 0 || in.Version < 0 || in.Port < 1 || in.Port > 65535 || (in.Protocol != "ssh" && in.Protocol != "telnet") || len(in.Profile) > 32 || !utf8.ValidString(in.Profile) || strings.ContainsAny(in.Profile, "\x00\r\n") {
		return InvalidErr
	}
	if permission == nil {
		permission = &actions.DataPermission{}
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device models.Device
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Scopes(actions.Permission(device.TableName(), permission)).First(&device, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return NotFoundErr
			}
			return err
		}
		current, err := Load(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return ConflictErr
		}
		if in.Version == 0 {
			return tx.Create(&Settings{DeviceID: id, Profile: in.Profile, Protocol: in.Protocol, Port: in.Port, Version: 1, UpdateBy: actor}).Error
		}
		q := tx.Model(&Settings{}).Where("device_id = ? AND version = ?", id, in.Version).Updates(map[string]any{"profile": in.Profile, "protocol": in.Protocol, "port": in.Port, "version": gorm.Expr("version + 1"), "update_by": actor})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		return nil
	})
}
