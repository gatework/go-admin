package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go-admin/app/other/models/tools"
)

const postgresAuditColumns = `name varchar(64) NOT NULL,
	create_by bigint NULL, update_by bigint NULL,
	created_at timestamptz NULL, updated_at timestamptz NULL,
	deleted_at bigint NOT NULL DEFAULT 0`

// Every connection, including the generated-code subprocess, uses its own
// schema. The caller's default schema and tables are never modified.
func newPostgresGenEnv(t *testing.T) genEnv {
	t.Helper()
	dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("GO_ADMIN_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GO_ADMIN_TEST_POSTGRES_DSN is required for PostgreSQL code generation")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	basePool, err := base.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = basePool.Close() })
	schema := fmt.Sprintf("gencode_%d", time.Now().UnixNano())
	if err := base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; err != nil {
			t.Error(err)
		}
	})
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + schema
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	t.Setenv("GO_ADMIN_GENERATED_POSTGRES_DSN", dsn)
	if err := db.AutoMigrate(&tools.SysTables{}, &tools.SysColumns{}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	restore := [3]string{config.DatabaseConfig.Driver, config.GenConfig.DBName, config.GenConfig.FrontPath}
	t.Cleanup(func() {
		config.DatabaseConfig.Driver, config.GenConfig.DBName, config.GenConfig.FrontPath = restore[0], restore[1], restore[2]
	})
	// The request connection is authoritative, not another tenant's global
	// database driver or the MySQL-only generator database setting.
	config.DatabaseConfig.Driver = "mysql"
	config.GenConfig.DBName = "not_this_postgres_database"
	config.GenConfig.FrontPath = filepath.Join(out, "ui")
	if err := os.Symlink(filepath.Join(root, "template"), filepath.Join(out, "template")); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	t.Chdir(out)
	return genEnv{db: db, root: root, out: out}
}

func postgresGenRequest(t *testing.T, db *gorm.DB, h gin.HandlerFunc, target string, params gin.Params) (int, json.RawMessage) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	c.Params = params
	c.Set("db", db)
	h(c)
	var res struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("expected one response for %s: %v, %s", target, err, w.Body.String())
	}
	if res.Code != http.StatusOK {
		t.Logf("%s: %s", target, w.Body.String())
	}
	return res.Code, res.Data
}

