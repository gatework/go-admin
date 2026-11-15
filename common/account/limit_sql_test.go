package account

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// KEY is reserved in MySQL. The rate limit's queries spelled the column bare,
// which is a syntax error there, and only PostgreSQL was ever tested.
func TestTakeLimitQuotesTheKeyColumnForMySQL(t *testing.T) {
	var statements []string
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/db", SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("capture", func(tx *gorm.DB) {
		statements = append(statements, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatal(err)
	}
	s := Service{DB: db, Now: func() time.Time { return time.Unix(1_800_000_000, 0) }}
	_ = s.TakeLimit("login:ip", 5, time.Minute) // dry run: RowsAffected is zero

	if len(statements) != 2 {
		t.Fatalf("captured %d update statements: %q", len(statements), statements)
	}
	for _, sql := range statements {
		where := sql[strings.Index(sql, "WHERE"):]
		if strings.Contains(where, " key ") || strings.HasPrefix(strings.TrimPrefix(where, "WHERE "), "key ") {
			t.Fatalf("bare reserved column in %q", sql)
		}
		if !strings.Contains(where, "`key` =") {
			t.Fatalf("key column not quoted in %q", sql)
		}
	}
}
