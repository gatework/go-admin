package version

import (
	"errors"
	"runtime"
	"strconv"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration/models"
	common "github.com/gatework/motor/common/models"

	"github.com/gatework/motor/cmd/migrate/migration"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, fileName, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(fileName), _1653638869132Test)
}

func _1653638869132Test(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var list []models.SysMenu
		err := tx.Model(&models.SysMenu{}).Order("parent_id,menu_id").Find(&list).Error
		if err != nil {
			return err
		}
		for _, v := range list {
			if v.ParentID == 0 {
				v.Paths = "/0/" + strconv.Itoa(v.MenuID)
			} else {
				var e models.SysMenu
				err = tx.Model(&models.SysMenu{}).Where("menu_id=?", v.ParentID).First(&e).Error
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					return err
				}
				v.Paths = e.Paths + "/" + strconv.Itoa(v.MenuID)
			}
			err = tx.Model(&v).Update("paths", v.Paths).Error
			if err != nil {
				return err
			}
		}
		return tx.Create(&common.Migration{
			Version: version,
		}).Error
	})
}