func TestPostgresGeneratorPipeline(t *testing.T) {
	g := newPostgresGenEnv(t)
	shapes := []pkShape{
		{Table: "gpg_serial", Class: "GpgSerial", PkColumn: "id", Key: "id bigserial PRIMARY KEY,", Assigned: true, Create: `{"name":"alpha"}`, Missing: "987654"},
		{Table: "gpg_identity", Class: "GpgIdentity", PkColumn: "order_no", Key: "order_no bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,", Assigned: true, Create: `{"name":"alpha"}`, Missing: "987654"},
		{Table: "gpg_assigned", Class: "GpgAssigned", PkColumn: "id", Key: "id bigint PRIMARY KEY,", Create: `{"id":"9007199254740993","name":"alpha"}`, ID: "9007199254740993", IDJSON: `"9007199254740993"`, Missing: "987654"},
		{Table: "gpg_uuid", Class: "GpgUuid", PkColumn: "id", Key: "id uuid PRIMARY KEY,", Create: `{"id":"6ba7b810-9dad-41d1-80b4-00c04fd430c8","name":"alpha"}`, ID: "6ba7b810-9dad-41d1-80b4-00c04fd430c8", IDJSON: `"6ba7b810-9dad-41d1-80b4-00c04fd430c8"`, Missing: "6ba7b811-9dad-41d1-80b4-00c04fd430c8"},
		{Table: "gpg_code", Class: "GpgCode", PkColumn: "code", Key: "code varchar(32) PRIMARY KEY,", Create: `{"code":"c-1","name":"alpha"}`, ID: "c-1", IDJSON: `"c-1"`, Missing: "no-such-code"},
	}
	for _, shape := range shapes {
		if err := g.db.Exec("CREATE TABLE " + shape.Table + " (" + shape.Key + postgresAuditColumns + ")").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := g.db.Exec("COMMENT ON TABLE gpg_serial IS 'PostgreSQL example'").Error; err != nil {
		t.Fatal(err)
	}
	if err := g.db.Exec("COMMENT ON COLUMN gpg_serial.name IS 'Display name'").Error; err != nil {
		t.Fatal(err)
	}
	code, data := postgresGenRequest(t, g.db, Gen{}.GetDBTableList, "/?tableName=gpg_serial&pageSize=10&pageIndex=1", nil)
	var tables struct {
		Count int              `json:"count"`
		List  []tools.DBTables `json:"list"`
	}
	if err := json.Unmarshal(data, &tables); code != 200 || err != nil || tables.Count != 1 || len(tables.List) != 1 || tables.List[0].TableComment != "PostgreSQL example" {
		t.Fatalf("table list: code=%d data=%s err=%v", code, data, err)
	}
	code, data = postgresGenRequest(t, g.db, Gen{}.GetDBColumnList, "/?tableName=gpg_serial&pageSize=20&pageIndex=1", nil)
	var columns struct {
		Count int               `json:"count"`
		List  []tools.DBColumns `json:"list"`
	}
	if err := json.Unmarshal(data, &columns); code != 200 || err != nil || columns.Count != 7 || len(columns.List) != 7 || columns.List[0].ColumnName != "id" || columns.List[1].ColumnComment != "Display name" {
		t.Fatalf("column list: code=%d data=%s err=%v", code, data, err)
	}
	generated := map[string]string{}
	for _, shape := range shapes {
		if code := callHandler(t, g.db, SysTable{}.Insert, "/?tables="+shape.Table, nil); code != 200 {
			t.Fatalf("importing %s: code=%d", shape.Table, code)
		}
		var row tools.SysTables
		if err := g.db.Where("table_name = ?", shape.Table).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		params := gin.Params{{Key: "tableId", Value: itoa(row.TableId)}}
		code, data := postgresGenRequest(t, g.db, Gen{}.Preview, "/preview", params)
		var preview map[string]string
		if err := json.Unmarshal(data, &preview); code != 200 || err != nil || len(preview) != 9 || !strings.Contains(preview["template/model.go.template"], "type "+shape.Class+" struct") {
			t.Fatalf("preview %s: code=%d data=%s err=%v", shape.Table, code, data, err)
		}
		if code := callHandler(t, g.db, Gen{}.GenCode, "/", params); code != 200 {
			t.Fatalf("generating %s: code=%d", shape.Table, code)
		}
		for dst, src := range backendFiles(g.out, g.root, shape.Table) {
			generated[dst] = src
		}
		api, err := os.ReadFile(filepath.Join(g.out, "ui/api/admin", strings.ReplaceAll(shape.Table, "_", "-")+".ts"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(api), "ids.map(Number)") {
			t.Fatalf("exact primary key is coerced to a number: %s", api)
		}
	}
	code, data = postgresGenRequest(t, g.db, Gen{}.GetDBTableList, "/?tableName=gpg_serial", nil)
	if err := json.Unmarshal(data, &tables); code != 200 || err != nil || tables.Count != 0 || len(tables.List) != 0 {
		t.Fatalf("imported table remains a candidate: code=%d data=%s err=%v", code, data, err)
	}
	var buf bytes.Buffer
	if err := crudTest.Execute(&buf, shapes); err != nil {
		t.Fatal(err)
	}
	testSource := strings.NewReplacer(`"gorm.io/driver/mysql"`, `"gorm.io/driver/postgres"`, `mysql.Open(os.Getenv("GO_ADMIN_TEST_MYSQL_DSN"))`, `postgres.Open(os.Getenv("GO_ADMIN_GENERATED_POSTGRES_DSN"))`, `db.WithContext(c)`, `db.WithContext(c.Request.Context())`,
		`id, idJSON := itoaGenerated(n), itoaGenerated(n)`, `id, idJSON := itoaGenerated(n), "\""+itoaGenerated(n)+"\""`,
		`if res.Code != 200 || row["name"] != "alpha" {`, `if res.Code != 200 || row["name"] != "alpha" || (row["id"] != id && row["orderNo"] != id && row["code"] != id) {`).Replace(buf.String())
	testFile := filepath.Join(g.out, "generated_postgres_crud_test.go")
	if err := os.WriteFile(testFile, []byte(testSource), 0600); err != nil {
		t.Fatal(err)
	}
	generated[filepath.Join(g.root, "app/admin/apis/zz_generated_postgres_crud_test.go")] = testFile
	overlay := filepath.Join(g.out, "overlay.json")
	encoded, err := json.Marshal(map[string]any{"Replace": generated})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	goCmd(t, g.root, "vet", "-overlay="+overlay, "./app/admin/...")
	ran := goCmd(t, g.root, "test", "-race", "-overlay="+overlay, "-count=1", "-v", "-run", "^TestGeneratedCRUD_", "./app/admin/apis/")
	for _, shape := range shapes {
		if !strings.Contains(string(ran), "--- PASS: TestGeneratedCRUD_"+shape.Class+" ") {
			t.Errorf("generated CRUD test did not execute for %s", shape.Table)
		}
	}
}

func TestPostgresGeneratorRejectsUnsupportedShapes(t *testing.T) {
	g := newPostgresGenEnv(t)
	for _, fixture := range []struct{ name, columns string }{
		{"gpg_nokey", "ref int," + postgresAuditColumns},
		{"gpg_composite", "a int, b int," + postgresAuditColumns + ", PRIMARY KEY(a,b)"},
		{"gpg_array", "id bigserial PRIMARY KEY, labels text[]," + postgresAuditColumns},
		{"gpg_json", "id bigserial PRIMARY KEY, payload jsonb," + postgresAuditColumns},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if err := g.db.Exec("CREATE TABLE " + fixture.name + " (" + fixture.columns + ")").Error; err != nil {
				t.Fatal(err)
			}
			metadata, err := (&tools.DBColumns{TableName: fixture.name}).GetList(g.db)
			if err != nil || len(metadata) < 7 {
				t.Fatalf("fixture metadata unavailable; refusal would be a hollow pass: %v (%d columns)", err, len(metadata))
			}
			code := callHandler(t, g.db, SysTable{}.Insert, "/?tables="+fixture.name, nil)
			if code == 200 {
				var row tools.SysTables
				if err := g.db.Where("table_name = ?", fixture.name).First(&row).Error; err != nil {
					t.Fatal(err)
				}
				code = callHandler(t, g.db, Gen{}.GenCode, "/", gin.Params{{Key: "tableId", Value: itoa(row.TableId)}})
			}
			if code == 200 {
				t.Fatal("unsupported table emitted code")
			}
			for _, filename := range backendFiles(g.out, g.root, fixture.name) {
				if _, err := os.Stat(filename); !os.IsNotExist(err) {
					t.Fatalf("unsupported table emitted %s (stat error %v)", filename, err)
				}
			}
		})
	}
}

