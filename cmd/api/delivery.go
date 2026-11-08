package api

import "go-admin/app/delivery/router"

func init() { AppRouters = append(AppRouters, router.InitRouter) }
