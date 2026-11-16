package api

import (
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/app/net/router"
	"github.com/gatework/motor/app/net/service"
	"github.com/gatework/motor/app/net/service/netconfig"
)

//nolint:gochecknoinits // Registers host routes before the runtime startup registry is sealed.
func init() { AppRouters = append(AppRouters, initNet) }

func initNet() {
	// The network configuration is read once, here, and handed to every
	// route and worker.
	cfg := netconfig.Load()
	workers := service.Workers(sdk.Runtime.Databases(), cfg)
	router.InitRouter(cfg)
	for _, worker := range workers {
		startWorker(worker.Name, worker.Run)
	}
}
