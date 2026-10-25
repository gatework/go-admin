package service

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockAdminRows 仅用于事务内的查询；表名来自模型，不能来自请求参数。
// SQL Server 用 UPDLOCK 持锁至事务结束，HOLDLOCK 同时保护树扫描的键范围。
func lockAdminRows(tx *gorm.DB, table string) *gorm.DB {
	if tx.Dialector.Name() == "sqlserver" {
		return tx.Table(table).Table("? WITH (UPDLOCK, HOLDLOCK)", clause.Table{Name: table})
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}