func TestPostgresGeneratorRejectsIncompatibleAuditColumns(t *testing.T) {
	g := newPostgresGenEnv(t)
	for _, fixture := range []struct{ name, columns, field, diagnostic string }{
		{"gpg_missing_audit", strings.Replace(postgresAuditColumns, "create_by bigint NULL,", "", 1), "create_by", "requires audit column"},
		{"gpg_bad_audit_type", strings.Replace(postgresAuditColumns, "created_at timestamptz", "created_at text", 1), "created_at", "incompatible type"},
		{"gpg_identity_audit", strings.Replace(postgresAuditColumns, "create_by bigint NULL", "create_by bigint GENERATED ALWAYS AS IDENTITY", 1), "create_by", "incompatible type"},
		{"gpg_nullable_delete", strings.Replace(postgresAuditColumns, "deleted_at bigint NOT NULL", "deleted_at bigint NULL", 1), "deleted_at", "incompatible type"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if err := g.db.Exec("CREATE TABLE " + fixture.name + " (id bigserial PRIMARY KEY," + fixture.columns + ")").Error; err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/?tables="+fixture.name, nil)
			c.Set("db", g.db)
			SysTable{}.Insert(c)
			var res struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code == 200 || res.Code == 0 || !strings.Contains(w.Body.String(), fixture.field) || !strings.Contains(w.Body.String(), fixture.diagnostic) {
				t.Fatalf("missing actionable audit diagnostic: %s (%v)", w.Body.String(), err)
			}
			var count int64
			if err := g.db.Model(&tools.SysTables{}).Where("table_name = ?", fixture.name).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("rejected audit schema persisted config: count=%d err=%v", count, err)
			}
		})
	}
}

