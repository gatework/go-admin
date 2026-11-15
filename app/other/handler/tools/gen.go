package tools

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/other/models/tools"
)

// Generator carries gen state for this module.
type Generator struct {
	api.API
}

// genLangFuncs backs the lang-zh/lang-en templates (PRD 010 F3/F9). The
// generated files are TypeScript, and go-admin-ui's eslint config requires
// single-quoted strings with no trailing comma (@stylistic/quotes,
// @stylistic/comma-dangle: never) - text/template's builtin `printf "%q"`
// only produces Go/JSON-style double-quoted output, so this supplies a
// single-quote equivalent instead of leaning on the builtin.
var genLangFuncs = template.FuncMap{
	"singleQuote": func(s string) string {
		r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`)
		return "'" + r.Replace(s) + "'"
	},
}

// parseGenTemplate is template.ParseFiles plus genLangFuncs, for the two
// language-pack templates. template.New's name must match the file's base
// name - ParseFiles reuses the template already registered under that name
// instead of creating an unnamed second one, which is what makes Execute
// find the parsed content afterwards.
func parseGenTemplate(path string) (*template.Template, error) {
	base := path[strings.LastIndex(path, "/")+1:]
	return template.New(base).Funcs(genLangFuncs).ParseFiles(path)
}

// Preview provides preview for this module.
func (e Generator) Preview(c *gin.Context) {
	e.API = api.API{Context: c, Logger: logger.NewHelper(logger.Default())}
	log := e.Logger
	table := tools.SysTables{}
	id, err := pkg.StringToInt(c.Param("tableId"))
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("tableId接收失败！错误详情：%s", err.Error()))
		return
	}
	table.TableID = id
	templates, message, err := loadGeneratorTemplates(previewTemplateSpecs())
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("%s！错误详情：%s", message, err.Error()))
		return
	}

	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("数据库链接获取失败！错误详情：%s", err.Error()))
		return
	}

	tab, err := table.Get(db, false)
	if err != nil {
		log.Errorf("get table error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("读取表配置失败！错误详情：%s", err.Error()))
		return
	}
	// MLTBName (table_name with underscores turned to dashes) is a gorm:"-"
	// field - table.Get never fills it in, so every template that reads it
	// (the .vue/.ts import paths, e.g. "@/api/{PackageName}/{MLTBName}")
	// silently rendered it empty here. NOActionsGen has set this since it
	// existed (see below); Preview never did, which is why the two paths
	// are not interchangeable stand-ins for each other and should not be
	// assumed to be.
	tab.MLTBName = strings.ReplaceAll(tab.TBName, "_", "-")
	if err := requireSinglePrimaryKey(tab); err != nil {
		e.Error(500, err, err.Error())
		return
	}
	// R2: infer a width for any column the config page left at colWidth's 0
	// sentinel, before vue.go.template reads .ColWidth - see column_width.go.
	applyInferredColumnWidths(tab.Columns)
	out, err := renderAll(tab, templates...)
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("模版渲染失败！错误详情：%s", err.Error()))
		return
	}

	mp := make(map[string]any)
	mp["template/model.go.template"] = string(out[0])
	mp["template/api.go.template"] = string(out[1])
	mp["template/api.ts.template"] = string(out[2])
	mp["template/vue.go.template"] = string(out[3])
	mp["template/router.go.template"] = string(out[4])
	mp["template/dto.go.template"] = string(out[5])
	mp["template/service.go.template"] = string(out[6])
	mp["template/lang-zh.go.template"] = string(out[7])
	mp["template/lang-en.go.template"] = string(out[8])
	e.OK(mp, "")
}

// Generate provides gen code for this module.
func (e Generator) Generate(c *gin.Context) {
	e.API = api.API{Context: c, Logger: logger.NewHelper(logger.Default())}
	log := e.Logger
	table := tools.SysTables{}
	id, err := pkg.StringToInt(c.Param("tableId"))
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("tableId参数接收失败！错误详情：%s", err.Error()))
		return
	}

	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("数据库链接获取失败！错误详情：%s", err.Error()))
		return
	}

	table.TableID = id
	tab, err := table.Get(db, false)
	if err != nil {
		log.Errorf("get table error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("读取表配置失败！错误详情：%s", err.Error()))
		return
	}

	if !e.writeFiles(c, tab) {
		return
	}

	e.OK("", "Code generated successfully！")
}

// GenerateMigration provides gen apito file for this module.
func (e Generator) GenerateMigration(c *gin.Context) {
	e.API = api.API{Context: c, Logger: logger.NewHelper(logger.Default())}
	log := e.Logger
	table := tools.SysTables{}
	id, err := pkg.StringToInt(c.Param("tableId"))
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("tableId参数获取失败！错误详情：%s", err.Error()))
		return
	}

	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db connection error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("数据库链接获取失败！错误详情：%s", err.Error()))
		return
	}

	table.TableID = id
	tab, err := table.Get(db, false)
	if err != nil {
		log.Errorf("get table error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("读取表配置失败！错误详情：%s", err.Error()))
		return
	}
	if !e.writeMigration(c, tab) {
		return
	}

	e.OK("", "Code generated successfully！")
}

// CreateMenu provides gen menu and api for this module.
func (e Generator) CreateMenu(c *gin.Context) {
	s := service.Menu{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	table := tools.SysTables{}
	id, err := pkg.StringToInt(c.Param("tableId"))
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("tableId参数解析失败！错误详情：%s", err.Error()))
		return
	}

	table.TableID = id
	tab, err := table.Get(e.DB, true)
	if err != nil {
		e.Logger.Errorf("get table error, %s", err.Error())
		e.Error(500, err, fmt.Sprintf("读取表配置失败！错误详情：%s", err.Error()))
		return
	}
	tab.MLTBName = strings.ReplaceAll(tab.TBName, "_", "-")

	Mmenu := generatedParentMenu(tab)
	if err := s.Create(&Mmenu); err != nil {
		e.Error(500, err, "menu creation failed")
		return
	}

	Cmenu := generatedChildMenu(tab, Mmenu.MenuID)
	if err := s.Create(&Cmenu); err != nil {
		e.Error(500, err, "menu creation failed")
		return
	}

	for _, action := range []struct{ title, permission string }{{"分页获取", "query"}, {"创建", "add"}, {"修改", "edit"}, {"删除", "remove"}} {
		menu := generatedActionMenu(tab, Cmenu.MenuID, action.title, action.permission)
		if err := s.Create(&menu); err != nil {
			e.Error(500, err, "menu creation failed")
			return
		}
	}

	e.OK("", "数据生成成功！")
}

// requireSinglePrimaryKey refuses a table whose primary key is not exactly one
// column. The templates address a row by a single key - one field on the
// model, one path parameter, one value per row in a delete - so a table with
// no primary key, or a composite one, has no shape they can render: generating
// it anyway writes a model whose GetId names nothing, or silently keys the
// whole table on whichever key column happened to be imported last.
func requireSinglePrimaryKey(tab tools.SysTables) error {
	var keys []string
	for _, c := range tab.Columns {
		if c.PK {
			keys = append(keys, c.ColumnName)
		}
	}
	switch len(keys) {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("表 %s 没有主键，代码生成需要恰好一个主键列", tab.TBName)
	default:
		return fmt.Errorf("表 %s 是联合主键（%s），代码生成需要恰好一个主键列", tab.TBName, strings.Join(keys, ", "))
	}
}

// renderAll executes each template against data and returns the outputs in
// the same order, stopping at the first template that fails. The caller
// writes nothing until every template has rendered: a template that fails
// part-way leaves a truncated file behind it, which compiles as nothing and
// at a glance looks like the real thing.
func renderAll(data any, tpls ...*template.Template) ([][]byte, error) {
	out := make([][]byte, len(tpls))
	for i, t := range tpls {
		var b bytes.Buffer
		if err := t.Execute(&b, data); err != nil {
			return nil, err
		}
		out[i] = b.Bytes()
	}
	return out, nil
}

// writeGenerated writes content to name, a slash-separated path under root,
// creating root and the directories in between.
//
// It goes through os.Root, so name cannot leave root: a ".." component, an
// absolute path, or a symlink under root that points outside it is refused
// rather than followed. root itself is configuration (the working directory,
// or gen.frontpath), and may be or contain a symlink.
//
// It replaced pkg.FileCreate here while that returned no error and, when the
// file could not be created, ended the process with log.Fatalln - one
// unwritable path took the whole server down with it. motor-core v2.11.0
// made pkg.FileCreate return the error.
func writeGenerated(root, name string, content []byte) error {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	name = filepath.FromSlash(name)
	if err := r.MkdirAll(filepath.Dir(name), os.ModePerm); err != nil {
		return err
	}
	return r.WriteFile(name, content, 0o644)
}

type generatorTemplate struct {
	path, message string
	language      bool
}

func loadGeneratorTemplates(specs []generatorTemplate) ([]*template.Template, string, error) {
	templates := make([]*template.Template, 0, len(specs))
	for _, spec := range specs {
		var parsed *template.Template
		var err error
		if spec.language {
			parsed, err = parseGenTemplate(spec.path)
		} else {
			parsed, err = template.ParseFiles(spec.path)
		}
		if err != nil {
			return nil, spec.message, err
		}
		templates = append(templates, parsed)
	}
	return templates, "", nil
}
func previewTemplateSpecs() []generatorTemplate {
	return []generatorTemplate{
		{"template/v4/model.go.template", "model模版读取失败", false},
		{"template/v4/no_actions/apis.go.template", "api模版读取失败", false},
		{"template/v4/ts.go.template", "ts模版读取失败", false},
		{"template/v4/vue.go.template", "vue模版读取失败", false},
		{"template/v4/no_actions/router_check_role.go.template", "路由模版读取失败", false},
		{"template/v4/dto.go.template", "dto模版读取失败", false},
		{"template/v4/no_actions/service.go.template", "service模版读取失败", false},
		{"template/v4/lang-zh.go.template", "zh语言包模版读取失败", true},
		{"template/v4/lang-en.go.template", "en语言包模版读取失败", true},
	}
}
func generationTemplateSpecs(routerFile string) []generatorTemplate {
	return []generatorTemplate{
		{"template/v4/model.go.template", "model模版读取失败", false},
		{"template/v4/no_actions/apis.go.template", "api模版读取失败", false},
		{routerFile, "路由模版失败", false},
		{"template/v4/ts.go.template", "ts模版解析失败", false},
		{"template/v4/vue.go.template", "vue模版解析失败", false},
		{"template/v4/dto.go.template", "dto模版解析失败失败", false},
		{"template/v4/no_actions/service.go.template", "service模版失败", false},
		{"template/v4/lang-zh.go.template", "zh语言包模版解析失败", true},
		{"template/v4/lang-en.go.template", "en语言包模版解析失败", true},
	}
}
func generatedParentMenu(tab tools.SysTables) dto.SysMenuInsertReq {
	return dto.SysMenuInsertReq{
		Title:     tab.TableComment,
		Icon:      "pass",
		Path:      "/" + tab.MLTBName,
		MenuType:  "M",
		Action:    "无",
		ParentID:  0,
		NoCache:   false,
		Component: "Layout",
		Sort:      0,
		Visible:   "0",
		IsFrame:   "0",
		CreateBy:  1}
}
func generatedChildMenu(tab tools.SysTables, parentID int) dto.SysMenuInsertReq {
	return dto.SysMenuInsertReq{
		MenuName:   tab.ClassName + "Manage",
		Title:      tab.TableComment,
		Icon:       "pass",
		Path:       "/" + tab.PackageName + "/" + tab.MLTBName,
		MenuType:   "C",
		Action:     "无",
		Permission: tab.PackageName + ":" + tab.BusinessName + ":list",
		ParentID:   parentID,
		NoCache:    false,
		Component:  "/" + tab.PackageName + "/" + tab.MLTBName + "/index",
		Sort:       0,
		Visible:    "0",
		IsFrame:    "0",
		CreateBy:   1,
		UpdateBy:   1}
}
func generatedActionMenu(tab tools.SysTables, parentID int, title, permission string) dto.SysMenuInsertReq {
	return dto.SysMenuInsertReq{MenuName: "", Title: title + tab.TableComment, Icon: "", Path: tab.TBName, MenuType: "F", Action: "无", Permission: tab.PackageName + ":" + tab.BusinessName + ":" + permission, ParentID: parentID, NoCache: false, Sort: 0, Visible: "0", IsFrame: "0", CreateBy: 1, UpdateBy: 1}
}

// writeFiles renders every template for tab and writes the files, and
// reports whether it did. On false it has already written the error response,
// and nothing has been written to disk if a template failed to render.
func (e Generator) writeFiles(c *gin.Context, tab tools.SysTables) bool {
	e.API = api.API{Context: c, Logger: logger.NewHelper(logger.Default())}
	log := e.Logger
	tab.MLTBName = strings.ReplaceAll(tab.TBName, "_", "-")
	if err := requireSinglePrimaryKey(tab); err != nil {
		e.Error(500, err, err.Error())
		return false
	}
	// Checked again here, not only on save: a configuration saved before the
	// save-time check existed is still in sys_tables.
	if err := validateGenPathFields(tab); err != nil {
		e.Error(500, err, err.Error())
		return false
	}
	// R2: see the matching call and comment in Preview above.
	applyInferredColumnWidths(tab.Columns)

	basePath := "template/v4/"
	routerFile := basePath + "no_actions/router_check_role.go.template"

	if tab.IsAuth == 2 {
		routerFile = basePath + "no_actions/router_no_check_role.go.template"
	}

	templates, message, err := loadGeneratorTemplates(generationTemplateSpecs(routerFile))
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("%s！错误详情：%s", message, err.Error()))
		return false
	}

	out, err := renderAll(tab, templates...)
	if err != nil {
		log.Error(err)
		e.Error(500, err, fmt.Sprintf("模版渲染失败！错误详情：%s", err.Error()))
		return false
	}

	// gen/{PackageName}/ nests under each locale so go-admin-ui's
	// gen-namespace.ts (`./*/*.ts` glob, one level under gen/) picks the file
	// up - a flat gen/{BusinessName}.ts would let two tables in different
	// packages silently overwrite each other's translations, since
	// BusinessName only has a pattern check, no uniqueness check.
	back, front := ".", config.Current().Settings.Gen.FrontPath
	files := []struct {
		root, name string
		content    []byte
	}{
		{back, "app/" + tab.PackageName + "/models/" + tab.TBName + ".go", out[0]},
		{back, "app/" + tab.PackageName + "/handler/" + tab.TBName + ".go", out[1]},
		{back, "app/" + tab.PackageName + "/router/" + tab.TBName + ".go", out[2]},
		{front, "api/" + tab.PackageName + "/" + tab.MLTBName + ".ts", out[3]},
		{front, "views/" + tab.PackageName + "/" + tab.MLTBName + "/index.vue", out[4]},
		{back, "app/" + tab.PackageName + "/service/dto/" + tab.TBName + ".go", out[5]},
		{back, "app/" + tab.PackageName + "/service/" + tab.TBName + ".go", out[6]},
		{front, "lang/zh-CN/gen/" + tab.PackageName + "/" + tab.BusinessName + ".ts", out[7]},
		{front, "lang/en-US/gen/" + tab.PackageName + "/" + tab.BusinessName + ".ts", out[8]},
	}
	for _, f := range files {
		if err := writeGenerated(f.root, f.name, f.content); err != nil {
			log.Error(err)
			e.Error(500, err, fmt.Sprintf("生成文件写入失败！错误详情：%s", err.Error()))
			return false
		}
	}
	return true
}

// genApiToFile writes the migration that seeds tab's menu and APIs, and
// reports whether it did; on false the error response is already written.
func (e Generator) writeMigration(c *gin.Context, tab tools.SysTables) bool {
	var err error
	e.API, err = request.New(c)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return false
	}

	basePath := "template/"

	t1, err := template.ParseFiles(basePath + "api_migrate.template")
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("数据迁移模版解析失败！错误详情：%s", err.Error()))
		return false
	}
	i := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	out, err := renderAll(struct {
		tools.SysTables
		GenerateTime string
	}{tab, i}, t1)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("数据迁移模版渲染失败！错误详情：%s", err.Error()))
		return false
	}
	if err := writeGenerated(".", "cmd/migrate/migration/version-local/"+i+"_migrate.go", out[0]); err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("数据迁移文件写入失败！错误详情：%s", err.Error()))
		return false
	}
	return true
}
