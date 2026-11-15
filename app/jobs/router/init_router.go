package router

import (
	//"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gin-gonic/gin"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	common "github.com/gatework/motor/common/middleware"
)

// InitRouter 路由初始化，不要怀疑，这里用到了
func InitRouter() {
	r, ok := sdk.Runtime.Engine().(*gin.Engine)
	if !ok || r == nil {
		log.NewHelper(log.Default()).Error("router: Gin engine is required")
		return
	}
	// the jwt middleware: shared instance InitMiddleware built at startup,
	// not one built here per module (see common/middleware.GetAuthMiddleware).
	authMiddleware := common.GetAuthMiddleware()

	// 注册业务路由
	initRouter(r, authMiddleware)
}
