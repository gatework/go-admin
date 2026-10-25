package router_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/go-admin-team/go-admin-core/v2/sdk/runtime"
	"gorm.io/gorm"

	adminapis "go-admin/app/admin/apis"
	"go-admin/app/admin/models"
	fileapis "go-admin/app/other/apis"
	"go-admin/common/actions"
	"go-admin/common/middleware"
	"go-admin/common/middleware/handler"
)

func auth(t *testing.T) *jwt.GinJWTMiddleware {
	t.Helper()
	previousRuntime := sdk.Runtime
	previousMode, previousDP, previousSecret, previousTimeout := config.ApplicationConfig.Mode, config.ApplicationConfig.EnableDP, config.JwtConfig.Secret, config.JwtConfig.Timeout
	sdk.Runtime = runtime.NewConfig()
	t.Cleanup(func() {
		sdk.Runtime = previousRuntime
		config.ApplicationConfig.Mode = previousMode
		config.ApplicationConfig.EnableDP = previousDP
		config.JwtConfig.Secret = previousSecret
		config.JwtConfig.Timeout = previousTimeout
	})
	gin.SetMode(gin.TestMode)
	config.ApplicationConfig.Mode = "prod"
	config.ApplicationConfig.EnableDP = true
	config.JwtConfig.Secret = "audit-test-key-not-a-real-secret"
	config.JwtConfig.Timeout = 3600
	mw, err := middleware.AuthInit()
	if err != nil {
		t.Fatal(err)
	}
	return mw
}

func dbFor(t *testing.T, host, userName string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.SysDept{}, &models.SysRole{}, &models.SysPost{}, &models.SysUser{}); err != nil {
		t.Fatal(err)
	}
	role := models.SysRole{RoleId: 1, RoleKey: "admin", RoleName: "admin", Status: "2", DataScope: "1"}
	if err = db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	u := models.SysUser{UserId: 1, Username: userName, Password: "Audit-password-1", RoleId: 1, Status: "2", DeptId: 1}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	sdk.Runtime.SetDbByTenant(host, db)
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })
	return db
}

func tokenFor(t *testing.T, mw *jwt.GinJWTMiddleware, db *gorm.DB, username, tenant string) string {
	t.Helper()
	login := handler.Login{Username: username, Password: "Audit-password-1"}
	u, r, err := login.GetUser(db)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := mw.TokenGenerator(map[string]interface{}{"user": u, "role": r, "tenant": tenant})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func userRouter(mw *jwt.GinJWTMiddleware) *gin.Engine {
	r := gin.New()
	r.Use(middleware.WithContextDb, func(c *gin.Context) { c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger)); c.Next() })
	r.GET("/api/v1/sys-user", mw.MiddlewareFunc(), middleware.AuthCheckRole(), actions.PermissionAction(), adminapis.SysUser{}.GetPage)
	return r
}