func TestPostgresGeneratorRejectsUnsupportedIdentifiers(t *testing.T) {
	g := newPostgresGenEnv(t)
	for _, fixture := range []struct{ name, columns, identifier, diagnostic string }{
		{"gpgCamel", "id bigserial PRIMARY KEY," + postgresAuditColumns, "gpgCamel", "lowercase"},
		{"gpg_camel_column", `id bigserial PRIMARY KEY, "camelName" text,` + postgresAuditColumns, "camelName", "lowercase"},
		{"order", "id bigserial PRIMARY KEY," + postgresAuditColumns, "order", "reserved keyword"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if err := g.db.Exec(`CREATE TABLE "` + fixture.name + `" (` + fixture.columns + ")").Error; err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/?tables="+fixture.name, nil)
			c.Set("db", g.db)
			SysTable{}.Insert(c)
			var res struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code == 200 || res.Code == 0 || !strings.Contains(w.Body.String(), fixture.identifier) || !strings.Contains(w.Body.String(), fixture.diagnostic) {
				t.Fatalf("missing actionable identifier diagnostic: %s (%v)", w.Body.String(), err)
			}
			var count int64
			if err := g.db.Model(&tools.SysTables{}).Where("table_name = ?", fixture.name).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("rejected identifier persisted config: count=%d err=%v", count, err)
			}
		})
	}
}

