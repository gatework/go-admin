package api

import (
	"context"

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/fabric/router"
	"go-admin/app/fabric/service"
	"gorm.io/gorm"
)

func init() { AppRouters = append(AppRouters, initFabric) }

func initFabric() {
	router.InitRouter()
	seen := map[*gorm.DB]bool{}
	for _, db := range sdk.Runtime.GetAllDb() {
		if db == nil || seen[db] {
			continue
		}
		seen[db] = true
		worker := service.NewWorker(&service.Fabric{Service: base.Service{Orm: db}}, nil)
		worker.Start()
		sdk.Runtime.SetShutdown(func(ctx context.Context) { worker.Close(ctx) })
	}
}
