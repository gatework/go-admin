package tools

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DBColumns struct {
	TableSchema            string `gorm:"column:TABLE_SCHEMA" json:"tableSchema"`
	TableName              string `gorm:"column:TABLE_NAME" json:"tableName"`
	ColumnName             string `gorm:"column:COLUMN_NAME" json:"columnName"`
	ColumnDefault          string `gorm:"column:COLUMN_DEFAULT" json:"columnDefault"`
	IsNullable             string `gorm:"column:IS_NULLABLE" json:"isNullable"`
	DataType               string `gorm:"column:DATA_TYPE" json:"dataType"`
	CharacterMaximumLength string `gorm:"column:CHARACTER_MAXIMUM_LENGTH" json:"characterMaximumLength"`
	CharacterSetName       string `gorm:"column:CHARACTER_SET_NAME" json:"characterSetName"`
	ColumnType             string `gorm:"column:COLUMN_TYPE" json:"columnType"`
	ColumnKey              string `gorm:"column:COLUMN_KEY" json:"columnKey"`
	Extra                  string `gorm:"column:EXTRA" json:"extra"`
	ColumnComment          string `gorm:"column:COLUMN_COMMENT" json:"columnComment"`
}

func (e *DBColumns) GetPage(tx *gorm.DB, pageSize int, pageIndex int) ([]DBColumns, int, error) {

	var doc []DBColumns
	var count int64

	if e.TableName == "" {
		return nil, 0, errors.New("table name cannot be empty！")
	}

	table, err := metadataColumns(tx)
	if err != nil {
		return nil, 0, err
	}
	table = table.Where(clause.Eq{Column: clause.Column{Name: "TABLE_NAME"}, Value: e.TableName})

	if err := table.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if err := table.Order(clause.OrderByColumn{Column: clause.Column{Name: "ORDINAL_POSITION"}}).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&doc).Error; err != nil {
		return nil, 0, err
	}
	return doc, int(count), nil
}

func (e *DBColumns) GetList(tx *gorm.DB) ([]DBColumns, error) {

	var doc []DBColumns
	if e.TableName == "" {
		return nil, errors.New("table name cannot be empty！")
	}

	table, err := metadataColumns(tx)
	if err != nil {
		return nil, err
	}
	table = table.Where(clause.Eq{Column: clause.Column{Name: "TABLE_NAME"}, Value: e.TableName}).Order(clause.OrderByColumn{Column: clause.Column{Name: "ORDINAL_POSITION"}})
	if err := table.Find(&doc).Error; err != nil {
		return doc, err
	}
	return doc, nil
}
