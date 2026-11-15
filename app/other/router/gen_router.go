package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/app/other/handler/tools"
	"github.com/gatework/motor/common/middleware"
)

// GenWriteRoutesEnabled reports whether the code generator's writing endpoints
// are registered in this process.
//
// Three of the generator's endpoints do not read. /gen/toproject writes seven
// Go and Vue source files onto this host, one of them under the path
// gen.frontpath names; /gen/apitofile writes a migration; /gen/todb inserts
// menus and APIs. They pass AuthCheckRole like every generator route, so they
// reach the administrator and any role granted the generator's menus - but
// writing source files onto the serving host is a bargain a workstation can
// make and a deployment cannot, whoever holds the grant.
//
// dev is the shipped default and is where the generator is meant to be used.
// demo keeps them because demo mode already has a better answer than a 404:
// DemoEnv refuses these three by name and explains itself, which is the thing
// the demo exists to show. test and prod get nothing.
//
// The mode is read once, while the routes are being built. Changing
// application.mode in a running process adds and removes nothing - a
// configuration reload rebuilds neither the engine nor its routes.
//
// core has constants for dev, test and prod but none for demo, which this
// repository spells as a literal in common/middleware/demo.go. Both are
// literals here so that the two read as one set rather than two conventions.
func GenWriteRoutesEnabled() bool {
	switch config.Current().Settings.Application.Mode {
	case "dev", "demo":
		return true
	default:
		return false
	}
}

//nolint:gochecknoinits // Host startup collects router registrations before serving requests.
func init() {
	routerCheckRole = append(routerCheckRole, sysNoCheckRoleRouter, registerDBRouter, registerSysTableRouter)
}

func sysNoCheckRoleRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	r1 := v1.Group("")
	{
		sys := handler.Captcha{}
		r1.GET("/captcha", sys.Generate)
	}

	// AuthCheckRole on every generator route: a role reaches them through the
	// code generator's menus, which the 1786700011000 migration binds them to.
	// They used to be in CasbinExclude, which let any account that could log
	// in read and change any table's configuration and, in dev mode, write
	// files from it.
	r := v1.Group("").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		gen := tools.Generator{}
		r.GET("/gen/preview/:tableId", gen.Preview)
		if GenWriteRoutesEnabled() {
			r.GET("/gen/toproject/:tableId", gen.Generate)
			r.GET("/gen/apitofile/:tableId", gen.GenerateMigration)
			r.GET("/gen/todb/:tableId", gen.CreateMenu)
		}
		sysTable := tools.Table{}
		r.GET("/gen/tabletree", sysTable.Tree)
	}
}

func registerDBRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	db := v1.Group("/db").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		gen := tools.Generator{}
		db.GET("/tables/page", gen.GetDBTableList)
		db.GET("/columns/page", gen.GetDBColumnList)
	}
}

func registerSysTableRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	tables := v1.Group("/sys/tables")
	{
		sysTable := tools.Table{}
		tables.Group("").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).GET("/page", sysTable.List)
		tablesInfo := tables.Group("/info").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
		{
			tablesInfo.POST("", sysTable.Create)
			tablesInfo.PUT("", sysTable.Update)
			tablesInfo.DELETE("/:tableId", sysTable.Delete)
			tablesInfo.GET("/:tableId", sysTable.Get)
			tablesInfo.GET("", sysTable.Columns)
		}
	}
}
