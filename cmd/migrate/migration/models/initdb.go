package models

import (
	"fmt"
	"log"
	"strings"

	"github.com/gatework/motor/config"

	"gorm.io/gorm"
)

// InitDB handles the initdb operation.
func InitDB(db *gorm.DB) (err error) {
	filePath := "config/db.sql"
	switch db.Name() {
	case "postgres":
		filePath := "config/db.sql"
		if err = ExecSQL(db, filePath); err != nil {
			return err
		}
		filePath = "config/pg.sql"
		err = ExecSQL(db, filePath)
	case "mysql":
		filePath = "config/db-begin-mysql.sql"
		if err = ExecSQL(db, filePath); err != nil {
			return err
		}
		filePath = "config/db.sql"
		if err = ExecSQL(db, filePath); err != nil {
			return err
		}
		filePath = "config/db-end-mysql.sql"
		err = ExecSQL(db, filePath)
	default:
		err = ExecSQL(db, filePath)
	}
	return err
}

// ExecSQL handles the execsql operation.
func ExecSQL(db *gorm.DB, filePath string) error {
	sql, err := readSeedSQL(filePath)
	if err != nil {
		fmt.Println("数据库基础数据初始化脚本读取失败！原因:", err.Error())
		return err
	}
	sqlList := strings.Split(sql, ";")
	for i := range len(sqlList) - 1 {
		if strings.Contains(sqlList[i], "--") {
			fmt.Println(sqlList[i])
			continue
		}
		sql := strings.ReplaceAll(sqlList[i]+";", "\n", "")
		sql = strings.TrimSpace(sql)
		if err = db.Exec(sql).Error; err != nil {
			log.Printf("error sql: %s", sql)
			if !strings.Contains(err.Error(), "Query was empty") {
				return err
			}
		}
	}
	return nil
}

// readSeedSQL handles the ioutil operation.
func readSeedSQL(filePath string) (string, error) {
	contents, err := config.ReadSeed(filePath)
	if err != nil {
		return "", err
	}
	return strings.Replace(string(contents), "\n", "", 1), nil
}
