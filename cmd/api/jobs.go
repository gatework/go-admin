package api

import "github.com/gatework/motor/app/jobs/router"

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	//注册路由 fixme 其他应用的路由，在本目录新建文件放在init方法
	AppRouters = append(AppRouters, router.InitRouter)
}
