//go:build integration

package account

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Uses an explicitly supplied disposable database; it never connects to the development DB by default.
func TestPostgresConcurrentMFAConsumption(t *testing.T) {
	dsn := os.Getenv("ACCOUNT_TEST_DSN")
	if dsn == "" {
		t.Skip("set ACCOUNT_TEST_DSN to a disposable PostgreSQL database")
	}
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	schema := fmt.Sprintf("account_test_%d", time.Now().UnixNano())
	if e = db.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	scoped, e := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	conn, e := scoped.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if e = scoped.AutoMigrate(&User{}, &Role{}, &State{}, &Challenge{}, &Recovery{}, &Limit{}, &Policy{}); e != nil {
		t.Fatal(e)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-password-123"), bcrypt.MinCost)
	scoped.Create(&User{UserId: 1, Username: "test", Password: string(hash), Status: "2"})
	now := time.Now()
	s := Service{DB: scoped, Key: strings.Repeat("k", 32), Now: func() time.Time { return now }}
	setup, e := s.Setup(1, "admin-password-123")
	if e != nil {
		t.Fatal(e)
	}
	code, _ := otpCode(setup.Secret, now)
	codes, e := s.Enable(1, code)
	if e != nil {
		t.Fatal(e)
	}
	now = now.Add(30 * time.Second)
	code, _ = otpCode(setup.Secret, now)
	u, _ := s.User(1)
	state, _ := s.State(1)
	for _, recovery := range []bool{false, true} {
		first, _ := s.LoginChallenge(u, state)
		second, _ := s.LoginChallenge(u, state)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, token := range []string{first, second} {
			wg.Add(1)
			go func(token string) {
				defer wg.Done()
				<-start
				var err error
				if recovery {
					_, err = s.CompleteLogin(token, "", codes[0])
				} else {
					_, err = s.CompleteLogin(token, code, "")
				}
				results <- err
			}(token)
		}
		close(start)
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("recovery=%v: expected one winner, got %d", recovery, success)
		}
	}
}
