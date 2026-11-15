package tools

import (
	"fmt"
	"strings"

	common "github.com/gatework/motor/common/models"

	"gorm.io/gorm"
)

// SysTables stores sys tables data in the database.
type SysTables struct {
	common.ModelTime
	common.ControlBy
	TableID             int          `gorm:"primaryKey;autoIncrement" json:"tableId"`        //表编码
	TBName              string       `gorm:"column:table_name;size:255;" json:"tableName"`   //表名称
	MLTBName            string       `gorm:"-" json:"-"`                                     //表名称
	TableComment        string       `gorm:"size:255;" json:"tableComment"`                  //表备注
	ClassName           string       `gorm:"size:255;" json:"className"`                     //类名
	TplCategory         string       `gorm:"size:255;" json:"tplCategory"`                   //
	PackageName         string       `gorm:"size:255;" json:"packageName"`                   //包名
	ModuleName          string       `gorm:"size:255;" json:"moduleName"`                    //go文件名
	ModuleFrontName     string       `gorm:"size:255;comment:前端文件名;" json:"moduleFrontName"` //前端文件名
	BusinessName        string       `gorm:"size:255;" json:"businessName"`                  //
	FunctionName        string       `gorm:"size:255;" json:"functionName"`                  //功能名称
	FunctionAuthor      string       `gorm:"size:255;" json:"functionAuthor"`                //功能作者
	PKColumn            string       `gorm:"size:255;" json:"pkColumn"`
	PKGoField           string       `gorm:"size:255;" json:"pkGoField"`
	PKJSONField         string       `gorm:"size:255;" json:"pkJsonField"`
	Options             string       `gorm:"size:255;" json:"options"`
	TreeCode            string       `gorm:"size:255;" json:"treeCode"`
	TreeParentCode      string       `gorm:"size:255;" json:"treeParentCode"`
	TreeName            string       `gorm:"size:255;" json:"treeName"`
	Tree                bool         `gorm:"size:1;default:0;" json:"tree"`
	Crud                bool         `gorm:"size:1;default:1;" json:"crud"`
	Remark              string       `gorm:"size:255;" json:"remark"`
	IsDataScope         int          `gorm:"size:1;" json:"isDataScope"`
	IsActions           int          `gorm:"size:1;" json:"isActions"`
	IsAuth              int          `gorm:"size:1;" json:"isAuth"`
	IsLogicalDelete     string       `gorm:"size:1;" json:"isLogicalDelete"`
	LogicalDelete       bool         `gorm:"size:1;" json:"logicalDelete"`
	LogicalDeleteColumn string       `gorm:"size:128;" json:"logicalDeleteColumn"`
	DataScope           string       `gorm:"-" json:"dataScope"`
	Params              Params       `gorm:"-" json:"params"`
	Columns             []SysColumns `gorm:"-" json:"columns"`
}

// TableName returns the database table used for this record.
func (*SysTables) TableName() string {
	return "sys_tables"
}

// Params stores params data in the database.
type Params struct {
	TreeCode       string `gorm:"-" json:"treeCode"`
	TreeParentCode string `gorm:"-" json:"treeParentCode"`
	TreeName       string `gorm:"-" json:"treeName"`
}

// GetPage stores get page data in the database.
func (e *SysTables) GetPage(tx *gorm.DB, pageSize int, pageIndex int) ([]SysTables, int, error) {
	var doc []SysTables

	table := tx.Table("sys_tables")

	if e.TBName != "" {
		table = table.Where("table_name = ?", e.TBName)
	}
	if e.TableComment != "" {
		table = table.Where("table_comment = ?", e.TableComment)
	}

	var count int64

	if err := table.Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&doc).Offset(-1).Limit(-1).Count(&count).Error; err != nil {
		return nil, 0, err
	}
	//table.Where("`deleted_at` IS NULL").Count(&count)
	return doc, int(count), nil
}

// Get stores get data in the database.
func (e *SysTables) Get(tx *gorm.DB, exclude bool) (SysTables, error) {
	var doc SysTables
	var err error
	table := tx.Table("sys_tables")

	if e.TBName != "" {
		table = table.Where("table_name = ?", e.TBName)
	}
	if e.TableID != 0 {
		table = table.Where("table_id = ?", e.TableID)
	}
	if e.TableComment != "" {
		table = table.Where("table_comment = ?", e.TableComment)
	}

	if err := table.First(&doc).Error; err != nil {
		return doc, err
	}
	var col SysColumns
	col.TableID = doc.TableID
	if doc.Columns, err = col.GetList(tx, exclude); err != nil {
		return doc, err
	}

	return doc, nil
}

