package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/runtime"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/app/admin/models"
)

const registrySwagger = `{"paths":{
	"/api/v1/tenants/{tenant}/devices/{id}":{"parameters":[],"get":{"summary":"device detail"}},
	"/api/v1/files/{path}":{"get":{"summary":"download"}},
	"/api/v1/new":{"post":{"summary":"create record"}}
}}`

func registryDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.AutoMigrate(&models.SysAPI{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func registryMessage(t *testing.T, routes ...runtime.Router) storage.Message {
	t.Helper()
	data, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	return storage.Message{Values: map[string]any{"routes": string(data)}}
}

func TestSyncAPIsUsesGeneratedSwagger(t *testing.T) {
	db := registryDatabase(t)
	previous := sdk.Runtime
	sdk.Runtime = runtime.New()
	sdk.Runtime.SetDB("tenant", db)
	t.Cleanup(func() { sdk.Runtime = previous })

	message := registryMessage(t, runtime.Router{
		HTTPMethod: "GET", RelativePath: "/api/v1/app-config", Handler: "github.com/gatework/motor/app/admin/apis.SysLoginLog.Get-fm",
	})
	if err := SyncAPIs(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	var row models.SysAPI
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Title == "" || row.Path != "/api/v1/app-config" || row.Handle != "GET /api/v1/app-config" {
		t.Fatalf("generated swagger was not used: %+v", row)
	}
}

func TestSyncAPIsFiltersInfrastructureAndPreservesMetadata(t *testing.T) {
	databases := map[string]*gorm.DB{"first": registryDatabase(t), "second": registryDatabase(t)}
	for _, db := range databases {
		row := models.SysAPI{
			Path: "/api/v1/tenants/:tenant/devices/:id", Action: "GET",
			Handle: "manual handler", Title: "manual title", Type: "BUS", AppCode: "inventory",
		}
		row.CreateBy, row.UpdateBy = 7, 8
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	routes := []runtime.Router{
		{HTTPMethod: "GET", RelativePath: "/api/v1/tenants/:tenant/devices/:id", Handler: "new handler"},
		{HTTPMethod: "GET", RelativePath: "/api/v1/files/*path", Handler: "file"},
		{HTTPMethod: "POST", RelativePath: "/api/v1/new", Handler: "create"},
		{HTTPMethod: "GET", RelativePath: "/api/v1/without-docs", Handler: "extension"},
		{HTTPMethod: "GET", RelativePath: "/api/v1/static-report", Handler: "business"},
	}
	for _, path := range []string{
		"/swagger", "/swagger/index.html", "/static", "/static/example.txt",
		"/form-generator/index.html", "/api/v1/sys/tables", "/api/v1/sys/tables/info",
		"/api/v1/health", "/api/v1/ready", "/api/v1/metrics",
	} {
		routes = append(routes, runtime.Router{HTTPMethod: "GET", RelativePath: path})
	}
	for _, method := range []string{"HEAD", "OPTIONS"} {
		routes = append(routes, runtime.Router{HTTPMethod: method, RelativePath: "/api/v1/new"})
	}
	message := registryMessage(t, routes...)
	for range 2 {
		if err := syncAPIs(t.Context(), message, registrySwagger, databases); err != nil {
			t.Fatal(err)
		}
	}
	for tenant, db := range databases {
		rows := []models.SysAPI{}
		if err := db.Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 5 {
			t.Fatalf("tenant %s has %d routes, want 5 business routes", tenant, len(rows))
		}
		manual := rows[0]
		if manual.Handle != "manual handler" || manual.Title != "manual title" || manual.Type != "BUS" ||
			manual.AppCode != "inventory" || manual.CreateBy != 7 || manual.UpdateBy != 8 {
			t.Fatalf("tenant %s lost operator metadata: %+v", tenant, manual)
		}
		if rows[1].Title != "download" || rows[2].Title != "create record" || rows[3].Title != "" {
			t.Fatalf("tenant %s has unexpected summaries: %+v", tenant, rows)
		}
	}
}

func TestSyncAPIsRejectsInvalidInputsBeforeWriting(t *testing.T) {
	valid := registryMessage(t, runtime.Router{HTTPMethod: "POST", RelativePath: "/api/v1/new"})
	tests := []struct {
		name     string
		message  storage.Message
		document string
	}{
		{name: "missing routes", message: storage.Message{}, document: registrySwagger},
		{name: "non-string routes", message: storage.Message{Values: map[string]any{"routes": 1}}, document: registrySwagger},
		{name: "invalid json", message: storage.Message{Values: map[string]any{"routes": "["}}, document: registrySwagger},
		{name: "null routes", message: storage.Message{Values: map[string]any{"routes": "null"}}, document: registrySwagger},
		{name: "invalid swagger", message: valid, document: "{"},
		{name: "missing swagger paths", message: valid, document: "{}"},
		{name: "invalid swagger operation", message: valid, document: `{"paths":{"/api/v1/new":{"post":42}}}`},
		{
			name: "late invalid route",
			message: registryMessage(t,
				runtime.Router{HTTPMethod: "POST", RelativePath: "/api/v1/new"},
				runtime.Router{HTTPMethod: "GET", RelativePath: "relative/path"},
			),
			document: registrySwagger,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := registryDatabase(t)
			if err := syncAPIs(t.Context(), test.message, test.document, map[string]*gorm.DB{"tenant": db}); err == nil {
				t.Fatal("invalid registry input was accepted")
			}
			var count int64
			if err := db.Model(&models.SysAPI{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("invalid input wrote %d API rows", count)
			}
		})
	}
}

func TestSyncAPIsRollsBackTenantOnWriteFailure(t *testing.T) {
	db := registryDatabase(t)
	failed := errors.New("write unavailable")
	if err := db.Callback().Create().Before("gorm:create").Register("fail_second_api", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*models.SysAPI); ok && row.Path == "/api/v1/fail" {
			tx.AddError(failed)
		}
	}); err != nil {
		t.Fatal(err)
	}
	message := registryMessage(t,
		runtime.Router{HTTPMethod: "POST", RelativePath: "/api/v1/new"},
		runtime.Router{HTTPMethod: "POST", RelativePath: "/api/v1/fail"},
	)
	err := syncAPIs(t.Context(), message, registrySwagger, map[string]*gorm.DB{"tenant": db})
	if !errors.Is(err, failed) {
		t.Fatalf("write failure was lost: %v", err)
	}
	var count int64
	if err := db.Model(&models.SysAPI{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed transaction left %d API rows", count)
	}
}

func TestSyncAPIsRespectsCancellation(t *testing.T) {
	db := registryDatabase(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := syncAPIs(ctx, registryMessage(t), registrySwagger, map[string]*gorm.DB{"tenant": db})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled synchronization returned %v", err)
	}
}
