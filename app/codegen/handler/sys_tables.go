package handler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gatework/motor/app/codegen/service"

	"github.com/gatework/motor-core/jwtauth/user"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/codegen/models"
)

// Table carries sys table state for this module.
type Table struct {
	api.API
}

// List 分页列表数据
// @Summary 分页列表数据
// @Description 生成表分页列表
// @Tags 工具 / 生成工具
// @Param tableName query string false "tableName / 数据表名称"
// @Param pageSize query int false "pageSize / 页条数"
// @Param pageIndex query int false "pageIndex / 页码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys/tables/page [get]
func (e Table) List(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger
	var data models.SysTables
	var err error
	var pageSize = 10
	var pageIndex = 1

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

	data.TBName = c.Request.FormValue("tableName")
	data.TableComment = c.Request.FormValue("tableComment")
	result, count, err := data.GetPage(db, pageSize, pageIndex)
	if err != nil {
		log.Errorf("GetPage error, %s", err.Error())
		e.Error(500, err, "")
		return
	}
	e.PageOK(result, count, pageIndex, pageSize, "查询成功")
}

// Get provides get for this module.
// Get
// @Summary 获取配置
// @Description 获取JSON
// @Tags 工具 / 生成工具
// @Param configKey path int true "configKey"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys/tables/info/{tableId} [get]
// @Security Bearer
func (e Table) Get(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	var data models.SysTables
	data.TableID, _ = pkg.StringToInt(c.Param("tableId"))
	result, err := data.Get(db, true)
	if err != nil {
		log.Errorf("Get error, %s", err.Error())
		e.Error(500, err, "")
		return
	}

	mp := make(map[string]any)
	mp["list"] = result.Columns
	mp["info"] = result
	e.OK(mp, "")
}

// Columns provides get sys tables info for this module.
func (e Table) Columns(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	var data models.SysTables
	if c.Request.FormValue("tableName") != "" {
		data.TBName = c.Request.FormValue("tableName")
	}
	result, err := data.Get(db, true)
	if err != nil {
		log.Errorf("Get error, %s", err.Error())
		e.Error(500, err, "抱歉未找到相关信息")
		return
	}

	mp := make(map[string]any)
	mp["list"] = result.Columns
	mp["info"] = result
	e.OK(mp, "")
	//res.Data = mp
	//c.JSON(http.StatusOK, res.ReturnOK())
}

// Tree provides get sys tables tree for this module.
func (e Table) Tree(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	var data models.SysTables
	result, err := data.GetTree(db)
	if err != nil {
		log.Errorf("GetTree error, %s", err.Error())
		e.Error(500, err, "抱歉未找到相关信息")
		return
	}

	e.OK(result, "")
}

// Create provides insert for this module.
// Create
// @Summary 添加表结构
// @Description 添加表结构
// @Tags 工具 / 生成工具
// @Accept  application/json
// @Product application/json
// @Param tables query string false "tableName / 数据表名称，逗号分隔"
// @Param data body object false "tables / 同上，query 未带时从 JSON body 读"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/sys/tables/info [post]
// @Security Bearer
func (e Table) Create(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	tablesList, err := tablesToImport(c)
	if err != nil {
		log.Errorf("read the table list, %s", err.Error())
		e.Error(500, err, "")
		return
	}

	// Every table is read and checked before any is saved, so a list with
	// one table whose name cannot become a file name is refused whole
	// instead of importing the tables ahead of it.
	tables := make([]models.SysTables, 0, len(tablesList))
	for i := range tablesList {
		data, err := service.InitTable(db, tablesList[i], user.Name(c))
		if err != nil {
			log.Errorf("reading table %s: %v", tablesList[i], err)
			e.Error(500, err, "")
			return
		}
		if err = service.ValidateGenPathFields(data); err != nil {
			log.Errorf("validate table error, %s", err.Error())
			e.Error(500, err, err.Error())
			return
		}
		tables = append(tables, data)
	}
	// One transaction for the whole list: a failure on a later table used to
	// leave the earlier ones imported.
	creator := user.ID(c)
	err = db.Transaction(func(tx *gorm.DB) error {
		for i := range tables {
			tables[i].CreateBy = creator
			if _, err := tables[i].Create(tx); err != nil {
				return fmt.Errorf("table %s: %w", tables[i].TBName, err)
			}
		}
		return nil
	})
	if err != nil {
		log.Errorf("importing tables: %v", err)
		e.Error(500, nil, "导入失败")
		return
	}
	e.OK(nil, "添加成功")

}

