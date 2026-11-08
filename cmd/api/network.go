package api

import "go-admin/app/network/router"

func init() {
	AppRouters = append(AppRouters, router.InitRouter)
}
