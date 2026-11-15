package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// RegisterFabric mounts authenticated business operations on the host router.
func RegisterFabric(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	h := handler.Fabric{}
	r := engine.Group("/api/v1/network-fabrics").Use(auth.MiddlewareFunc()).Use(fabricRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	r.GET("", h.List)
	r.GET("/rules", h.Rules)
	r.POST("/rules/preview", h.PreviewRules)
	r.PUT("/rules", h.SaveRules)
	r.GET("/members", h.Members)
	r.PUT("/members/:deviceId", h.SetMember)
	r.GET("/endpoints", h.Endpoints)
	r.GET("/collections/:collectionId", h.Collection)
	r.GET("/changes/:changeId", h.Change)
	r.POST("/changes/:changeId/execute", h.ExecuteChange)
	r.POST("/changes/:changeId/cancel", h.CancelChange)
	r.GET("/:id", h.Get)
	r.PUT("/:id/settings", h.Settings)
	r.GET("/:id/nodes", h.Nodes)
	r.GET("/:id/segments", h.Segments)
	r.GET("/:id/vpc-groups", h.VPCGroups)
	r.GET("/:id/findings", h.Findings)
	r.GET("/:id/assessments", h.Assessments)
	r.GET("/:id/collections", h.Collections)
	r.POST("/:id/collections", h.CreateCollection)
	r.GET("/:id/changes", h.Changes)
	r.POST("/:id/change-candidates", h.ChangeCandidates)
	r.POST("/:id/changes", h.PreviewChange)
}
