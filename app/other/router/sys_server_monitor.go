package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/other/handler"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // Host startup collects router registrations before serving requests.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysServerMonitorRouter)
}

// 需认证的路由代码
func registerSysServerMonitorRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.ServerMonitor{}
	r := v1.Group("/server-monitor").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.GET("", h.ServerInfo)
	}
}
