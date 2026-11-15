package api

import "github.com/gatework/motor/app/codegen/router"

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	AppRouters = append(AppRouters, router.InitRouter)
}
