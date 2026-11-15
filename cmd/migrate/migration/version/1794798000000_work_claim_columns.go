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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateWorkClaimColumns)
}

// workClaimTable is a job table whose worker claims rows with a token and an
// expiry. Those columns were named lease_token/lease_until, which read as the
// device lease they are not (see GLOSSARY.md, "Work claim").
type workClaimTable struct {
	name     string
	hasToken bool // fabric tables carry only the expiry
	indexed  bool // the expiry has GORM's default idx_<table>_<column> index
}

var workClaimTables = []workClaimTable{
	{"sys_delivery_target", true, true},
	{"sys_firewall_import", true, false},
	{"sys_firewall_analysis", true, false},
	{"sys_firewall_draft", true, false},
	{"sys_firewall_collection_target", true, true},
	{"sys_wireless_collection", true, true},
	{"sys_fabric_collection", false, true},
	{"sys_fabric_change", false, true},
}

func migrateWorkClaimColumns(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := renameWorkClaimColumns(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// renameWorkClaimColumns renames each column and index that still has its old
// name. A table built after this change by an earlier migration's AutoMigrate
// of the runtime model already has the new names and is left alone, as is a
// table that does not exist in this deployment.
func renameWorkClaimColumns(db *gorm.DB) error {
	m := db.Migrator()
	for _, table := range workClaimTables {
		if !m.HasTable(table.name) {
			continue
		}
		renames := [][2]string{{"lease_until", "claim_until"}}
		if table.hasToken {
			renames = append(renames, [2]string{"lease_token", "claim_token"})
		}
		for _, r := range renames {
			if m.HasColumn(table.name, r[0]) && !m.HasColumn(table.name, r[1]) {
				if err := m.RenameColumn(table.name, r[0], r[1]); err != nil {
					return err
				}
			}
		}
		oldIndex, newIndex := "idx_"+table.name+"_lease_until", "idx_"+table.name+"_claim_until"
		if table.indexed && m.HasIndex(table.name, oldIndex) && !m.HasIndex(table.name, newIndex) {
			if err := m.RenameIndex(table.name, oldIndex, newIndex); err != nil {
				return err
			}
		}
	}
	return nil
}
