package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
)

var (
	routerNoCheckRole = make([]func(*gin.RouterGroup), 0)
	routerCheckRole   = make([]func(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware), 0)
)

// mountResourceRoutes mounts every resource module's routes under /api/v1.
func mountResourceRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware) *gin.Engine {

	// 无需认证的路由
	mountPublicRoutes(r)
	// 需要认证的路由
	mountAuthenticatedRoutes(r, authMiddleware)

	return r
}

// 无需认证的路由示例
func mountPublicRoutes(r *gin.Engine) {
	// 可根据业务需求来设置接口版本
	v1 := r.Group("/api/v1")
	for _, f := range routerNoCheckRole {
		f(v1)
	}
}

// 需要认证的路由示例
func mountAuthenticatedRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware) {
	// 可根据业务需求来设置接口版本
	v1 := r.Group("/api/v1")
	for _, f := range routerCheckRole {
		f(v1, authMiddleware)
	}
}
