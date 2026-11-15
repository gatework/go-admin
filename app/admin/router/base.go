package router

import (
	"mime"

	"github.com/gatework/motor/app/admin/handler"

	"github.com/gatework/motor-core/sdk/config"

	"github.com/gin-gonic/gin"
	ginSwagger "github.com/swaggo/gin-swagger"

	jwt "github.com/gatework/motor-core/jwtauth"

	swaggerfiles "github.com/swaggo/files"

	"github.com/gatework/motor/common/middleware"
	"github.com/gatework/motor/common/middleware/auth"
	_ "github.com/gatework/motor/docs/admin" // Registers generated Swagger documentation. // Registers the documentation or route assets used by this package.
)

// InitSysRouter performs the init sys router administration operation.
func InitSysRouter(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware) *gin.RouterGroup {
	g := r.Group("")
	sysBaseRouter(g)
	// 静态文件
	sysStaticFileRouter(g)
	// swagger；注意：生产环境可以注释掉
	if config.Current().Settings.Application.Mode != "prod" {
		sysSwaggerRouter(g)
	}
	// 需要认证
	sysCheckRoleRouterInit(g, authMiddleware)
	return g
}

func sysBaseRouter(r *gin.RouterGroup) {

	if config.Current().Settings.Application.Mode != "prod" {
		r.GET("/", handler.Motor)
	}
	r.GET("/info", auth.Ping)
}

func sysStaticFileRouter(r *gin.RouterGroup) {
	err := mime.AddExtensionType(".js", "application/javascript")
	if err != nil {
		return
	}
	r.Static("/static", "./static")
}

func sysSwaggerRouter(r *gin.RouterGroup) {
	r.GET("/swagger/admin/*any", ginSwagger.WrapHandler(swaggerfiles.NewHandler(), ginSwagger.InstanceName("admin")))
}

func sysCheckRoleRouterInit(r *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	wss := r.Group("").Use(middleware.WebsocketToken).Use(authMiddleware.MiddlewareFunc())
	{
		connect, disconnect := auth.WebsocketHandlers(authMiddleware)
		wss.GET("/ws/:id/:channel", connect)
		wss.GET("/wslogout/:id/:channel", disconnect)
	}

	v1 := r.Group("/api/v1")
	{
		v1.POST("/login", authMiddleware.LoginHandler)
		// GET /api/v1/refresh_token 已移除，原因见 issue #820：
		// 该接口用业务 token 即可换取新 token，而续期上限 MaxRefresh 依据的
		// orig_iat 在每次续期时被一并重置，上限永远无法到达 —— token 一旦泄
		// 露即等同于永久访问权。它此前还位于 CasbinExclude 中，任何角色的已
		// 登录用户都能调用，不受权限约束。
		//
		// 官方前端从未调用该接口（store 中的 refreshToken action 无人 dispatch），
		// 移除不影响正常使用。若确需无感续期，应在 motor-core 中区分
		// access token 与 refresh token 后重新实现，而非沿用此路由。
	}
	registerBaseRouter(v1, authMiddleware)
}

func registerBaseRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.Menu{}
	departments := handler.Department{}
	v1auth := v1.Group("").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		v1auth.GET("/roleMenuTreeselect/:roleId", h.Tree)
		v1auth.GET("/roleDeptTreeselect/:roleId", departments.RoleTree)
		v1auth.POST("/logout", auth.LogOut)
	}
}
