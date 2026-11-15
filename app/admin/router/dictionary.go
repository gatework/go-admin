package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerDictRouter)
}

func registerDictRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	dictAPI := handler.Dictionary{}
	dataAPI := handler.DictionaryEntry{}
	dicts := v1.Group("/dict").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{

		dicts.GET("/data", dataAPI.List)
		dicts.GET("/data/:dictCode", dataAPI.Get)
		dicts.POST("/data", dataAPI.Create)
		dicts.PUT("/data/:dictCode", dataAPI.Update)
		dicts.DELETE("/data", dataAPI.Delete)

		dicts.GET("/type-option-select", dictAPI.All)
		dicts.GET("/type", dictAPI.List)
		dicts.GET("/type/:id", dictAPI.Get)
		dicts.POST("/type", dictAPI.Create)
		dicts.PUT("/type/:id", dictAPI.Update)
		dicts.DELETE("/type", dictAPI.Delete)
	}
	opSelect := v1.Group("/dict-data").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		opSelect.GET("/option-select", dataAPI.All)
	}
}