func request(r http.Handler, host, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/sys-user", nil)
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func TestCrossTenantTokenRejected(t *testing.T) {
	mw := auth(t)
	a := dbFor(t, "tenant-a.test", "tenant-a-admin")
	dbFor(t, "tenant-b.test", "tenant-b-sensitive-user")
	token := tokenFor(t, mw, a, "tenant-a-admin", "tenant-a.test")
	w := request(userRouter(mw), "tenant-b.test", token)
	if strings.Contains(w.Body.String(), "tenant-b-sensitive-user") {
		t.Fatalf("cross-tenant token accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestDisabledAndDeletedUserTokenRejected(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "revocation.test", "revoked-admin")
	token := tokenFor(t, mw, db, "revoked-admin", "revocation.test")
	if err := db.Create(&models.SysUser{UserId: 2, Username: "protected-other-user", RoleId: 1, Status: "2"}).Error; err != nil {
		t.Fatal(err)
	}
	r := userRouter(mw)
	if w := request(r, "revocation.test", token); !strings.Contains(w.Body.String(), "protected-other-user") {
		t.Fatalf("valid token rejected: %s", w.Body.String())
	}
	if err := db.Model(&models.SysUser{}).Where("user_id = ?", 1).Update("status", "1").Error; err != nil {
		t.Fatal(err)
	}
	login := handler.Login{Username: "revoked-admin", Password: "Audit-password-1"}
	if _, _, err := login.GetUser(db); err == nil {
		t.Fatal("disabled login unexpectedly allowed")
	}
	w := request(r, "revocation.test", token)
	if strings.Contains(w.Body.String(), "protected-other-user") {
		t.Fatalf("disabled token accepted: %s", w.Body.String())
	}
	if err := db.Delete(&models.SysUser{}, 1).Error; err != nil {
		t.Fatal(err)
	}
	w = request(r, "revocation.test", token)
	if strings.Contains(w.Body.String(), "protected-other-user") {
		t.Fatalf("deleted token accepted: %s", w.Body.String())
	}
}

func TestUploadedHTMLServedAsSandboxedAttachment(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "upload.test", "upload-user")
	token := tokenFor(t, mw, db, "upload-user", "upload.test")
	prior, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prior)
	r := gin.New()
	r.Use(middleware.WithContextDb, middleware.Secure, func(c *gin.Context) { c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger)); c.Next() })
	r.POST("/api/v1/public/uploadFile", mw.MiddlewareFunc(), fileapis.File{}.UploadFile)
	r.Static("/static", "./static")
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "active.html")
	if err != nil {
		t.Fatal(err)
	}
	content := `<!doctype html><script>document.documentElement.dataset.audit="executed"</script>`
	part.Write([]byte(content))
	form.Close()
	req := httptest.NewRequest("POST", "/api/v1/public/uploadFile", &body)
	req.Host = "upload.test"
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("upload response %s: %v", w.Body.String(), err)
	}
	if response.Code != 200 || !strings.HasSuffix(response.Data.Path, ".html") {
		t.Fatalf("upload rejected: %s", w.Body.String())
	}
	get := httptest.NewRequest("GET", response.Data.Path, nil)
	get.Host = "upload.test"
	w = httptest.NewRecorder()
	r.ServeHTTP(w, get)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || w.Body.String() != content {
		t.Fatalf("not inline html: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	if w.Header().Get("Content-Disposition") == "" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("active-content guard missing: %v", w.Header())
	}
}

func TestAuthorizationChangesInvalidateExistingToken(t *testing.T) {
	for _, tc := range []struct {
		name, table, column string
		value               interface{}
	}{
		{"password", "sys_user", "password", "replacement-password-hash"},
		{"department", "sys_user", "dept_id", 2},
		{"role assignment", "sys_user", "role_id", 2},
		{"role key", "sys_role", "role_key", "replaced-admin"},
		{"scope", "sys_role", "data_scope", "5"},
		{"disabled role", "sys_role", "status", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := auth(t)
			db := dbFor(t, "changed.test", "change-admin")
			token := tokenFor(t, mw, db, "change-admin", "changed.test")
			r := userRouter(mw)
			if w := request(r, "changed.test", token); !strings.Contains(w.Body.String(), "change-admin") {
				t.Fatalf("valid token rejected: %s", w.Body.String())
			}
			if err := db.Table(tc.table).Where("1 = 1").Update(tc.column, tc.value).Error; err != nil {
				t.Fatal(err)
			}
			w := request(r, "changed.test", token)
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != 403 {
				t.Fatalf("stale credentials accepted: %s", w.Body.String())
			}
		})
	}
}

func TestWildcardTenantAcceptsItsHostAliases(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "*", "shared-admin")
	token := tokenFor(t, mw, db, "shared-admin", "*")
	for _, host := range []string{"admin.example", "admin.example:8000"} {
		w := request(userRouter(mw), host, token)
		if !strings.Contains(w.Body.String(), "shared-admin") {
			t.Fatalf("wildcard alias rejected: %s", w.Body.String())
		}
	}
}

func TestLegacyUnboundTokenRequiresLogin(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "legacy.test", "legacy-admin")
	token := tokenFor(t, mw, db, "legacy-admin", "")
	w := request(userRouter(mw), "legacy.test", token)
	if strings.Contains(w.Body.String(), "legacy-admin") {
		t.Fatalf("unbound token accepted: %s", w.Body.String())
	}
}
