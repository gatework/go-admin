package router_test

import (
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"go-admin/app/admin/models"
	"go-admin/app/admin/service"
	"go-admin/app/admin/service/dto"
	"go-admin/common/actions"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"go-admin/common/middleware"
	"go-admin/common/middleware/handler"
)

func TestJWTConfigurationReloadKeepsOneKeySnapshot(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "snapshot.test", "snapshot-admin")
	token := tokenFor(t, mw, db, "snapshot-admin", "snapshot.test")
	config.JwtConfig.Secret = "replacement-test-key"
	if w := request(userRouter(mw), "snapshot.test", token); !strings.Contains(w.Body.String(), "snapshot-admin") {
		t.Fatalf("config reload split signer and credential stamp: %s", w.Body.String())
	}
	restart, err := middleware.AuthInit()
	if err != nil {
		t.Fatal(err)
	}
	if w := request(userRouter(restart), "snapshot.test", token); strings.Contains(w.Body.String(), "snapshot-admin") {
		t.Fatal("restarted middleware accepted old key")
	}
	fresh := tokenFor(t, restart, db, "snapshot-admin", "snapshot.test")
	if w := request(userRouter(restart), "snapshot.test", fresh); !strings.Contains(w.Body.String(), "snapshot-admin") {
		t.Fatalf("new-key login failed: %s", w.Body.String())
	}
}

func TestLogoutRevokesAllUserSessionsDurably(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "logout.test", "logout-admin")
	first := tokenFor(t, mw, db, "logout-admin", "logout.test")
	second := tokenFor(t, mw, db, "logout-admin", "logout.test")
	r := gin.New()
	r.Use(middleware.WithContextDb)
	r.POST("/logout", mw.MiddlewareFunc(), handler.LogOut)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Host = "logout.test"
	req.Header.Set("Authorization", "Bearer "+first)
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if !strings.Contains(out.Body.String(), `"code":200`) {
		t.Fatalf("logout failed: %s", out.Body.String())
	}
	restarted, err := middleware.AuthInit()
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if w := request(userRouter(restarted), "logout.test", token); strings.Contains(w.Body.String(), "logout-admin") {
			t.Fatalf("logged-out token still works: %s", w.Body.String())
		}
	}
	fresh := tokenFor(t, restarted, db, "logout-admin", "logout.test")
	if w := request(userRouter(restarted), "logout.test", fresh); !strings.Contains(w.Body.String(), "logout-admin") {
		t.Fatalf("fresh login denied: %s", w.Body.String())
	}
}

