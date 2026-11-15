package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/request"
)

// prepareRequest builds the request-scoped API (the request's logger and the
// tenant database bound to the request context) and answers 500 itself when
// the tenant has no database. The handlers used to assemble the API by hand,
// with the process default logger, and look the database up separately.
func prepareRequest(c *gin.Context) (api.API, *gorm.DB, bool) {
	requestAPI, err := request.New(c)
	if err != nil {
		requestAPI.Logger.Errorf("request database unavailable: %v", err)
		requestAPI.Error(http.StatusInternalServerError, nil, "数据库连接获取失败")
		return requestAPI, nil, false
	}
	return requestAPI, requestAPI.DB, true
}
