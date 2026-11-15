package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor/common/account"

	"github.com/gin-gonic/gin"
	"github.com/mssola/user_agent"

	"github.com/gatework/motor-core/captcha"
	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor/common/global"
)

// AuthCallbacks keeps the credential MAC on the same immutable startup key as
// the JWT signer. Rotating the configured key takes effect after restart.
func AuthCallbacks(secret string) (func(any) jwt.MapClaims, func(any, *gin.Context) bool) {
	key := []byte(secret)
	return func(data any) jwt.MapClaims { return payloadFunc(data, key) },
		func(data any, c *gin.Context) bool { return authorize(data, c, key) }
}

func payloadFunc(data any, key []byte) jwt.MapClaims {
	if v, ok := data.(map[string]any); ok {
		u, _ := v["user"].(SysUser)
		r, _ := v["role"].(SysRole)
		return jwt.MapClaims{
			jwt.IdentityKey:  u.UserID,
			jwt.RoleIDKey:    r.RoleID,
			jwt.RoleKey:      r.RoleKey,
			jwt.NiceKey:      u.Username,
			jwt.DataScopeKey: r.DataScope,
			jwt.RoleNameKey:  r.RoleName,
			// deptid completes what the data-permission scope is decided by,
			// so it can be read from the token instead of joined for on every
			// request. core's user.GetDeptId has always read this claim.
			"deptid":     u.DeptID,
			"tenant":     v["tenant"],
			"credential": credentialStamp(u, r, key),
			"auth_stamp": account.Stamp(string(key), u.Password, accountVersion(v)),
		}
	}
	return jwt.MapClaims{}
}

// accountVersion defaults to zero for callers issuing a token before MFA is enabled.
func accountVersion(values map[string]any) int64 {
	version, _ := values["account_version"].(int64)
	return version
}

// IdentityHandler handles the identityhandler operation.
func IdentityHandler(c *gin.Context) any {
	claims := jwt.ExtractClaims(c)
	return map[string]any{
		"IdentityKey": claims["identity"],
		"UserName":    claims["nice"],
		"RoleKey":     claims["rolekey"],
		"UserId":      claims["identity"],
		"RoleIds":     claims["roleid"],
		"DataScope":   claims["datascope"],
	}
}

// Authenticator 获取token
// @Summary 登陆
// @Description 获取token
// @Description LoginHandler can be used by clients to get a jwt token.
// @Description Payload needs to be json in the form of {"username": "USERNAME", "password": "PASSWORD"}.
// @Description Reply will be of the form {"token": "TOKEN"}.
// @Description dev mode：It should be noted that all fields cannot be empty, and a value of 0 can be passed in addition to the account password
// @Description 注意：开发模式：需要注意全部字段不能为空，账号密码外可以传入0值
// @Tags 登陆
// @Accept  application/json
// @Product application/json
// @Param account body Login  true "account"
// @Success 200 {string} string "{"code": 200, "expire": "2019-08-07T12:45:48+08:00", "token": ".eyJleHAiOjE1NjUxNTMxNDgsImlkIjoiYWRtaW4iLCJvcmlnX2lhdCI6MTU2NTE0OTU0OH0.-zvzHvbg0A" }"
// @Router /api/v1/login [post]
func Authenticator(c *gin.Context) (any, error) {
	log := api.GetRequestLogger(c)
	db, err := pkg.GetORM(c)
	if err != nil {
		log.Errorf("get db error, %s", err.Error())
		response.Error(c, 500, err, "数据库连接获取失败")
		return nil, jwt.ErrFailedAuthentication
	}

	var loginVals Login
	var status = "2"
	var msg = "登录成功"
	var username = ""
	defer func() {
		LoginLogToDB(c, status, msg, username)
	}()

	if err = c.ShouldBind(&loginVals); err != nil {
		username = loginVals.Username
		msg = "数据解析失败"
		status = "1"

		return nil, jwt.ErrMissingLoginValues
	}
	security := account.New(db.WithContext(c.Request.Context()))
	if err = security.TakeLimit("login:"+c.ClientIP(), 20, 5*time.Minute); err != nil {
		status = "1"
		msg = "登录频率超限"
		return nil, jwt.ErrFailedAuthentication
	}
	var sysUser SysUser
	var role SysRole
	var authVersion int64
	if loginVals.ChallengeToken != "" {
		verified, e := security.CompleteLogin(loginVals.ChallengeToken, loginVals.TOTPCode, loginVals.RecoveryCode)
		if e != nil {
			status = "1"
			msg = "MFA 验证失败"
			return nil, jwt.ErrFailedAuthentication
		}
		if err = db.Where("user_id = ? AND status = '2' AND deleted_at = 0", verified.UserID).First(&sysUser).Error; err != nil || sysUser.Password != verified.Password {
			return nil, jwt.ErrFailedAuthentication
		}
		authVersion = verified.SessionVersion
		if err = db.Where("role_id = ? AND status = '2' AND deleted_at = 0", sysUser.RoleID).First(&role).Error; err != nil {
			return nil, jwt.ErrFailedAuthentication
		}
	} else {
		sysUser, role, authVersion, err = authenticatePassword(c, db, security, loginVals, &status, &msg)
		if err != nil {
			return nil, err
		}
	}
	username = sysUser.Username
	return map[string]any{"user": sysUser, "role": role, "account_version": authVersion, "tenant": Tenant(c.Request.Host)}, nil
}

