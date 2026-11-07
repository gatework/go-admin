package tools

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DBTables struct {
	TableName      string `gorm:"column:TABLE_NAME" json:"tableName"`
	Engine         string `gorm:"column:ENGINE" json:"engine"`
	TableRows      string `gorm:"column:TABLE_ROWS" json:"tableRows"`
	TableCollation string `gorm:"column:TABLE_COLLATION" json:"tableCollation"`
	CreateTime     string `gorm:"column:CREATE_TIME" json:"createTime"`
	UpdateTime     string `gorm:"column:UPDATE_TIME" json:"updateTime"`
	TableComment   string `gorm:"column:TABLE_COMMENT" json:"tableComment"`
}

func (e *DBTables) GetPage(tx *gorm.DB, pageSize int, pageIndex int) ([]DBTables, int, error) {
	table, err := metadataTables(tx)
	if err != nil {
		return nil, 0, err
	}

	var doc []DBTables
	var count int64

	// Tables already registered with the generator are not candidates. Read them
	// through the model on this connection: the subquery used to spell the
	// schema out by hand, so it only resolved when sys_tables happened to live
	// in the schema being generated from, and it counted soft-deleted rows.
	var generated []string
	if err := tx.Model(&SysTables{}).Pluck("table_name", &generated).Error; err != nil {
		return nil, 0, err
	}

	if len(generated) > 0 {
		// NOT IN (NULL) is unknown for every row, so an empty list has to skip
		// the clause instead of rendering it.
		table = table.Where(clause.Not(clause.IN{Column: clause.Column{Name: "TABLE_NAME"}, Values: tableNamesAsValues(generated)}))
	}

	if e.TableName != "" {
		table = table.Where(clause.Eq{Column: clause.Column{Name: "TABLE_NAME"}, Value: e.TableName})
	}
	if err := table.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if err := table.Order(clause.OrderByColumn{Column: clause.Column{Name: "TABLE_NAME"}}).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&doc).Error; err != nil {
		return nil, 0, err
	}
	return doc, int(count), nil
}

func (e *DBTables) Get(tx *gorm.DB) (DBTables, error) {

	var doc DBTables
	if e.TableName == "" {
		return doc, errors.New("table name cannot be empty！")
	}
	table, err := metadataTables(tx)
	if err != nil {
		return doc, err
	}
	table = table.Where(clause.Eq{Column: clause.Column{Name: "TABLE_NAME"}, Value: e.TableName})
	if err := table.Take(&doc).Error; err != nil {
		return doc, err
	}
	return doc, nil
}

func tableNamesAsValues(names []string) []interface{} {
	values := make([]interface{}, len(names))
	for i, name := range names {
		values[i] = name
	}
	return values
}
