package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateWirelessResourceSearch)
}

// wirelessSearchRow is the wireless resource table as this migration knows it:
// its facts and the search columns it adds. The resource lists used to load
// every row and filter in memory; these columns let SQL filter and page.
type wirelessSearchRow struct {
	ID           int    `gorm:"primaryKey"`
	Facts        string `gorm:"type:text"`
	SearchIP     string `gorm:"column:search_ip;type:text"`
	SearchMAC    string `gorm:"column:search_mac;type:text"`
	SearchAP     string `gorm:"column:search_ap;type:text"`
	SearchSSID   string `gorm:"column:search_ssid;type:text"`
	SearchVLAN   string `gorm:"column:search_vlan;type:text"`
	SearchBand   string `gorm:"column:search_band;type:text"`
	SearchStatus string `gorm:"column:search_status;type:text"`
	SearchAPName string `gorm:"column:search_ap_name;size:255"`
	SearchName   string `gorm:"column:search_name;size:255"`
	SearchText   string `gorm:"column:search_text;type:text"`
}

func (wirelessSearchRow) TableName() string { return "sys_wireless_resource" }

// wirelessSearchKeys are the fact keys each filter read when this migration
// was written, frozen so that it never changes meaning.
var wirelessSearchKeys = map[string][]string{
	"search_ip":     {"ip_address", "ip", "client_ip", "client_ipv6", "management_ip"},
	"search_mac":    {"mac_address", "client_mac", "bssid"},
	"search_ap":     {"ap_name", "name", "ap_id"},
	"search_ssid":   {"ssid"},
	"search_vlan":   {"vlan", "vlan_id"},
	"search_band":   {"band"},
	"search_status": {"status", "operational_status"},
}

func migrateWirelessResourceSearch(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addWirelessResourceSearch(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// addWirelessResourceSearch adds the missing search columns and fills them
// for rows that have none yet. Numbers decoded from stored JSON print as Go
// floats; the next collection rewrites each row with the collector's values.
func addWirelessResourceSearch(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&wirelessSearchRow{}) {
		return nil
	}
	for _, column := range []string{"SearchIP", "SearchMAC", "SearchAP", "SearchSSID", "SearchVLAN", "SearchBand", "SearchStatus", "SearchAPName", "SearchName", "SearchText"} {
		if !m.HasColumn(&wirelessSearchRow{}, column) {
			if err := m.AddColumn(&wirelessSearchRow{}, column); err != nil {
				return err
			}
		}
	}
	var rows []wirelessSearchRow
	return db.Select("id", "facts").Where("search_text IS NULL OR search_text = ''").FindInBatches(&rows, 500, func(tx *gorm.DB, _ int) error {
		for _, row := range rows {
			facts := map[string]any{}
			if row.Facts != "" {
				if err := json.Unmarshal([]byte(row.Facts), &facts); err != nil {
					return fmt.Errorf("wireless resource %d facts: %w", row.ID, err)
				}
			}
			if err := tx.Model(&wirelessSearchRow{}).Where("id = ?", row.ID).Updates(wirelessSearchValues(facts)).Error; err != nil {
				return err
			}
		}
		return nil
	}).Error
}

func wirelessSearchValues(facts map[string]any) map[string]any {
	text := func(v any) string {
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	out := map[string]any{
		"search_ap_name": text(facts["ap_name"]),
		"search_name":    text(facts["name"]),
		"search_text":    strings.ToLower(text(facts)),
	}
	for column, keys := range wirelessSearchKeys {
		values := []string{}
		for _, k := range keys {
			if v := text(facts[k]); v != "" {
				values = append(values, strings.ToLower(v))
			}
		}
		out[column] = strings.Join(values, "\n")
	}
	return out
}
