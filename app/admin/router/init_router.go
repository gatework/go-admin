package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/common/middleware"
)

// InitRouter mounts the administration routes on the host engine.
func InitRouter() {
	var r *gin.Engine
	h := sdk.Runtime.Engine()
	if h == nil {
		panic("router: host engine is not initialized")
	}
	switch h := h.(type) {
	case *gin.Engine:
		r = h
	default:
		panic("router: host engine must be a gin engine")
	}

	// the jwt middleware: shared instance InitMiddleware built at startup,
	// not one built here per module (see common/middleware.GetAuthMiddleware).
	authMiddleware := middleware.GetAuthMiddleware()

	// 注册系统路由
	_ = InitSysRouter(r, authMiddleware)

	// 注册各资源路由（用户、角色、菜单、部门、岗位、字典、配置、日志等）
	mountResourceRoutes(r, authMiddleware)
}