// tablesToImport reads the comma-separated table list carried by an import
// request, from the query string or from a JSON body.
//
// The list has only ever travelled in the query string, which is the single
// place FormValue looks once the request declares itself as JSON. A front end
// that puts it in the body instead therefore left this empty, and the import
// went on to ask information_schema for a table named "" -- go-admin-ui v3.2.0
// shipped exactly that, and every import failed with the message below.
// Reading the body when the query has nothing keeps either front end working.
func tablesToImport(c *gin.Context) ([]string, error) {
	raw := c.Request.FormValue("tables")
	if raw == "" {
		var body struct {
			Tables string `json:"tables"`
		}
		// A body that is absent, or shaped some other way, is not itself worth
		// reporting: the list is missing either way, and the message below says
		// so in the terms the caller asked in.
		if err := c.ShouldBindJSON(&body); err == nil {
			raw = body.Tables
		}
	}

	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, name := range parts {
		// Splitting "" yields one empty name rather than nothing at all, so
		// without this an empty list reads as a request to import one table
		// whose name happens to be blank.
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, errors.New(emptyTableNameMsg)
	}
	return names, nil
}

// Update provides update for this module.
// Update
// @Summary 修改表结构
// @Description 修改表结构
// @Tags 工具 / 生成工具
// @Accept  application/json
// @Product application/json
// @Param data body models.SysTables true "body"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/sys/tables/info [put]
// @Security Bearer
func (e Table) Update(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger
	var data models.SysTables
	if err := c.ShouldBind(&data); err != nil {
		e.Error(400, nil, "数据解析失败")
		return
	}

	// PRD 010 F10: this bind-and-save path has no field-level validation of
	// its own (API契约.md §1.2/§2.1, D6) - see sys_tables_validate.go for
	// what each check guards and why colWidth is sanitized in place rather
	// than rejected.
	if err := service.ValidateAndSanitizeColumns(data.Columns); err != nil {
		log.Errorf("validate columns error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}
	if err := service.ValidateGenPathFields(data); err != nil {
		log.Errorf("validate table error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}
	if err := service.ValidateBusinessNameUnique(db, data.PackageName, data.BusinessName, data.TableID); err != nil {
		log.Errorf("validate businessName error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}

	data.UpdateBy = 0
	result, err := data.Update(db)
	if err != nil {
		log.Errorf("Update error, %s", err.Error())
		e.Error(500, err, "")
		return
	}
	e.OK(result, "修改成功")
}

// Delete provides delete for this module.
// Delete
// @Summary 删除表结构
// @Description 删除表结构
// @Tags 工具 / 生成工具
// @Param tableId path int true "tableId"
// @Success 200 {string} string	"{"code": 200, "message": "删除成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "删除失败"}"
// @Router /api/v1/sys/tables/info/{tableId} [delete]
func (e Table) Delete(c *gin.Context) {
	requestAPI, db, ok := prepareRequest(c)
	if !ok {
		return
	}
	e.API = requestAPI
	log := e.Logger

	var data models.SysTables
	IDs := pkg.IDsStrToIDsIntGroup("tableId", c)
	_, err := data.BatchDelete(db, IDs)
	if err != nil {
		log.Errorf("BatchDelete error, %s", err.Error())
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(nil, "删除成功")
}
