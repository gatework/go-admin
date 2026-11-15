package service

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/codegen/models"
)

// InitTable reads a database table's metadata and builds its generator
// configuration: names, columns with their Go and UI mappings, and defaults.
// author is recorded as the generated code's author.
func InitTable(tx *gorm.DB, table, author string) (models.SysTables, error) {
	var data models.SysTables
	var dbTable models.DBTables
	var dbColumn models.DBColumns
	data.TBName = table

	dbTable.TableName = data.TBName
	dbtable, err := dbTable.Get(tx)
	if err != nil {
		return data, err
	}

	dbColumn.TableName = data.TBName
	data.ClassName, data.BusinessName, err = generatorNames(dbColumn.TableName)
	if err != nil {
		return data, err
	}

	data.PackageName = "admin"
	data.TplCategory = "crud"
	data.Crud = true
	// 中横线表名称，接口路径、前端文件夹名称和js名称使用
	data.ModuleName = strings.ReplaceAll(data.TBName, "_", "-")
	dbcolumn, err := dbColumn.GetList(tx)
	if err != nil {
		return data, err
	}
	if tx.Name() == "postgres" {
		if err := validatePostgresGeneratorIdentifier(data.TBName); err != nil {
			return data, err
		}
		var reserved bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM pg_get_keywords() WHERE word = ? AND catcode = 'R')", data.TBName).Scan(&reserved).Error; err != nil {
			return data, fmt.Errorf("validate PostgreSQL generator table name: %w", err)
		}
		if reserved {
			return data, fmt.Errorf("PostgreSQL table name %q is a reserved keyword and unsupported by generated search queries; rename the table using a non-reserved lowercase identifier", data.TBName)
		}
		for _, source := range dbcolumn {
			if err := validatePostgresGeneratorIdentifier(source.ColumnName); err != nil {
				return data, err
			}
		}
		if err := validatePostgresAuditColumns(dbcolumn); err != nil {
			return data, err
		}
	}
	data.TableComment = dbtable.TableComment
	if dbtable.TableComment == "" {
		data.TableComment = data.ClassName
	}

	data.FunctionName = data.TableComment
	data.IsLogicalDelete = "1"
	data.LogicalDelete = true
	// Soft deletion in this project is the deleted_at marker of ModelTime.
	data.LogicalDeleteColumn = "deleted_at"
	data.IsActions = 2
	data.IsDataScope = 1
	data.IsAuth = 1

	data.FunctionAuthor = author
	if err := initializeGeneratorColumns(&data, dbcolumn, tx.Name()); err != nil {
		return data, err
	}
	return data, ValidateAndSanitizeColumns(data.Columns)
}

func initializeGeneratorColumns(data *models.SysTables, dbcolumn []models.DBColumns, driver string) error {
	var err error
	for i := range dbcolumn {
		var column models.SysColumns
		column.ColumnComment = dbcolumn[i].ColumnComment
		column.ColumnName = dbcolumn[i].ColumnName
		column.ColumnType = dbcolumn[i].ColumnType
		column.Sort = i + 1
		column.Insert = true
		column.IsInsert = "1"
		column.QueryType = "EQ"
		column.IsPK = "0"

		column.GoField, column.JSONField, err = generatorNames(dbcolumn[i].ColumnName)
		if err != nil {
			return err
		}

		if strings.Contains(dbcolumn[i].ColumnKey, "PR") {
			column.IsPK = "1"
			column.PK = true
			data.PKColumn = dbcolumn[i].ColumnName
			//column.GoField = strings.ToUpper(column.GoField)
			//column.JsonField = strings.ToUpper(column.JsonField)
			data.PKGoField = column.GoField
			data.PKJSONField = column.JSONField
		}
		column.IsRequired = "0"
		if strings.Contains(dbcolumn[i].IsNullable, "NO") {
			column.IsRequired = "1"
			column.Required = true
		}

		if err := mapGeneratorColumn(&column, dbcolumn[i], driver); err != nil {
			return err
		}

		data.Columns = append(data.Columns, column)
	}
	return nil
}
