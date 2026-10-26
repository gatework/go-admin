package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/pquerna/otp/totp"
	"go-admin/common/account"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestJWTIsIssuedOnlyAfterMFAAndRevokedOnChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCOUNT_SECURITY_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	oldMode := config.ApplicationConfig.Mode
	oldSecret := config.JwtConfig.Secret
	config.ApplicationConfig.Mode = "dev"
	config.JwtConfig.Secret = "test-jwt-secret"
	t.Cleanup(func() { config.ApplicationConfig.Mode = oldMode; config.JwtConfig.Secret = oldSecret })
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&SysUser{}, &SysRole{}, &account.State{}, &account.Challenge{}, &account.Recovery{}, &account.Limit{}); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-password-123"), bcrypt.MinCost)
	db.Create(&SysRole{RoleId: 1, RoleKey: "admin", Status: "2"})
	db.Create(&SysUser{UserId: 1, Username: "admin", Password: string(hash), RoleId: 1, Status: "2"})
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte(config.JwtConfig.Secret), Timeout: time.Hour, PayloadFunc: PayloadFunc, IdentityHandler: IdentityHandler, Authenticator: Authenticator, Authorizator: Authorizator, Unauthorized: Unauthorized, TokenLookup: "header: Authorization", TokenHeadName: "Bearer"})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("db", db); c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger)) })
	r.POST("/login", auth.LoginHandler)
	r.GET("/private", auth.MiddlewareFunc(), func(c *gin.Context) { c.JSON(200, gin.H{"code": 200}) })
	call := func(method, path, token string, data any) map[string]any {
		body, _ := json.Marshal(data)
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e, w.Body.String())
		}
		return out
	}
	// Missing fields must not consume an arbitrary account's shared quota.
	call("POST", "/login", "", map[string]string{"username": "admin"})
	var limits int64
	db.Model(&account.Limit{}).Count(&limits)
	if limits != 1 {
		t.Fatal("incomplete login consumed account quota", limits)
	}
	credentials := map[string]string{"username": "admin", "password": "admin-password-123", "code": "0", "uuid": "0"}
	first := call("POST", "/login", "", credentials)
	token, ok := first["token"].(string)
	if !ok {
		t.Fatal(first)
	}
	if call("GET", "/private", token, nil)["code"] != float64(200) {
		t.Fatal("fresh token rejected")
	}
	s := account.New(db)
	setup, e := s.Setup(1, "admin-password-123")
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	codes, e := s.Enable(1, code)
	if e != nil {
		t.Fatal(e)
	}
	if call("GET", "/private", token, nil)["code"] == float64(200) {
		t.Fatal("pre-MFA token still valid")
	}
	challenge := call("POST", "/login", "", credentials)
	if challenge["mfaRequired"] != true || challenge["token"] != nil {
		t.Fatal("MFA bypass", challenge)
	}
	second := call("POST", "/login", "", map[string]any{"challengeToken": challenge["challengeToken"], "recoveryCode": codes[0]})
	newToken, ok := second["token"].(string)
	if !ok {
		t.Fatal(second)
	}
	if call("GET", "/private", newToken, nil)["code"] != float64(200) {
		t.Fatal("verified token rejected")
	}
	replay := call("POST", "/login", "", map[string]any{"challengeToken": challenge["challengeToken"], "recoveryCode": codes[1]})
	if replay["token"] != nil {
		t.Fatal("challenge replay")
	}
	newHash, _ := bcrypt.GenerateFromPassword([]byte("new-password-123"), bcrypt.MinCost)
	db.Model(&SysUser{}).Where("user_id = 1").Update("password", string(newHash))
	if call("GET", "/private", newToken, nil)["code"] == float64(200) {
		t.Fatal("token survived password change")
	}
}