// GetTree stores get tree data in the database.
func (e *SysTables) GetTree(tx *gorm.DB) ([]SysTables, error) {
	var doc []SysTables
	var err error
	table := tx.Table("sys_tables")

	if e.TBName != "" {
		table = table.Where("table_name = ?", e.TBName)
	}
	if e.TableID != 0 {
		table = table.Where("table_id = ?", e.TableID)
	}
	if e.TableComment != "" {
		table = table.Where("table_comment = ?", e.TableComment)
	}

	if err := table.Find(&doc).Error; err != nil {
		return doc, err
	}
	for i := range doc {
		var col SysColumns
		//col.FkCol = append(col.FkCol, SysColumns{ColumnId: 0, ColumnName: "请选择"})
		col.TableID = doc[i].TableID
		if doc[i].Columns, err = col.GetList(tx, false); err != nil {
			return doc, err
		}

	}

	return doc, nil
}

// Create inserts the table and every one of its columns, or none of them. A
// column that failed to insert used to be dropped without a word, leaving a
// table the config page lists with columns missing and code generated from
// fewer columns than the table has.
func (e *SysTables) Create(tx *gorm.DB) (SysTables, error) {
	var doc SysTables
	e.CreateBy = 0
	err := tx.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("sys_tables").Create(&e).Error; err != nil {
			return err
		}
		for i := range e.Columns {
			e.Columns[i].TableID = e.TableID
			if _, err := e.Columns[i].Create(tx); err != nil {
				return fmt.Errorf("column %s: %w", e.Columns[i].ColumnName, err)
			}
		}
		return nil
	})
	if err != nil {
		return doc, err
	}
	return *e, nil
}

// Update stores update data in the database.
func (e *SysTables) Update(tx *gorm.DB) (update SysTables, err error) {
	//if err = orm.Eloquent.Table("sys_tables").First(&update, e.TableId).Error; err != nil {
	//	return
	//}

	//参数1:是要修改的数据
	//参数2:是修改的数据
	e.UpdateBy = 0
	// One transaction for the table and its columns, as in Create: a column
	// whose write failed used to be skipped while the request reported
	// success, so the config page showed a save that had not happened.
	err = tx.Transaction(func(tx *gorm.DB) error {
		return e.updateWithColumns(tx)
	})
	return
}

// Delete stores delete data in the database.
func (e *SysTables) Delete(db *gorm.DB) (success bool, err error) {
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	if err = tx.Table("sys_tables").Delete(SysTables{}, "table_id = ?", e.TableID).Error; err != nil {
		success = false
		return
	}
	if err = tx.Table("sys_columns").Delete(SysColumns{}, "table_id = ?", e.TableID).Error; err != nil {
		success = false
		return
	}
	success = true
	return
}

// BatchDelete stores batch delete data in the database.
func (e *SysTables) BatchDelete(tx *gorm.DB, id []int) (Result bool, err error) {
	if err = tx.Unscoped().Table(e.TableName()).Where(" table_id in (?)", id).Delete(&SysColumns{}).Error; err != nil {
		return
	}
	Result = true
	return
}

func (e *SysTables) updateWithColumns(tx *gorm.DB) (err error) {
	if err = tx.Table("sys_tables").Where("table_id = ?", e.TableID).Updates(&e).Error; err != nil {
		return
	}

	tableNames := make([]string, 0)
	for i := range e.Columns {
		if e.Columns[i].FKTableName != "" {
			tableNames = append(tableNames, e.Columns[i].FKTableName)
		}
	}

	tables := make([]SysTables, 0)
	tableMap := make(map[string]*SysTables)
	if len(tableNames) > 0 {
		if err = tx.Table("sys_tables").Where("table_name in (?)", tableNames).Find(&tables).Error; err != nil {
			return
		}
		for i := range tables {
			tableMap[tables[i].TBName] = &tables[i]
		}
	}

	for i := range e.Columns {
		if e.Columns[i].FKTableName != "" {
			t, ok := tableMap[e.Columns[i].FKTableName]
			if ok {
				e.Columns[i].FKTableNameClass = t.ClassName
				t.MLTBName = strings.ReplaceAll(t.TBName, "_", "-")
				e.Columns[i].FKTableNamePackage = t.MLTBName
			} else {
				tableNameList := strings.Split(e.Columns[i].FKTableName, "_")
				e.Columns[i].FKTableNameClass = ""
				//e.Columns[i].FkTableNamePackage = ""
				for a := range tableNameList {
					strStart := string([]byte(tableNameList[a])[:1])
					strEnd := string([]byte(tableNameList[a])[1:])
					e.Columns[i].FKTableNameClass += strings.ToUpper(strStart) + strEnd
					//e.Columns[i].FkTableNamePackage += strings.ToLower(strStart) + strings.ToLower(strEnd)
				}
			}
		}
		if _, err = e.Columns[i].Update(tx); err != nil {
			return fmt.Errorf("column %s: %w", e.Columns[i].ColumnName, err)
		}
	}
	return
}