func TestPostgresGeneratedScalarCRUD(t *testing.T) {
	g := newPostgresGenEnv(t)
	params := g.importTable(t, "gpg_scalar", `id bigserial PRIMARY KEY,
		enabled boolean NOT NULL, quantity integer NOT NULL, large_count bigint NOT NULL,
		amount numeric(30,4) NOT NULL, ratio double precision NOT NULL,
		event_date date NOT NULL, happened_at timestamptz NOT NULL,
		note text NULL, optional_count integer NULL, payload bytea NULL,
		doubled integer GENERATED ALWAYS AS (quantity * 2) STORED, `+postgresAuditColumns)
	for column, queryType := range map[string]string{"enabled": "EQ", "quantity": "EQ", "optional_count": "EQ", "note": "LIKE"} {
		result := g.db.Model(&tools.SysColumns{}).Where("table_id = ? AND column_name = ?", params[0].Value, column).
			Updates(map[string]any{"is_query": "1", "query": true, "query_type": queryType})
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("configure %s query: rows=%d err=%v", column, result.RowsAffected, result.Error)
		}
	}
	code, data := postgresGenRequest(t, g.db, Gen{}.Preview, "/preview", params)
	var preview map[string]string
	if err := json.Unmarshal(data, &preview); code != 200 || err != nil || len(preview) != 9 {
		t.Fatalf("scalar preview: code=%d data=%s err=%v", code, data, err)
	}
	if code := callHandler(t, g.db, Gen{}.GenCode, "/", params); code != 200 {
		t.Fatalf("generating scalar fixture: code=%d", code)
	}
	// The generated form must preserve scalar JSON types, not submit boolean
	// and numeric text inputs to Go bool/int fields.
	vue := g.page(t, "gpg_scalar")
	for _, required := range []string{"<el-checkbox", "<el-input-number"} {
		if !strings.Contains(vue, required) {
			t.Fatalf("generated scalar editor lacks %s", required)
		}
	}
	if strings.Contains(vue, "form.model.doubled") {
		t.Fatal("generated expression is an editable form field")
	}
	api, err := os.ReadFile(filepath.Join(g.out, "ui/api/admin/gpg-scalar.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"createBy?: number", "updateBy?: number"} {
		if !strings.Contains(string(api), field) {
			t.Fatalf("audit TypeScript type disagrees with embedded model: missing %s in %s", field, api)
		}
	}
	generated := backendFiles(g.out, g.root, "gpg_scalar")
	testFile := filepath.Join(g.out, "generated_postgres_scalar_test.go")
	if err := os.WriteFile(testFile, []byte(postgresScalarCRUD), 0600); err != nil {
		t.Fatal(err)
	}
	generated[filepath.Join(g.root, "app/admin/apis/zz_generated_postgres_scalar_test.go")] = testFile
	encoded, err := json.Marshal(map[string]any{"Replace": generated})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(g.out, "overlay.json")
	if err := os.WriteFile(overlay, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	goCmd(t, g.root, "vet", "-overlay="+overlay, "./app/admin/...")
	ran := goCmd(t, g.root, "test", "-race", "-overlay="+overlay, "-count=1", "-v", "-run", "^TestGeneratedPostgresScalars$", "./app/admin/apis/")
	if !strings.Contains(string(ran), "--- PASS: TestGeneratedPostgresScalars ") {
		t.Fatal("generated scalar CRUD test did not run")
	}
}

const postgresScalarCRUD = `package apis

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "strconv"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func TestGeneratedPostgresScalars(t *testing.T) {
    db, err := gorm.Open(postgres.Open(os.Getenv("GO_ADMIN_GENERATED_POSTGRES_DSN")), &gorm.Config{})
    if err != nil { t.Fatal(err) }
    pool, err := db.DB()
    if err != nil { t.Fatal(err) }
    defer pool.Close()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(func(c *gin.Context) { c.Set("db", db.WithContext(c.Request.Context())); c.Next() })
    api := GpgScalar{}
    r.POST("/x", api.Insert)
    r.GET("/x/:id", api.Get)
    r.GET("/x", api.GetPage)
    r.PUT("/x/:id", api.Update)
    r.DELETE("/x", api.Delete)
    requestStatus := func(method, target, body string) (int, json.RawMessage) {
        t.Helper()
        req := httptest.NewRequest(method, target, strings.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        w := httptest.NewRecorder()
        r.ServeHTTP(w, req)
        var res struct { Code int; Data json.RawMessage }
        if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
            t.Fatalf("%s %s: %s (%v)", method, target, w.Body.String(), err)
        }
        return res.Code, res.Data
    }
    request := func(method, target, body string) json.RawMessage {
        t.Helper()
        code, data := requestStatus(method, target, body)
        if code != 200 { t.Fatalf("%s %s: code=%d data=%s", method, target, code, data) }
        return data
    }
    createBody := "{\"name\":\"alpha\",\"enabled\":true,\"quantity\":7,\"largeCount\":\"9223372036854775806\",\"amount\":\"12345678901234567890.1234\",\"ratio\":1.25,\"eventDate\":\"2026-10-25T00:00:00Z\",\"happenedAt\":\"2026-10-25T12:00:00Z\",\"payload\":\"YWJj\",\"doubled\":999}"
    request(http.MethodPost, "/x", createBody)
    var id int
    if err := db.Table("gpg_scalar").Select("id").Where("name = ?", "alpha").Scan(&id).Error; err != nil || id == 0 { t.Fatalf("insert id=%d err=%v", id, err) }
    // Audit fields come from the framework's embedded ControlBy, whose
    // JSON numbers must agree with the generated TypeScript metadata.
    if err := db.Table("gpg_scalar").Where("id = ?", id).Updates(map[string]interface{}{"create_by":41, "update_by":42}).Error; err != nil { t.Fatal(err) }
    target := "/x/" + strconv.Itoa(id)
    var row map[string]interface{}
    if err := json.Unmarshal(request(http.MethodGet, target, ""), &row); err != nil { t.Fatal(err) }
    if row["createBy"] != float64(41) || row["updateBy"] != float64(42) { t.Fatalf("audit JSON must be numeric: %#v", row) }
    if row["enabled"] != true || row["quantity"] != float64(7) || row["largeCount"] != "9223372036854775806" || row["amount"] != "12345678901234567890.1234" || row["ratio"] != 1.25 || row["doubled"] != float64(14) || row["payload"] != "YWJj" || row["note"] != nil || row["optionalCount"] != nil {
        t.Fatalf("scalar readback changed value/type: %#v", row)
    }
    var page struct { Count int; List []map[string]interface{} }
    if err := json.Unmarshal(request(http.MethodGet, "/x?pageIndex=1&pageSize=10", ""), &page); err != nil || page.Count != 1 || len(page.List) != 1 { t.Fatalf("list: %#v err=%v", page, err) }
    request(http.MethodPut, target, "{\"name\":\"beta\",\"enabled\":false,\"quantity\":0,\"largeCount\":\"0\",\"amount\":\"0.0000\",\"ratio\":0,\"eventDate\":\"2026-10-25T00:00:00Z\",\"happenedAt\":\"2026-10-25T12:00:00Z\",\"note\":\"visible\",\"optionalCount\":0,\"payload\":\"\",\"doubled\":999}")
    if err := json.Unmarshal(request(http.MethodGet, target, ""), &row); err != nil { t.Fatal(err) }
    if row["name"] != "beta" || row["largeCount"] != "0" || row["enabled"] != false || row["quantity"] != float64(0) || row["doubled"] != float64(0) || row["optionalCount"] != float64(0) || row["note"] != "visible" || row["amount"] != "0.0000" {
        t.Fatalf("zero/false/scalar update changed value: %#v", row)
    }
    // A second, contrasting row makes false/zero query omission visible.
    // The nullable LIKE value must be bound as text, never a pointer display.
    request(http.MethodPost, "/x", createBody)
    for _, filter := range []struct { query, name string }{
        {"enabled=false", "beta"}, {"quantity=0", "beta"},
        {"optionalCount=0", "beta"}, {"note=isib", "beta"},
        {"enabled=true", "alpha"}, {"quantity=7", "alpha"},
    } {
        var filtered struct { Count int; List []map[string]interface{} }
        if err := json.Unmarshal(request(http.MethodGet, "/x?pageIndex=1&pageSize=10&"+filter.query, ""), &filtered); err != nil || filtered.Count != 1 || len(filtered.List) != 1 || filtered.List[0]["name"] != filter.name {
            t.Fatalf("filter %s: %#v err=%v", filter.query, filtered, err)
        }
    }
    var otherID int
    if err := db.Table("gpg_scalar").Select("id").Where("name = ? AND deleted_at = 0", "alpha").Scan(&otherID).Error; err != nil || otherID == 0 || otherID == id { t.Fatalf("contrast row id=%d err=%v", otherID, err) }
    request(http.MethodDelete, "/x", "{\"ids\":["+strconv.Quote(strconv.Itoa(otherID))+"]}")
    request(http.MethodDelete, "/x", "{\"ids\":["+strconv.Quote(strconv.Itoa(id))+"]}")
    var count int64
    if err := db.Table("gpg_scalar").Where("deleted_at = 0").Count(&count).Error; err != nil || count != 0 { t.Fatalf("delete count=%d err=%v",count,err) }

    // Delete after the generated service's lookup and before its write. A
    // successful lookup must never allow Save's update-or-insert fallback.
    request(http.MethodPost, "/x", createBody)
    if err := db.Table("gpg_scalar").Select("id").Where("name = ? AND deleted_at = 0", "alpha").Scan(&id).Error; err != nil { t.Fatal(err) }
    armed, deleted := true, false
    var hookErr error
    const hookName = "test:delete_before_generated_update"
    if err := db.Callback().Query().After("gorm:query").Register(hookName, func(tx *gorm.DB) {
        if !armed || tx.Statement.Table != "gpg_scalar" { return }
        armed = false
        result := db.Exec("DELETE FROM gpg_scalar WHERE id = ?", id)
        hookErr, deleted = result.Error, result.RowsAffected == 1
    }); err != nil { t.Fatal(err) }
    defer db.Callback().Query().Remove(hookName)
    code, _ := requestStatus(http.MethodPut, "/x/"+strconv.Itoa(id), createBody)
    if armed || !deleted || hookErr != nil || code == 200 || code == 0 { t.Fatalf("concurrent deletion: armed=%v deleted=%v hook=%v code=%d", armed, deleted, hookErr, code) }
    if err := db.Table("gpg_scalar").Where("id = ?", id).Count(&count).Error; err != nil || count != 0 { t.Fatalf("update recreated a deleted row: count=%d err=%v", count, err) }
}
`
