package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/app/admin/models"
)

// StoreOperationLog shortens JSONResult before writing it. Cutting at a byte offset
// splits a multi-byte character, and PostgreSQL and strict-mode MySQL reject the
// resulting string, so the row is dropped and the only trace is a log line.
func TestStoreOperationLogTruncatesOnCharacterBoundary(t *testing.T) {
	const tenant = "opera-log-truncate-test"
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.SysOperaLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	previous := sdk.Runtime.DB(tenant)
	sdk.Runtime.SetDB(tenant, db)
	t.Cleanup(func() { sdk.Runtime.SetDB(tenant, previous) })

	// 150 characters, 3 bytes each: byte 100 falls inside the 34th character.
	long := strings.Repeat("中", 150)

	payload, err := json.Marshal(map[string]any{"title": "t", "jsonResult": long})
	if err != nil {
		t.Fatal(err)
	}
	msg := storage.Message{Values: map[string]any{"payload": string(payload), storage.PrefixKey: tenant}}

	if err := StoreOperationLog(t.Context(), msg); err != nil {
		t.Fatalf("StoreOperationLog: %v", err)
	}

	var rows []models.SysOperaLog
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row written, got %d", len(rows))
	}
	got := rows[0].JSONResult
	if !utf8.ValidString(got) {
		t.Errorf("stored JsonResult is not valid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 100 {
		t.Errorf("want 100 characters kept, got %d", n)
	}
}
