package api

import "go-admin/app/diagnostics/router"

func init() { AppRouters = append(AppRouters, router.InitRouter) }
