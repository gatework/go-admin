package router_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/runtime"

	adminhandler "github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/app/admin/models"
	filehandler "github.com/gatework/motor/app/other/handler"
	accountsecurity "github.com/gatework/motor/common/account"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
	"github.com/gatework/motor/common/middleware/auth"
	"github.com/gatework/motor/internal/testconfig"
)

func authMiddleware(t *testing.T) *jwt.GinJWTMiddleware {
	t.Helper()
	previousRuntime := sdk.Runtime
	previousMode, previousDP, previousSecret, previousTimeout := config.Current().Settings.Application.Mode, config.Current().Settings.Application.EnableDP, config.Current().Settings.JWT.Secret, config.Current().Settings.JWT.Timeout
	sdk.Runtime = runtime.New()
	t.Cleanup(func() {
		sdk.Runtime = previousRuntime
		testconfig.Apply(t, func(settings *config.Config) {
			settings.
				Application.Mode = previousMode
		})
		testconfig.Apply(t, func(settings *config.Config) {
			settings.
				Application.EnableDP = previousDP
		})
		testconfig.Apply(t, func(settings *config.Config) {
			settings.
				JWT.Secret = previousSecret
		})
		testconfig.Apply(t, func(settings *config.Config) {
			settings.
				JWT.Timeout = previousTimeout
		})
	})
	gin.SetMode(gin.TestMode)
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.Mode = "prod"
	})
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			JWT.Secret = "audit-test-key-not-a-real-secret"
	})
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			JWT.Timeout = 3600
	})
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
	if err = db.AutoMigrate(&accountsecurity.State{}, &models.SysDept{}, &models.SysRole{}, &models.SysPost{}, &models.SysUser{}); err != nil {
		t.Fatal(err)
	}
	role := models.SysRole{RoleID: 1, RoleKey: "admin", RoleName: "admin", Status: "2", DataScope: "1"}
	if err = db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	u := models.SysUser{UserID: 1, Username: userName, Password: "Audit-password-1", RoleID: 1, Status: "2", DeptID: 1}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	sdk.Runtime.SetDB(host, db)
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })
	return db
}

func tokenFor(t *testing.T, mw *jwt.GinJWTMiddleware, db *gorm.DB, username, tenant string) string {
	t.Helper()
	login := auth.Login{Username: username, Password: "Audit-password-1"}
	u, r, err := login.GetUser(db)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := mw.TokenGenerator(map[string]any{"user": u, "role": r, "tenant": tenant})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func userRouter(mw *jwt.GinJWTMiddleware) *gin.Engine {
	r := gin.New()
	r.Use(middleware.WithContextDB, func(c *gin.Context) { c.Set(pkg.LoggerKey, logger.NewHelper(logger.Default())); c.Next() })
	r.GET("/api/v1/sys-user", mw.MiddlewareFunc(), middleware.AuthCheckRole(), actions.PermissionAction(), adminhandler.User{}.List)
	return r
}

func request(r http.Handler, host, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sys-user", nil)
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func TestCrossTenantTokenRejected(t *testing.T) {
	mw := authMiddleware(t)
	a := dbFor(t, "tenant-a.test", "tenant-a-admin")
	dbFor(t, "tenant-b.test", "tenant-b-sensitive-user")
	token := tokenFor(t, mw, a, "tenant-a-admin", "tenant-a.test")
	w := request(userRouter(mw), "tenant-b.test", token)
	if strings.Contains(w.Body.String(), "tenant-b-sensitive-user") {
		t.Fatalf("cross-tenant token accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestDisabledAndDeletedUserTokenRejected(t *testing.T) {
	mw := authMiddleware(t)
	db := dbFor(t, "revocation.test", "revoked-admin")
	token := tokenFor(t, mw, db, "revoked-admin", "revocation.test")
	if err := db.Create(&models.SysUser{UserID: 2, Username: "protected-other-user", RoleID: 1, Status: "2"}).Error; err != nil {
		t.Fatal(err)
	}
	r := userRouter(mw)
	if w := request(r, "revocation.test", token); !strings.Contains(w.Body.String(), "protected-other-user") {
		t.Fatalf("valid token rejected: %s", w.Body.String())
	}
	if err := db.Model(&models.SysUser{}).Where("user_id = ?", 1).Update("status", "1").Error; err != nil {
		t.Fatal(err)
	}
	login := auth.Login{Username: "revoked-admin", Password: "Audit-password-1"}
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
	mw := authMiddleware(t)
	db := dbFor(t, "upload.test", "upload-user")
	token := tokenFor(t, mw, db, "upload-user", "upload.test")
	t.Chdir(t.TempDir())
	r := gin.New()
	r.Use(middleware.WithContextDB, middleware.Secure, func(c *gin.Context) { c.Set(pkg.LoggerKey, logger.NewHelper(logger.Default())); c.Next() })
	r.POST("/api/v1/public/uploadFile", mw.MiddlewareFunc(), filehandler.File{}.UploadFile)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/uploadFile", &body)
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
	get := httptest.NewRequest(http.MethodGet, response.Data.Path, nil)
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
		value               any
	}{
		{"password", "sys_user", "password", "replacement-password-hash"},
		{"department", "sys_user", "dept_id", 2},
		{"role assignment", "sys_user", "role_id", 2},
		{"role key", "sys_role", "role_key", "replaced-admin"},
		{"scope", "sys_role", "data_scope", "5"},
		{"disabled role", "sys_role", "status", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := authMiddleware(t)
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
	mw := authMiddleware(t)
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
	mw := authMiddleware(t)
	db := dbFor(t, "legacy.test", "legacy-admin")
	token := tokenFor(t, mw, db, "legacy-admin", "")
	w := request(userRouter(mw), "legacy.test", token)
	if strings.Contains(w.Body.String(), "legacy-admin") {
		t.Fatalf("unbound token accepted: %s", w.Body.String())
	}
}
