package apis

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
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service"
	"go-admin/common/actions"
)

func profileEngine(db *gorm.DB, id int) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger))
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(id), "rolekey": "ordinary-role", "roleid": float64(2), "datascope": "5", "deptid": float64(1)})
		c.Next()
	})
	r.Use(actions.PermissionAction())
	r.POST("/avatar", SysUser{}.InsetAvatar)
	r.PUT("/profile", SysUser{}.Update)
	r.PUT("/password", SysUser{}.UpdatePwd)
	return r
}

func TestAvatarUsesTokenIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		missing bool
	}{
		{name: "data permission disabled"},
		{name: "self scope", enabled: true},
		{name: "missing user", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := config.ApplicationConfig.EnableDP
			config.ApplicationConfig.EnableDP = tc.enabled
			t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
			db, _ := setupPrivescDB(t)
			u := models.SysUser{UserId: 42, Username: "avatar-user", Password: "test-password", Status: "2"}
			u.CreateBy = 1
			if !tc.missing {
				if err := db.Create(&u).Error; err != nil {
					t.Fatal(err)
				}
			}
			other := models.SysUser{UserId: 43, Username: "other-avatar", Status: "2"}
			if err := db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			if err := os.MkdirAll("static/uploadfile", 0700); err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			if err := mw.WriteField("userId", "43"); err != nil {
				t.Fatal(err)
			}
			w, err := mw.CreateFormFile("upload[]", "photo.jpg")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Write([]byte("avatar fixture")); err != nil {
				t.Fatal(err)
			}
			if err := mw.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/avatar", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			rec := httptest.NewRecorder()
			profileEngine(db, u.UserId).ServeHTTP(rec, req)
			want := 200
			if tc.missing {
				want = 500
			}
			assertProfileCode(t, rec, want)
			if !tc.missing {
				var after models.SysUser
				if err := db.First(&after, u.UserId).Error; err != nil {
					t.Fatal(err)
				}
				if after.Avatar == "" || after.CreateBy != 1 || after.UpdateBy != 42 {
					t.Fatalf("self avatar not saved correctly: %+v", after)
				}
			}
			var otherAfter models.SysUser
			if err := db.First(&otherAfter, other.UserId).Error; err != nil {
				t.Fatal(err)
			}
			if otherAfter.Avatar != "" {
				t.Fatal("avatar request modified another user")
			}
		})
	}
}

func TestSelfPasswordIgnoresCreatorScope(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db, _ := setupPrivescDB(t)
	u := models.SysUser{UserId: 42, Username: "self-user", Password: "old-password", Status: "2"}
	u.CreateBy = 1
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	s := service.SysUser{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	err := s.UpdatePwd(u.UserId, "old-password", "new-password", &actions.DataPermission{UserId: u.UserId, DataScope: actions.DataScopeSelf})
	if err != nil {
		t.Fatalf("user created by admin cannot change own password: %v", err)
	}
}

func TestSelfProfileUpdateWithDataPermission(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db, _ := setupPrivescDB(t)
	u := models.SysUser{UserId: 42, Username: "profile-user", Password: "old-password", RoleId: 2, DeptId: 1, Status: "2"}
	u.CreateBy = 1
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/profile", strings.NewReader(`{"userId":42,"username":"profile-user","nickName":"Updated","phone":"13800000000","email":"user@example.com","roleId":1,"deptId":999,"status":"1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	profileEngine(db, u.UserId).ServeHTTP(rec, req)
	assertProfileCode(t, rec, 200)
	var after models.SysUser
	if err := db.First(&after, u.UserId).Error; err != nil {
		t.Fatal(err)
	}
	if after.NickName != "Updated" || after.RoleId != 2 || after.DeptId != 1 || after.Status != "2" || after.CreateBy != 1 {
		t.Fatalf("self update changed privileged fields or failed: %+v", after)
	}
}

func TestProfileServiceFailuresHaveStructuredResponses(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = false
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db, _ := setupPrivescDB(t)
	req := httptest.NewRequest(http.MethodPut, "/profile", strings.NewReader(`{"userId":42,"username":"absent","nickName":"Missing","phone":"13800000000","email":"user@example.com","deptId":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	profileEngine(db, 42).ServeHTTP(rec, req)
	assertProfileCode(t, rec, 500)
}

func TestAvatarMalformedUploadHasStructuredResponse(t *testing.T) {
	db, _ := setupPrivescDB(t)
	req := httptest.NewRequest(http.MethodPost, "/avatar", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	profileEngine(db, 42).ServeHTTP(rec, req)
	assertProfileCode(t, rec, 422)
}

func TestSelfPasswordRequestUsesTokenIdentity(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	for _, tc := range []struct {
		name, oldPassword, newPassword string
		want                           int
	}{
		{"valid change", "old-password", "new-password", 200},
		{"wrong old password", "wrong-password", "new-password", 403},
		{"bcrypt length rejected", "old-password", strings.Repeat("x", 73), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := setupPrivescDB(t)
			u := models.SysUser{UserId: 42, Username: "password-user", Password: "old-password", Status: "2"}
			u.CreateBy = 1
			other := models.SysUser{UserId: 43, Username: "other-user", Password: "other-password", Status: "2"}
			if err := db.Create(&u).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]any{"userId": 43, "oldPassword": tc.oldPassword, "newPassword": tc.newPassword})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			profileEngine(db, u.UserId).ServeHTTP(rec, req)
			assertProfileCode(t, rec, tc.want)
			var after, otherAfter models.SysUser
			if err := db.First(&after, u.UserId).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&otherAfter, other.UserId).Error; err != nil {
				t.Fatal(err)
			}
			wantPassword := "old-password"
			if tc.want == 200 {
				wantPassword = tc.newPassword
			}
			if err := bcrypt.CompareHashAndPassword([]byte(after.Password), []byte(wantPassword)); err != nil {
				t.Fatal("own password does not match expected credential")
			}
			if otherAfter.Password != other.Password {
				t.Fatal("password endpoint changed another user")
			}
		})
	}
}

func assertProfileCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	var result struct{ Code int }
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid structured response: %q: %v", rec.Body.String(), err)
	}
	if result.Code != want {
		t.Fatalf("response=%s want code %d", rec.Body.String(), want)
	}
}
