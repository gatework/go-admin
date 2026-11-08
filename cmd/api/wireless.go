package api

import (
	"context"

	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/wireless/router"
	"go-admin/app/wireless/service"
	"gorm.io/gorm"
)

func init() { AppRouters = append(AppRouters, initWireless) }

func initWireless() {
	router.InitRouter()
	seen := map[*gorm.DB]bool{}
	for _, db := range sdk.Runtime.GetAllDb() {
		if db == nil || seen[db] {
			continue
		}
		seen[db] = true
		worker := service.NewWorker(&service.Wireless{Service: base.Service{Orm: db, Log: logger.NewHelper(logger.DefaultLogger)}}, nil)
		worker.Start()
		sdk.Runtime.SetShutdown(func(ctx context.Context) { worker.Close(ctx) })
	}
}
