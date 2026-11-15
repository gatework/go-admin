package handler

import (
	"github.com/gin-gonic/gin"

	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/codegen/models"
)

// GetDBTableList 分页列表数据
// @Summary 分页列表数据 / page list data
// @Description 数据库表分页列表 / database table page list
// @Tags 工具 / 生成工具
// @Param tableName query string false "tableName / 数据表名称"
// @Param pageSize query int false "pageSize / 页条数"
// @Param pageIndex query int false "pageIndex / 页码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/db/tables/page [get]
func (e Generator) GetDBTableList(c *gin.Context) {
	//var res response.Response
	var data models.DBTables
	var err error
	var pageSize = 10
	var pageIndex = 1
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	if size := c.Request.FormValue("pageSize"); size != "" {
		pageSize, err = pkg.StringToInt(size)
		if err != nil {
			e.Error(400, err, "invalid pagination")
			return
		}
	}

	if index := c.Request.FormValue("pageIndex"); index != "" {
		pageIndex, err = pkg.StringToInt(index)
		if err != nil {
			e.Error(400, err, "invalid pagination")
			return
		}
	}

	data.TableName = c.Request.FormValue("tableName")
	result, count, err := data.GetPage(db, pageSize, pageIndex)
	if err != nil {
		log.Errorf("GetPage error, %s", err.Error())
		e.Error(500, err, "")
		return
	}
	e.PageOK(result, count, pageIndex, pageSize, "查询成功")
}
