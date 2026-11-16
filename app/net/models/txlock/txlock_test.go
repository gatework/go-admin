package txlock

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A second transaction holding the same lock waits for the first to finish.
func TestHoldSerializesTransactionsOnTheSameName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lock.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Row{}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var order []string
	held := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = db.Transaction(func(tx *gorm.DB) error {
			if err := Hold(tx, DeviceGroups); err != nil {
				t.Error(err)
			}
			close(held)
			time.Sleep(200 * time.Millisecond)
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
			return nil
		})
	}()
	go func() {
		defer wg.Done()
		<-held
		_ = db.Transaction(func(tx *gorm.DB) error {
			if err := Hold(tx, DeviceGroups); err != nil {
				t.Error(err)
			}
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			return nil
		})
	}()
	wg.Wait()
	if len(order) != 2 || order[0] != "first" {
		t.Fatalf("order %v; the second holder ran inside the first", order)
	}
	var row Row
	if err := db.First(&row, "name = ?", DeviceGroups).Error; err != nil || row.Version != 2 {
		t.Fatalf("row %+v %v", row, err)
	}
}
