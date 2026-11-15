package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	common "github.com/gatework/motor/common/middleware"
)

// InitRouter 路由初始化，不要怀疑，这里用到了
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
	authMiddleware := common.GetAuthMiddleware()

	// 注册系统路由
	_ = InitSysRouter(r, authMiddleware)

	// 注册业务路由
	// TODO: 这里可存放业务路由，里边并无实际路由只有演示代码
	InitExamplesRouter(r, authMiddleware)
}
