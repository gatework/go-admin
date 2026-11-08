package api

import (
	"context"
	"slices"

	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/firewall/router"
	"go-admin/app/firewall/service"
	"gorm.io/gorm"
)

func init() { AppRouters = append(AppRouters, initFirewall) }
func initFirewall() {
	dbs := sdk.Runtime.GetAllDb()
	keys := make([]string, 0, len(dbs))
	for key := range dbs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	seen := map[*gorm.DB]bool{}
	for _, key := range keys {
		db := dbs[key]
		if db == nil || seen[db] {
			continue
		}
		seen[db] = true
		root := service.RegisterRoot(db, key)
		worker := service.NewWorker(&service.Firewall{Service: base.Service{Orm: db, Log: logger.NewHelper(logger.DefaultLogger)}, Root: root})
		worker.Start()
		collector := service.NewCollectionWorker(&service.Firewall{Service: base.Service{Orm: db, Log: logger.NewHelper(logger.DefaultLogger)}, Root: root}, nil)
		collector.Start()
		sdk.Runtime.SetShutdown(func(ctx context.Context) { collector.Close(ctx) })
		sdk.Runtime.SetShutdown(func(ctx context.Context) { worker.Close(ctx) })
	}
	router.InitRouter()
}
