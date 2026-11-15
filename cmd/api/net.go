package api

import (
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/app/net/router"
	"github.com/gatework/motor/app/net/service"
)

//nolint:gochecknoinits // Registers host routes before the runtime startup registry is sealed.
func init() { AppRouters = append(AppRouters, initNet) }

func initNet() {
	workers := service.Workers(sdk.Runtime.Databases())
	router.InitRouter()
	for _, worker := range workers {
		startWorker(worker.Name, worker.Run)
	}
}