func TestDisableEnableDoesNotReviveTokens(t *testing.T) {
	for _, kind := range []string{"user status", "user update", "role status", "role update"} {
		t.Run(kind, func(t *testing.T) {
			mw := auth(t)
			db := dbFor(t, "lifecycle.test", "lifecycle-admin")
			token := tokenFor(t, mw, db, "lifecycle-admin", "lifecycle.test")
			users := service.SysUser{}
			users.Orm = db
			users.Log = logger.NewHelper(logger.DefaultLogger)
			roles := service.SysRole{}
			roles.Orm = db
			roles.Log = users.Log
			set := func(status string) {
				t.Helper()
				var err error
				switch kind {
				case "user status":
					err = users.UpdateStatus(&dto.UpdateSysUserStatusReq{UserId: 1, Status: status}, &actions.DataPermission{DataScope: actions.DataScopeAll})
				case "user update":
					err = users.Update(&dto.SysUserUpdateReq{UserId: 1, Username: "lifecycle-admin", RoleId: 1, DeptId: 1, Status: status}, &actions.DataPermission{DataScope: actions.DataScopeAll}, 99)
				case "role status":
					err = roles.UpdateStatus(&dto.UpdateStatusReq{RoleId: 1, Status: status})
				case "role update":
					if migrationErr := db.AutoMigrate(&models.SysApi{}, &models.SysMenu{}, &gormadapter.CasbinRule{}); migrationErr != nil {
						t.Fatal(migrationErr)
					}
					err = roles.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleName: "admin", RoleKey: "admin", DataScope: "1", Status: status}, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			set("2")
			if w := request(userRouter(mw), "lifecycle.test", token); !strings.Contains(w.Body.String(), "lifecycle-admin") {
				t.Fatalf("same status revoked token: %s", w.Body.String())
			}
			set("1")
			set("2")
			if w := request(userRouter(mw), "lifecycle.test", token); strings.Contains(w.Body.String(), "lifecycle-admin") {
				t.Fatal("disable-enable revived old token")
			}
			fresh := tokenFor(t, mw, db, "lifecycle-admin", "lifecycle.test")
			if w := request(userRouter(mw), "lifecycle.test", fresh); !strings.Contains(w.Body.String(), "lifecycle-admin") {
				t.Fatal("fresh token denied")
			}
		})
	}
}

func TestProfileDoesNotResetRevocationOrRevokeCurrentSession(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "profile-session.test", "profile-admin")
	if err := db.Table("sys_user").Where("user_id=1").UpdateColumn("auth_version", 7).Error; err != nil {
		t.Fatal(err)
	}
	token := tokenFor(t, mw, db, "profile-admin", "profile-session.test")
	users := service.SysUser{}
	users.Orm = db
	users.Log = logger.NewHelper(logger.DefaultLogger)
	if err := users.Update(&dto.SysUserUpdateReq{UserId: 1, Username: "profile-admin", NickName: "Updated"}, nil, 1); err != nil {
		t.Fatal(err)
	}
	if w := request(userRouter(mw), "profile-session.test", token); !strings.Contains(w.Body.String(), "profile-admin") {
		t.Fatalf("ordinary profile revoked token: %s", w.Body.String())
	}
	var user models.SysUser
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	user.AuthVersion = 0
	if err := db.Select("*").Updates(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.AuthVersion != 7 {
		t.Fatalf("Select(*) overwrote server auth version: %d", user.AuthVersion)
	}
}

func TestResetPasswordDoesNotRecreateDeletedAccount(t *testing.T) {
	auth(t)
	db := dbFor(t, "reset-delete.test", "deleted-during-reset")
	users := service.SysUser{}
	users.Orm = db
	users.Log = logger.NewHelper(logger.DefaultLogger)
	var deleted bool
	if err := db.Callback().Query().After("gorm:query").Register("delete-account-after-read", func(tx *gorm.DB) {
		if deleted || tx.Statement.Table != "sys_user" {
			return
		}
		deleted = true
		if err := db.Session(&gorm.Session{NewDB: true}).Delete(&models.SysUser{}, 1).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := users.ResetPwd(&dto.ResetSysUserPwdReq{UserId: 1, Password: "replacement-password"}, &actions.DataPermission{DataScope: actions.DataScopeAll})
	if err == nil {
		t.Fatal("reset reported success after account was deleted")
	}
	var count int64
	if err := db.Model(&models.SysUser{}).Where("user_id=1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("password reset resurrected deleted account")
	}
}

func TestPostgresSessionRevocation(t *testing.T) {
	dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GO_ADMIN_TEST_POSTGRES_DSN is required")
	}
	mw := auth(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.Exec("DROP SCHEMA IF EXISTS auth_runtime_test CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE SCHEMA auth_runtime_test").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SET search_path TO auth_runtime_test").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA auth_runtime_test CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(&models.SysDept{}, &models.SysRole{}, &models.SysPost{}, &models.SysUser{}); err != nil {
		t.Fatal(err)
	}
	role := models.SysRole{RoleId: 1, RoleName: "admin", RoleKey: "admin", Status: "2", DataScope: "1"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SysDept{DeptId: 1, DeptName: "Test department"}).Error; err != nil {
		t.Fatal(err)
	}
	account := models.SysUser{UserId: 1, Username: "postgres-admin", Password: "Audit-password-1", RoleId: 1, DeptId: 1, Status: "2"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	sdk.Runtime.SetDbByTenant("postgres.test", db)
	token := tokenFor(t, mw, db, "postgres-admin", "postgres.test")
	users := service.SysUser{}
	users.Orm = db
	users.Log = logger.NewHelper(logger.DefaultLogger)
	for _, status := range []string{"1", "2"} {
		if err := users.UpdateStatus(&dto.UpdateSysUserStatusReq{UserId: 1, Status: status}, &actions.DataPermission{DataScope: actions.DataScopeAll}); err != nil {
			t.Fatal(err)
		}
	}
	if w := request(userRouter(mw), "postgres.test", token); strings.Contains(w.Body.String(), "postgres-admin") {
		t.Fatal("PostgreSQL disable-enable revived token")
	}
	fresh := tokenFor(t, mw, db, "postgres-admin", "postgres.test")
	if w := request(userRouter(mw), "postgres.test", fresh); !strings.Contains(w.Body.String(), "postgres-admin") {
		t.Fatalf("fresh token failed: %s", w.Body.String())
	}
	r := gin.New()
	r.Use(middleware.WithContextDb)
	r.POST("/logout", mw.MiddlewareFunc(), handler.LogOut)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Host = "postgres.test"
	req.Header.Set("Authorization", "Bearer "+fresh)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if !strings.Contains(response.Body.String(), `"code":200`) {
		t.Fatalf("logout failed: %s", response.Body.String())
	}
	if w := request(userRouter(mw), "postgres.test", fresh); strings.Contains(w.Body.String(), "postgres-admin") {
		t.Fatal("PostgreSQL logout did not revoke token")
	}
}

func TestResetPasswordKeepsSessionVersionAndHashesPassword(t *testing.T) {
	auth(t)
	db := dbFor(t, "reset-positive.test", "reset-admin")
	if err := db.Table("sys_user").Where("user_id=1").UpdateColumn("auth_version", 7).Error; err != nil {
		t.Fatal(err)
	}
	users := service.SysUser{}
	users.Orm = db
	users.Log = logger.NewHelper(logger.DefaultLogger)
	if err := users.ResetPwd(&dto.ResetSysUserPwdReq{UserId: 1, Password: "replacement-password"}, &actions.DataPermission{DataScope: actions.DataScopeAll}); err != nil {
		t.Fatal(err)
	}
	var account models.SysUser
	if err := db.First(&account, 1).Error; err != nil {
		t.Fatal(err)
	}
	if valid, err := pkg.CompareHashAndPassword(account.Password, "replacement-password"); err != nil || !valid {
		t.Fatalf("new password unusable: %v", err)
	}
	if account.AuthVersion != 7 {
		t.Fatalf("password reset replaced session version: %d", account.AuthVersion)
	}
}

func TestRoleReassignmentDoesNotReviveOldToken(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "reassign.test", "reassign-admin")
	if err := db.Create(&models.SysRole{RoleId: 2, RoleKey: "temporary", RoleName: "temporary", Status: "2", DataScope: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	token := tokenFor(t, mw, db, "reassign-admin", "reassign.test")
	s := &service.SysUser{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	for _, roleID := range []int{2, 1} {
		if err := s.Update(&dto.SysUserUpdateReq{UserId: 1, Username: "reassign-admin", RoleId: roleID, DeptId: 1, Status: "2"}, &actions.DataPermission{DataScope: actions.DataScopeAll}, 99); err != nil {
			t.Fatal(err)
		}
	}
	if w := request(userRouter(mw), "reassign.test", token); strings.Contains(w.Body.String(), "reassign-admin") {
		t.Fatalf("revoked token revived after role reassignment cycle: %s", w.Body.String())
	}
}

func TestUserUpdateRechecksPermissionAtWrite(t *testing.T) {
	auth(t)
	db := dbFor(t, "scope.test", "scope-user")
	if err := db.Model(&models.SysUser{}).Where("user_id=1").Update("create_by", 10).Error; err != nil {
		t.Fatal(err)
	}
	moved := false
	if err := db.Callback().Query().After("gorm:query").Register("review:transfer_owner", func(tx *gorm.DB) {
		if !moved && tx.Statement.Table == "sys_user" {
			moved = true
			if err := db.Model(&models.SysUser{}).Where("user_id=1").Update("create_by", 20).Error; err != nil {
				tx.AddError(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	s := &service.SysUser{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	err := s.Update(&dto.SysUserUpdateReq{UserId: 1, Username: "scope-user", NickName: "outside-scope", RoleId: 1, DeptId: 1, Status: "1"}, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 10}, 99)
	var got models.SysUser
	if readErr := db.First(&got, 1).Error; readErr != nil {
		t.Fatal(readErr)
	}
	if err == nil || got.CreateBy != 20 || got.NickName == "outside-scope" || got.Status != "2" || got.AuthVersion != 0 {
		t.Fatalf("scope lost after read but mutation persisted: error=%v creator=%d nickname=%q status=%s auth_version=%d", err, got.CreateBy, got.NickName, got.Status, got.AuthVersion)
	}
}

func TestSecurityIdentityReversalDoesNotReviveTokens(t *testing.T) {
	for _, kind := range []string{"username", "department", "role key", "role scope", "scope endpoint", "custom departments"} {
		t.Run(kind, func(t *testing.T) {
			mw := auth(t)
			db := dbFor(t, "identity.test", "identity-admin")
			if err := db.AutoMigrate(&models.SysApi{}, &models.SysMenu{}, &models.SysDept{}, &gormadapter.CasbinRule{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&[]models.SysDept{{DeptId: 1, DeptName: "original"}, {DeptId: 2, DeptName: "alternate"}}).Error; err != nil {
				t.Fatal(err)
			}
			users := service.SysUser{}
			users.Orm = db
			users.Log = logger.NewHelper(logger.DefaultLogger)
			roles := service.SysRole{}
			roles.Orm = db
			roles.Log = users.Log
			apply := func(alternate bool) {
				t.Helper()
				var err error
				switch kind {
				case "username", "department":
					d := dto.SysUserUpdateReq{UserId: 1, Username: "identity-admin", RoleId: 1, DeptId: 1, Status: "2"}
					if alternate {
						if kind == "username" {
							d.Username = "temporary-name"
						} else {
							d.DeptId = 2
						}
					}
					caller := 99
					if kind == "username" {
						caller = 1
					}
					err = users.Update(&d, &actions.DataPermission{DataScope: actions.DataScopeAll}, caller)
				case "role key", "role scope":
					d := dto.SysRoleUpdateReq{RoleId: 1, RoleName: "admin", RoleKey: "admin", DataScope: "1", Status: "2"}
					if alternate {
						if kind == "role key" {
							d.RoleKey = "temporary-admin"
						} else {
							d.DataScope = "5"
						}
					}
					err = roles.Update(&d, nil)
				default:
					d := dto.RoleDataScopeReq{RoleId: 1, DataScope: "1"}
					if kind == "custom departments" {
						d.DataScope = "2"
						d.DeptIds = []int{1}
						if alternate {
							d.DeptIds = []int{2}
						}
					} else if alternate {
						d.DataScope = "5"
					}
					roles.UpdateDataScope(&d)
					err = roles.Error
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			apply(false)
			token := tokenFor(t, mw, db, "identity-admin", "identity.test")
			apply(false)
			if w := request(userRouter(mw), "identity.test", token); !strings.Contains(w.Body.String(), `"code":200`) {
				t.Fatalf("idempotent identity update revoked session: %s", w.Body.String())
			}
			apply(true)
			apply(false)
			if w := request(userRouter(mw), "identity.test", token); !strings.Contains(w.Body.String(), `"code":403`) {
				t.Fatalf("%s reversal revived session: %s", kind, w.Body.String())
			}
			fresh := tokenFor(t, mw, db, "identity-admin", "identity.test")
			if w := request(userRouter(mw), "identity.test", fresh); !strings.Contains(w.Body.String(), `"code":200`) {
				t.Fatalf("fresh session denied: %s", w.Body.String())
			}
		})
	}
}
