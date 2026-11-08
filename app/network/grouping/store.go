package grouping

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Lock(tx *gorm.DB) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.DeviceGroupLock{ID: 1}).Error; err != nil {
		return err
	}
	return tx.Model(&models.DeviceGroupLock{}).Where("id = 1").Update("version", gorm.Expr("version + 1")).Error
}

type Directory []models.DeviceGroup

func Load(tx *gorm.DB) (Directory, error) {
	var groups Directory
	err := tx.Order("position, name, id").Find(&groups).Error
	return groups, err
}
func (groups Directory) Resolve(name, serial string, current *int) *int {
	rule := Region(name, serial)
	var matched *models.DeviceGroup
	// Runtime rules bind only to stable codes. A custom group's display name
	// cannot restore a deleted standard target or attract automatic members.
	for i := range groups {
		if groups[i].Code == rule.Code {
			matched = &groups[i]
			break
		}
	}
	if matched == nil || (!matched.Enabled && (current == nil || *current != matched.Id)) {
		return nil
	}
	id := matched.Id
	return &id
}

// SeedAndBackfill is migration-only. Sync deliberately never seeds defaults,
// so deleting a standard group remains effective until explicitly recreated.
// Empty source markers identify rows predating the migration; retries do not
// reinterpret a manual override, even when the legacy column is retained.
func SeedAndBackfill(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := Lock(tx); err != nil {
			return err
		}
		var rows []models.Device
		if err := tx.Unscoped().Where("group_source = '' OR classification_source = ''").Order("id").Find(&rows).Error; err != nil {
			return err
		}
		groups, err := Load(tx)
		if err != nil {
			return err
		}
		legacy := map[string]int{}
		for _, g := range groups {
			legacy[g.Name] = g.Id
		}
		for _, row := range rows {
			if row.GroupSource != "" || row.LegacyGroup == "" {
				continue
			}
			if _, ok := legacy[row.LegacyGroup]; ok {
				continue
			}
			code := legacyGroupCode(row.LegacyGroup, groups)
			group := models.DeviceGroup{Code: code, Name: row.LegacyGroup, Enabled: true, Version: 1}
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
			legacy[row.LegacyGroup] = group.Id
			groups = append(groups, group)
		}
		for position, rule := range Regions {
			exists := false
			for _, g := range groups {
				if g.Code == rule.Code || strings.EqualFold(g.Name, rule.Name) {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			group := models.DeviceGroup{Code: rule.Code, Name: rule.Name, Description: "Netdisco 自动关联区域", Enabled: true, Position: position, Version: 1}
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
			groups = append(groups, group)
		}
		for _, row := range rows {
			updates := map[string]any{}
			changed := false
			if row.GroupSource == "" {
				source := "auto"
				id := groups.Resolve(row.Name, row.Serial, row.GroupID)
				if row.LegacyGroup != "" {
					source = "manual"
					value := legacy[row.LegacyGroup]
					id = &value
				}
				updates["group_source"] = source
				updates["group_id"] = id
				changed = !SameID(row.GroupID, id)
			}
			if row.ClassificationSource == "" {
				if row.DeviceType != "" || row.DeviceRole != "" {
					updates["classification_source"] = "manual"
				} else {
					role, kind := Classification(row.Name, row.Serial)
					updates["device_role"] = role
					updates["device_type"] = kind
					updates["classification_source"] = "auto"
					changed = true
				}
			}
			if changed {
				updates["version"] = gorm.Expr("version + 1")
			}
			if err := tx.Unscoped().Model(&models.Device{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func SameID(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func legacyGroupCode(name string, groups Directory) string {
	used := map[string]bool{}
	for _, group := range groups {
		used[group.Code] = true
	}
	// A historical standard label remains a manual assignment, but the group
	// receives its stable rule identity so future renames do not break inference.
	for _, rule := range Regions {
		if strings.EqualFold(name, rule.Name) && !used[rule.Code] {
			return rule.Code
		}
	}
	digest := sha256.Sum256([]byte(name))
	base := "legacy." + hex.EncodeToString(digest[:])
	code := base
	for suffix := 1; used[code]; suffix++ {
		code = base + "." + strconv.Itoa(suffix)
	}
	return code
}