// LoginLogToDB Write log to database
func LoginLogToDB(c *gin.Context, status string, msg string, username string) {
	if !config.Current().Settings.Logger.EnabledDB {
		return
	}
	log := api.GetRequestLogger(c)
	l := make(map[string]any)

	ua := user_agent.New(c.Request.UserAgent())
	l["ipaddr"] = c.ClientIP()
	l["loginLocation"] = ""
	l["loginTime"] = pkg.GetCurrentTime()
	l["status"] = status
	l["remark"] = c.Request.UserAgent()
	browserName, browserVersion := ua.Browser()
	l["browser"] = browserName + " " + browserVersion
	l["os"] = ua.OS()
	l["platform"] = ua.Platform()
	l["username"] = username
	l["msg"] = msg

	payload, err := json.Marshal(l)
	if err != nil {
		log.Errorf("encode log message: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	message := storage.Message{Topic: global.LoginLog, Values: map[string]any{"payload": string(payload)}}
	if err := sdk.Runtime.QueueForTenant(c.Request.Host).Publish(ctx, message); err != nil {
		log.Errorf("publish log message: %v", err)
	}
}

// LogOut closes the authenticated user session.
// @Summary 退出登录
// @Description 获取token
// LoginHandler can be used by clients to get a jwt token.
// Reply will be of the form {"token": "TOKEN"}.
// @Accept  application/json
// @Product application/json
// @Success 200 {string} string "{"code": 200, "msg": "成功退出系统" }"
// @Router /logout [post]
// @Security Bearer
func LogOut(c *gin.Context) {
	db, err := pkg.GetORM(c)
	if err != nil {
		response.Error(c, 500, err, "退出失败")
		return
	}
	// Table updates deliberately bypass AuthVersion's create-only model field.
	result := db.WithContext(c.Request.Context()).Table("sys_user").
		Where("user_id = ? AND deleted_at = ? AND status = ?", user.ID(c), 0, "2").
		UpdateColumn("auth_version", gorm.Expr("auth_version + ?", 1))
	if result.Error != nil {
		response.Error(c, 500, result.Error, "退出失败")
		return
	}
	if result.RowsAffected != 1 {
		response.Error(c, 403, nil, "退出失败")
		return
	}
	closeUserWebsockets(Tenant(c.Request.Host), user.ID(c))
	LoginLogToDB(c, "2", "退出成功", user.Name(c))
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "退出成功",
	})

}

// Tenant resolves the same wildcard precedence as the runtime database registry.
// Host aliases sharing the wildcard database belong to one tenant.
func Tenant(host string) string {
	if sdk.Runtime.Databases()["*"] != nil {
		return "*"
	}
	return host
}

