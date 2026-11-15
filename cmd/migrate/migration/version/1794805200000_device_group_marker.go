package version

import (
	"runtime"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateDeviceGroupMarker)
}

// deviceGroupMarkerRow is the device group table as this migration knows it:
// the marker column it adds. Sync used to match device names against a list
// compiled into the binary; the marker moves that rule onto the group.
type deviceGroupMarkerRow struct {
	Marker string `gorm:"size:32;not null;default:''"`
}

func (deviceGroupMarkerRow) TableName() string { return "sys_network_device_group" }

// standardGroupMarkers is the compiled list as it stood when the marker moved
// to the database, frozen here so this migration never changes meaning.
var standardGroupMarkers = map[string]string{
	"site.transsion-tower":      "szx9",
	"site.shenzhen-yun-tech":    "szx10",
	"site.shenzhen-baiwangxin":  "szx8",
	"site.shenzhen-chuanyiyuan": "szx11",
	"site.shanghai":             "sha",
	"site.chongqing":            "ckg",
}

func migrateDeviceGroupMarker(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addDeviceGroupMarker(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// addDeviceGroupMarker adds the column when it is missing (a database created
// after this change already has it) and gives each standard group without a
// marker the one it was matched by.
func addDeviceGroupMarker(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&deviceGroupMarkerRow{}) {
		return nil
	}
	if !m.HasColumn(&deviceGroupMarkerRow{}, "marker") {
		if err := m.AddColumn(&deviceGroupMarkerRow{}, "Marker"); err != nil {
			return err
		}
	}
	for code, marker := range standardGroupMarkers {
		if err := db.Model(&deviceGroupMarkerRow{}).Where("code = ? AND marker = ''", code).Update("marker", marker).Error; err != nil {
			return err
		}
	}
	return nil
}
