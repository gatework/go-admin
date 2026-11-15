package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/platform/handler"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // Host startup collects router registrations before serving requests.
func init() {
	routerCheckRole = append(routerCheckRole, registerFileRouter)
}

// 需认证的路由代码
func registerFileRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	var api = handler.File{}
	r := v1.Group("").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.POST("/public/uploadFile", api.UploadFile)
	}
}
