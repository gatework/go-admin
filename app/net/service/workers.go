// Package service assembles network operations for the application host.
// The host starts and joins workers using its application lifetime.
package service

import (
	"context"
	"maps"
	"slices"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor/app/net/service/delivery"
	"github.com/gatework/motor/app/net/service/fabric"
	"github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/inventory"
	"github.com/gatework/motor/app/net/service/wireless"
)

// Worker is a network background operation whose lifetime belongs to the host.
type Worker struct {
	Name string
	Run  func(context.Context) error
}

// Workers builds one worker set per physical tenant database without starting it.
// Sorted tenant names preserve deterministic ownership of shared firewall stores.
func Workers(databases map[string]*gorm.DB) []Worker {
	workers := make([]Worker, 0, len(databases)*6)
	seen := make(map[*gorm.DB]struct{}, len(databases))
	for _, tenant := range slices.Sorted(maps.Keys(databases)) {
		db := databases[tenant]
		if _, exists := seen[db]; db == nil || exists {
			continue
		}
		seen[db] = struct{}{}
		root := firewall.RegisterRoot(db, tenant)
		log := logger.NewHelper(logger.Default())
		policy := &firewall.Firewall{DB: db, Logger: log, Root: root}
		workers = append(workers,
			Worker{Name: "fabric", Run: fabric.NewWorker(&fabric.Fabric{DB: db, Logger: log}, nil).Run},
			Worker{Name: "wireless", Run: wireless.NewWorker(&wireless.Wireless{DB: db, Logger: log}, nil).Run},
			Worker{Name: "delivery", Run: delivery.NewWorker(&delivery.Workflow{DB: db, Logger: log, Config: delivery.FromEnvironment()}, nil).Run},
			Worker{Name: "firewall offline", Run: firewall.NewWorker(policy).Run},
			Worker{Name: "inventory sync", Run: inventory.NewSyncWorker(&inventory.Device{DB: db, Logger: log}).Run},
			Worker{Name: "firewall collection", Run: firewall.NewCollectionWorker(&firewall.Firewall{DB: db, Logger: log, Root: root}, nil).Run},
		)
	}
	return workers
}
