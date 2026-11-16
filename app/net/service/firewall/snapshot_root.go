package firewall

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/net/models/firewall/storage"
)

var roots sync.Map

// RegisterRoot associates an existing tenant database with a stable private store.
func RegisterRoot(db *gorm.DB, tenant string) string {
	root := os.Getenv("FIREWALL_SNAPSHOT_DIR")
	if root == "" {
		root = filepath.Join("data", "firewall")
	}
	sum := sha256.Sum256([]byte(tenant))
	root = filepath.Join(root, hex.EncodeToString(sum[:16]))
	// Request middleware uses WithContext, producing a different *gorm.DB per
	// request. Its underlying tenant connection pool remains stable.
	if pool, err := db.DB(); err == nil {
		roots.Store(pool, root)
	}
	return root
}

func (s *Firewall) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	if s.DB == nil {
		return "", UnavailableErr
	}
	pool, err := s.DB.DB()
	if err != nil {
		return "", UnavailableErr
	}
	if v, ok := roots.Load(pool); ok {
		root, ok := v.(string)
		if !ok {
			return "", UnavailableErr
		}
		return root, nil
	}
	return "", UnavailableErr
}

func (s *Firewall) store() (*storage.Store, error) {
	root, e := s.root()
	if e != nil {
		return nil, e
	}
	return storage.New(s.DB, root)
}
