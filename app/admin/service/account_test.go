package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"

	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/account"
)

// Actions are rate-limited per client address; reads of settings and status
// are not. Ten requests from one address pass, the eleventh is refused.
func TestAccountOperationsAreRateLimitedExceptReads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&account.Limit{}, &account.Policy{}); err != nil {
		t.Fatal(err)
	}
	s := &Account{}
	s.DB, s.Logger = db, logger.NewHelper(logger.Default())
	noop := func(context.Context, *Account, account.Service, int, dto.AccountRequest) (any, error) {
		return nil, nil
	}
	for range 20 {
		if _, err := s.Run(t.Context(), "settings", 0, "198.51.100.1", dto.AccountRequest{}, noop); err != nil {
			t.Fatalf("settings read was limited: %v", err)
		}
	}
	for i := range 10 {
		if _, err := s.Run(t.Context(), "register", 0, "198.51.100.1", dto.AccountRequest{}, noop); err != nil {
			t.Fatalf("register attempt %d refused: %v", i+1, err)
		}
	}
	if _, err := s.Run(t.Context(), "register", 0, "198.51.100.1", dto.AccountRequest{}, noop); err == nil {
		t.Fatal("eleventh register attempt from one address allowed")
	}
	if _, err := s.Run(t.Context(), "register", 0, "198.51.100.2", dto.AccountRequest{}, noop); err != nil {
		t.Fatalf("another address shared the limit: %v", err)
	}
}
