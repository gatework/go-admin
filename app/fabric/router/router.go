package router

import (
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"go-admin/app/fabric/apis"
	"go-admin/common/actions"
	"go-admin/common/middleware"
)

func InitRouter() {
	engine, ok := sdk.Runtime.GetEngine().(*gin.Engine)
	if !ok {
		log.Fatal("Fabric requires Gin")
		return
	}
	InitBusinessRouter(engine, middleware.GetAuthMiddleware())
}
func InitBusinessRouter(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	api := apis.Fabric{}
	r := engine.Group("/api/v1/network-fabrics").Use(auth.MiddlewareFunc()).Use(routePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	r.GET("", api.Page)
	r.GET("/rules", api.Rules)
	r.POST("/rules/preview", api.PreviewRules)
	r.PUT("/rules", api.SaveRules)
	r.GET("/members", api.Members)
	r.PUT("/members/:deviceId", api.SetMember)
	r.GET("/endpoints", api.Endpoints)
	r.GET("/collections/:collectionId", api.Collection)
	r.GET("/changes/:changeId", api.Change)
	r.POST("/changes/:changeId/execute", api.ExecuteChange)
	r.POST("/changes/:changeId/cancel", api.CancelChange)
	r.GET("/:id", api.Get)
	r.PUT("/:id/settings", api.Settings)
	r.GET("/:id/nodes", api.Nodes)
	r.GET("/:id/segments", api.Segments)
	r.GET("/:id/vpc-groups", api.VPCGroups)
	r.GET("/:id/findings", api.Findings)
	r.GET("/:id/assessments", api.Assessments)
	r.GET("/:id/collections", api.Collections)
	r.POST("/:id/collections", api.CreateCollection)
	r.GET("/:id/changes", api.Changes)
	r.POST("/:id/change-candidates", api.ChangeCandidates)
	r.POST("/:id/changes", api.PreviewChange)
}
