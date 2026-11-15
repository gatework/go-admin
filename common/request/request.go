// Package request connects HTTP handlers to the host's request-scoped resources.
package request

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/pkg"
)

// New resolves the tenant database selected by middleware and borrows runtime
// logging and cache resources for one request. The API is available on a missing
// database so a handler can still return the standard error envelope.
func New(c *gin.Context) (api.API, error) {
	db, dbErr := pkg.GetORM(c)
	a, err := api.New(c, api.Dependencies{DB: db, Logger: sdk.Runtime.Logger(), Cache: sdk.Runtime.Cache()})
	if err != nil {
		return api.API{}, err
	}
	return *a, dbErr
}
