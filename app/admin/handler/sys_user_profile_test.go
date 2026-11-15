package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gatework/motor/common/account"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/internal/testconfig"
)

func profileEngine(db *gorm.DB, id int) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(pkg.LoggerKey, logger.NewHelper(logger.Default()))
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(id), "rolekey": "ordinary-role", "roleid": float64(2), "datascope": "5", "deptid": float64(1)})
		c.Next()
	})
	r.Use(actions.PermissionAction())
	r.POST("/avatar", User{}.UpdateAvatar)
	r.PUT("/profile", User{}.Update)
	r.PUT("/password", User{}.ChangePassword)
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

			testconfig.Apply(t, func(settings *config.Config) {
				settings.
					Application.EnableDP = tc.enabled
			})

			db, _ := setupPrivescDB(t)
			u := models.SysUser{UserID: 42, Username: "avatar-user", Password: "test-password", Status: "2",
				CreateBy: 1}
			if !tc.missing {
				if err := db.Create(&u).Error; err != nil {
					t.Fatal(err)
				}
			}
			other := models.SysUser{UserID: 43, Username: "other-avatar", Status: "2"}
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
			profileEngine(db, u.UserID).ServeHTTP(rec, req)
			want := 200
			if tc.missing {
				want = 500
			}
			assertProfileCode(t, rec, want)
			if !tc.missing {
				var after models.SysUser
				if err := db.First(&after, u.UserID).Error; err != nil {
					t.Fatal(err)
				}
				if after.Avatar == "" || after.CreateBy != 1 || after.UpdateBy != 42 {
					t.Fatalf("self avatar not saved correctly: %+v", after)
				}
			}
			var otherAfter models.SysUser
			if err := db.First(&otherAfter, other.UserID).Error; err != nil {
				t.Fatal(err)
			}
			if otherAfter.Avatar != "" {
				t.Fatal("avatar request modified another user")
			}
		})
	}
}

func TestSelfPasswordIgnoresCreatorScope(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	db, _ := setupPrivescDB(t)
	u := models.SysUser{UserID: 42, Username: "self-user", Password: "old-password", Status: "2",
		CreateBy: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	s := service.User{
		DB:     db,
		Logger: logger.NewHelper(logger.Default())}
	err := s.ChangePassword(t.Context(), u.UserID, "old-password", "new-password-long-enough", &actions.DataPermission{UserID: u.UserID, DataScope: actions.DataScopeSelf})
	if err != nil {
		t.Fatalf("user created by admin cannot change own password: %v", err)
	}
}

func TestSelfProfileUpdateWithDataPermission(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	db, _ := setupPrivescDB(t)
	u := models.SysUser{UserID: 42, Username: "profile-user", Password: "old-password", RoleID: 2, DeptID: 1, Status: "2",
		CreateBy: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/profile", strings.NewReader(`{"userId":42,"username":"profile-user","nickName":"Updated","phone":"13800000000","email":"user@example.com","roleId":1,"deptId":999,"status":"1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	profileEngine(db, u.UserID).ServeHTTP(rec, req)
	assertProfileCode(t, rec, 200)
	var after models.SysUser
	if err := db.First(&after, u.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if after.NickName != "Updated" || after.RoleID != 2 || after.DeptID != 1 || after.Status != "2" || after.CreateBy != 1 {
		t.Fatalf("self update changed privileged fields or failed: %+v", after)
	}
}

func TestProfileServiceFailuresHaveStructuredResponses(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = false
	})

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

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	for _, tc := range []struct {
		name, oldPassword, newPassword string
		want                           int
	}{
		{"valid change", "old-password", "new-password-long-enough", 200},
		{"wrong old password", "wrong-password", "new-password-long-enough", 403},
		{"too long for bcrypt", "old-password", strings.Repeat("x", 73), 400},
		{"too short", "old-password", "short", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := setupPrivescDB(t)
			u := models.SysUser{UserID: 42, Username: "password-user", Password: "old-password", Status: "2",
				CreateBy: 1}
			other := models.SysUser{UserID: 43, Username: "other-user", Password: "other-password", Status: "2"}
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
			profileEngine(db, u.UserID).ServeHTTP(rec, req)
			assertProfileCode(t, rec, tc.want)
			var after, otherAfter models.SysUser
			if err := db.First(&after, u.UserID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&otherAfter, other.UserID).Error; err != nil {
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

// A password the user chooses is at least 12 bytes, as at registration.
func TestChangePasswordRefusesShortPasswords(t *testing.T) {
	db, _ := setupPrivescDB(t)
	u := models.SysUser{UserID: 43, Username: "short-user", Password: "old-password", Status: "2"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	s := service.User{DB: db, Logger: logger.NewHelper(logger.Default())}
	if err := s.ChangePassword(t.Context(), u.UserID, "old-password", "short", nil); !errors.Is(err, account.PasswordErr) {
		t.Fatalf("short password accepted: %v", err)
	}
}