// credentialStamp revokes old tokens when credentials or authorization identity
// changes without placing the password hash itself in the readable JWT payload.
func credentialStamp(u SysUser, r SysRole, key []byte) string {
	state, _ := json.Marshal([]any{u.UserID, u.CreatedAt.UnixNano(), u.AuthVersion, u.Username, u.Password, u.RoleID, u.DeptID, r.RoleID, r.CreatedAt.UnixNano(), r.RoleKey, r.DataScope, r.Status})
	mac := hmac.New(sha256.New, key)
	mac.Write(state)
	return hex.EncodeToString(mac.Sum(nil))
}

func authorize(data any, c *gin.Context, key []byte) bool {
	if _, ok := data.(map[string]any); !ok {
		return false
	}
	claims := jwt.ExtractClaims(c)
	tenant, ok := claims["tenant"].(string)
	if !ok || tenant == "" || tenant != Tenant(c.Request.Host) {
		return false
	}
	stamp, ok := claims["credential"].(string)
	if !ok || stamp == "" {
		return false
	}
	db, err := pkg.GetORM(c)
	if err != nil {
		return false
	}
	id := user.ID(c)
	if id <= 0 {
		return false
	}
	var u SysUser
	if db.Where("user_id = ? AND status = ?", id, "2").First(&u).Error != nil {
		return false
	}
	var r SysRole
	if db.Where("role_id = ? AND status = ?", u.RoleID, "2").First(&r).Error != nil {
		return false
	}
	state, err := account.New(db.WithContext(c.Request.Context())).State(id)
	if err != nil {
		return false
	}
	accountStamp, ok := claims["auth_stamp"].(string)
	if !ok || !hmac.Equal([]byte(accountStamp), []byte(account.Stamp(string(key), u.Password, state.Version))) {
		return false
	}
	return hmac.Equal([]byte(stamp), []byte(credentialStamp(u, r, key)))
}

// Unauthorized handles the unauthorized operation.
func Unauthorized(c *gin.Context, code int, message string) {
	if token, ok := c.Get("account_mfa_challenge"); ok {
		c.JSON(http.StatusOK, gin.H{"code": 200, "mfaRequired": true, "challengeToken": token, "expiresIn": 300})
		return
	}
	c.Set("status", code)
	c.JSON(code, gin.H{
		"code": code,
		"msg":  message,
	})
}

func authenticatePassword(c *gin.Context, db *gorm.DB, security account.Service, loginVals Login, status, msg *string) (SysUser, SysRole, int64, error) {
	var sysUser SysUser
	var role SysRole
	var authVersion int64
	var err error
	if loginVals.Username == "" || loginVals.Password == "" || loginVals.Code == "" || loginVals.UUID == "" {
		*status = "1"
		return sysUser, role, authVersion, jwt.ErrMissingLoginValues
	}
	if config.Current().Settings.Application.Mode != "dev" && !captcha.Verify(loginVals.UUID, loginVals.Code, true) {
		*status = "1"
		*msg = "验证码错误"
		return sysUser, role, authVersion, jwt.ErrInvalidVerificationode
	}
	// Account quotas are consumed only after required fields and CAPTCHA pass.
	if err = security.TakeLimit("login-user:"+loginVals.Username, 20, 5*time.Minute); err != nil {
		*status = "1"
		*msg = "登录频率超限"
		return sysUser, role, authVersion, jwt.ErrFailedAuthentication
	}
	var e error
	sysUser, role, e = loginVals.GetUser(db)
	if e != nil || role.Status != "2" {
		*status = "1"
		*msg = "登录失败"
		return sysUser, role, authVersion, jwt.ErrFailedAuthentication
	}
	state, e := security.State(sysUser.UserID)
	if e != nil {
		return sysUser, role, authVersion, jwt.ErrFailedAuthentication
	}
	authVersion = state.Version
	if state.Enabled {
		u := account.User{UserID: sysUser.UserID, Username: sysUser.Username, Password: sysUser.Password}
		token, e := security.LoginChallenge(u, state)
		if e != nil {
			return sysUser, role, authVersion, jwt.ErrFailedAuthentication
		}
		c.Set("account_mfa_challenge", token)
		*msg = "等待 MFA 验证"
		return sysUser, role, authVersion, jwt.ErrFailedAuthentication
	}
	return sysUser, role, authVersion, nil
}
