package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"

	// "github.com/gatework/motor-core/sdk/pkg"

	jwt "github.com/gatework/motor-core/jwtauth"
	common "github.com/gatework/motor/common/middleware"
)

var (
	routerNoCheckRole = make([]func(*gin.RouterGroup), 0)
	routerCheckRole   = make([]func(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware), 0)
)

// InitRouter 路由初始化
func InitRouter() {
	var r *gin.Engine
	h := sdk.Runtime.Engine()
	if h == nil {
		h = gin.New()
		sdk.Runtime.SetEngine(h)
	}
	switch h := h.(type) {
	case *gin.Engine:
		r = h
	default:
		panic("router: host engine must be a gin engine")
	}

	// the jwt middleware: shared instance InitMiddleware built at startup,
	// not one built here per module (see common/middleware.GetAuthMiddleware).
	authMiddleware := common.GetAuthMiddleware()

	// 注册业务路由
	InitBusinessRouter(r, authMiddleware)
}

// InitBusinessRouter mounts authenticated and public business routes.
func InitBusinessRouter(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware) *gin.Engine {

	// 无需认证的路由
	noCheckRoleRouter(r)
	// 需要认证的路由
	checkRoleRouter(r, authMiddleware)

	return r
}

// noCheckRoleRouter 无需认证的路由
func noCheckRoleRouter(r *gin.Engine) {
	// 可根据业务需求来设置接口版本
	v := r.Group("/api/v1")

	for _, f := range routerNoCheckRole {
		f(v)
	}
}

// checkRoleRouter 需要认证的路由
func checkRoleRouter(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware) {
	// 可根据业务需求来设置接口版本
	v := r.Group("/api/v1")

	for _, f := range routerCheckRole {
		f(v, authMiddleware)
	}
}
