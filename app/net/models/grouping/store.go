package grouping

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/txlock"
)

// Lock provides the lock operation within the caller's authorized network scope.
func Lock(tx *gorm.DB) error {
	return txlock.Hold(tx, txlock.DeviceGroups)
}

// Directory describes directory for network operations.
type Directory []models.DeviceGroup

// Load loads persisted  state.
func Load(tx *gorm.DB) (Directory, error) {
	var groups Directory
	err := tx.Order("position, name, id").Find(&groups).Error
	return groups, err
}

// Resolve returns the group a device is assigned to automatically: the first
// group, in the directory's position order, whose marker the device's name
// (or serial, when nameless) contains; else the FallbackCode group. A
// disabled group takes no new members but keeps current, the device's group
// now. Nil means ungrouped.
func (groups Directory) Resolve(name, serial string, current *int) *int {
	value := strings.ToLower(identity(name, serial))
	var matched *models.DeviceGroup
	for i := range groups {
		if marker := strings.ToLower(groups[i].Marker); marker != "" && strings.Contains(value, marker) {
			matched = &groups[i]
			break
		}
	}
	if matched == nil {
		for i := range groups {
			if groups[i].Code == FallbackCode {
				matched = &groups[i]
				break
			}
		}
	}
	if matched == nil || (!matched.Enabled && (current == nil || *current != matched.ID)) {
		return nil
	}
	id := matched.ID
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
			legacy[g.Name] = g.ID
		}
		groups, err = seedLegacyGroups(tx, rows, groups, legacy)
		if err != nil {
			return err
		}
		for position, rule := range standardSites {
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
			group := models.DeviceGroup{Code: rule.Code, Name: rule.Name, Description: "Netdisco 自动关联区域", Marker: rule.Marker, Enabled: true, Position: position, Version: 1}
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
			groups = append(groups, group)
		}
		return backfillDevices(tx, rows, groups, legacy)
	})
}

// SameID provides the same i d operation within the caller's authorized network scope.
func SameID(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func legacyGroupCode(name string, groups Directory) string {
	used := map[string]bool{}
	for _, group := range groups {
		used[group.Code] = true
	}
	// A historical standard label remains a manual assignment, but the group
	// receives its stable rule identity so future renames do not break inference.
	for _, rule := range standardSites {
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

// standardMarker is the seeded marker of a standard site code, or "".
func standardMarker(code string) string {
	for _, site := range standardSites {
		if site.Code == code {
			return site.Marker
		}
	}
	return ""
}

func seedLegacyGroups(tx *gorm.DB, rows []models.Device, groups Directory, legacy map[string]int) (Directory, error) {
	for _, row := range rows {
		if row.GroupSource != "" || row.LegacyGroup == "" {
			continue
		}
		if _, ok := legacy[row.LegacyGroup]; ok {
			continue
		}
		code := legacyGroupCode(row.LegacyGroup, groups)
		group := models.DeviceGroup{Code: code, Name: row.LegacyGroup, Marker: standardMarker(code), Enabled: true, Version: 1}
		if err := tx.Create(&group).Error; err != nil {
			return groups, err
		}
		legacy[row.LegacyGroup] = group.ID
		groups = append(groups, group)
	}
	return groups, nil
}

func backfillDevices(tx *gorm.DB, rows []models.Device, groups Directory, legacy map[string]int) error {
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
		if err := tx.Unscoped().Model(&models.Device{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
