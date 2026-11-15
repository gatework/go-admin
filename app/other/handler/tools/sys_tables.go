package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/other/models/tools"
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	var data tools.SysTables
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

	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	var data tools.SysTables
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	var data tools.SysTables
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	var data tools.SysTables
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	tablesList, err := tablesToImport(c)
	if err != nil {
		log.Errorf("read the table list, %s", err.Error())
		e.Error(500, err, "")
		return
	}

	// Every table is read and checked before any is saved, so a list with
	// one table whose name cannot become a file name is refused whole
	// instead of importing the tables ahead of it.
	tables := make([]tools.SysTables, 0, len(tablesList))
	for i := range tablesList {
		data, err := genTableInit(db, tablesList, i, c)
		if err != nil {
			log.Errorf("genTableInit error, %s", err.Error())
			e.Error(500, err, "")
			return
		}
		if err = validateGenPathFields(data); err != nil {
			log.Errorf("validate table error, %s", err.Error())
			e.Error(500, err, err.Error())
			return
		}
		tables = append(tables, data)
	}
	for i := range tables {
		if _, err = tables[i].Create(db); err != nil {
			log.Errorf("Create error, %s", err.Error())
			e.Error(500, err, "")
			return
		}
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

func genTableInit(tx *gorm.DB, tablesList []string, i int, c *gin.Context) (tools.SysTables, error) {
	var data tools.SysTables
	var dbTable tools.DBTables
	var dbColumn tools.DBColumns
	data.TBName = tablesList[i]
	data.CreateBy = 0

	dbTable.TableName = data.TBName
	dbtable, err := dbTable.Get(tx)
	if err != nil {
		return data, err
	}

	dbColumn.TableName = data.TBName
	data.ClassName, data.BusinessName, err = generatorNames(dbColumn.TableName)
	if err != nil {
		return data, err
	}

	//data.ModuleFrontName = strings.ReplaceAll(data.ModuleName, "_", "-")
	data.PackageName = "admin"
	data.TplCategory = "crud"
	data.Crud = true
	// 中横线表名称，接口路径、前端文件夹名称和js名称使用
	data.ModuleName = strings.ReplaceAll(data.TBName, "_", "-")
	dbcolumn, err := dbColumn.GetList(tx)
	if err != nil {
		return data, err
	}
	if tx.Name() == "postgres" {
		if err := validatePostgresGeneratorIdentifier(data.TBName); err != nil {
			return data, err
		}
		var reserved bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM pg_get_keywords() WHERE word = ? AND catcode = 'R')", data.TBName).Scan(&reserved).Error; err != nil {
			return data, fmt.Errorf("validate PostgreSQL generator table name: %w", err)
		}
		if reserved {
			return data, fmt.Errorf("PostgreSQL table name %q is a reserved keyword and unsupported by generated search queries; rename the table using a non-reserved lowercase identifier", data.TBName)
		}
		for _, source := range dbcolumn {
			if err := validatePostgresGeneratorIdentifier(source.ColumnName); err != nil {
				return data, err
			}
		}
		if err := validatePostgresAuditColumns(dbcolumn); err != nil {
			return data, err
		}
	}
	data.CreateBy = 0
	data.TableComment = dbtable.TableComment
	if dbtable.TableComment == "" {
		data.TableComment = data.ClassName
	}

	data.FunctionName = data.TableComment
	//data.BusinessName = data.ModuleName
	data.IsLogicalDelete = "1"
	data.LogicalDelete = true
	data.LogicalDeleteColumn = "is_del"
	data.IsActions = 2
	data.IsDataScope = 1
	data.IsAuth = 1

	data.FunctionAuthor = "wenjianzhang"
	if err := initializeGeneratorColumns(&data, dbcolumn, tx.Name()); err != nil {
		return data, err
	}
	return data, validateAndSanitizeColumns(data.Columns)
}

// Update provides update for this module.
// Update
// @Summary 修改表结构
// @Description 修改表结构
// @Tags 工具 / 生成工具
// @Accept  application/json
// @Product application/json
// @Param data body tools.SysTables true "body"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/sys/tables/info [put]
// @Security Bearer
func (e Table) Update(c *gin.Context) {
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	var data tools.SysTables
	if err := c.ShouldBind(&data); err != nil {
		e.Error(400, nil, "数据解析失败")
		return
	}
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	// PRD 010 F10: this bind-and-save path has no field-level validation of
	// its own (API契约.md §1.2/§2.1, D6) - see sys_tables_validate.go for
	// what each check guards and why colWidth is sanitized in place rather
	// than rejected.
	if err = validateAndSanitizeColumns(data.Columns); err != nil {
		log.Errorf("validate columns error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}
	if err = validateGenPathFields(data); err != nil {
		log.Errorf("validate table error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}
	if err = validateBusinessNameUnique(db, data.PackageName, data.BusinessName, data.TableID); err != nil {
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
	e.Context = c
	e.Logger = logger.NewHelper(logger.Default())
	log := e.Logger
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, "数据库连接获取失败")
		return
	}

	var data tools.SysTables
	IDs := pkg.IDsStrToIDsIntGroup("tableId", c)
	_, err = data.BatchDelete(db, IDs)
	if err != nil {
		log.Errorf("BatchDelete error, %s", err.Error())
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(nil, "删除成功")
}

func initializeGeneratorColumns(data *tools.SysTables, dbcolumn []tools.DBColumns, driver string) error {
	var err error
	for i := range dbcolumn {
		var column tools.SysColumns
		column.ColumnComment = dbcolumn[i].ColumnComment
		column.ColumnName = dbcolumn[i].ColumnName
		column.ColumnType = dbcolumn[i].ColumnType
		column.Sort = i + 1
		column.Insert = true
		column.IsInsert = "1"
		column.QueryType = "EQ"
		column.IsPK = "0"

		column.GoField, column.JSONField, err = generatorNames(dbcolumn[i].ColumnName)
		if err != nil {
			return err
		}

		if strings.Contains(dbcolumn[i].ColumnKey, "PR") {
			column.IsPK = "1"
			column.PK = true
			data.PKColumn = dbcolumn[i].ColumnName
			//column.GoField = strings.ToUpper(column.GoField)
			//column.JsonField = strings.ToUpper(column.JsonField)
			data.PKGoField = column.GoField
			data.PKJSONField = column.JSONField
		}
		column.IsRequired = "0"
		if strings.Contains(dbcolumn[i].IsNullable, "NO") {
			column.IsRequired = "1"
			column.Required = true
		}

		if err := mapGeneratorColumn(&column, dbcolumn[i], driver); err != nil {
			return err
		}

		data.Columns = append(data.Columns, column)
	}
	return nil
}
