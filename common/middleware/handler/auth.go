package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go-admin/common"
	"gorm.io/gorm"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/captcha"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/jwtauth/user"
	"github.com/go-admin-team/go-admin-core/v2/response"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/api"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/mssola/user_agent"
	"go-admin/common/global"
)

// AuthCallbacks keeps the credential MAC on the same immutable startup key as
// the JWT signer. Rotating the configured key takes effect after restart.
func AuthCallbacks(secret string) (func(interface{}) jwt.MapClaims, func(interface{}, *gin.Context) bool) {
	key := []byte(secret)
	return func(data interface{}) jwt.MapClaims { return payloadFunc(data, key) },
		func(data interface{}, c *gin.Context) bool { return authorize(data, c, key) }
}

// PayloadFunc remains available for callers constructing their own middleware.
// Host middleware uses AuthCallbacks to pin signing and stamp keys together.
func PayloadFunc(data interface{}) jwt.MapClaims {
	return payloadFunc(data, []byte(config.JwtConfig.Secret))
}

func payloadFunc(data interface{}, key []byte) jwt.MapClaims {
	if v, ok := data.(map[string]interface{}); ok {
		u, _ := v["user"].(SysUser)
		r, _ := v["role"].(SysRole)
		return jwt.MapClaims{
			jwt.IdentityKey:  u.UserId,
			jwt.RoleIdKey:    r.RoleId,
			jwt.RoleKey:      r.RoleKey,
			jwt.NiceKey:      u.Username,
			jwt.DataScopeKey: r.DataScope,
			jwt.RoleNameKey:  r.RoleName,
			// deptid completes what the data-permission scope is decided by,
			// so it can be read from the token instead of joined for on every
			// request. core's user.GetDeptId has always read this claim.
			"deptid":     u.DeptId,
			"tenant":     v["tenant"],
			"credential": credentialStamp(u, r, key),
		}
	}
	return jwt.MapClaims{}
}

func IdentityHandler(c *gin.Context) interface{} {
	claims := jwt.ExtractClaims(c)
	return map[string]interface{}{
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
func Authenticator(c *gin.Context) (interface{}, error) {
	log := api.GetRequestLogger(c)
	db, err := pkg.GetOrm(c)
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
	if config.ApplicationConfig.Mode != "dev" {
		if !captcha.Verify(loginVals.UUID, loginVals.Code, true) {
			username = loginVals.Username
			msg = "验证码错误"
			status = "1"

			return nil, jwt.ErrInvalidVerificationode
		}
	}
	sysUser, role, e := loginVals.GetUser(db)
	if e == nil {
		username = loginVals.Username

		return map[string]interface{}{"user": sysUser, "role": role, "tenant": Tenant(c.Request.Host)}, nil
	} else {
		msg = "登录失败"
		status = "1"
		log.Warnf("%s login failed!", loginVals.Username)
	}
	return nil, jwt.ErrFailedAuthentication
}

// LoginLogToDB Write log to database
func LoginLogToDB(c *gin.Context, status string, msg string, username string) {
	if !config.LoggerConfig.EnabledDB {
		return
	}
	log := api.GetRequestLogger(c)
	l := make(map[string]interface{})

	ua := user_agent.New(c.Request.UserAgent())
	l["ipaddr"] = common.GetClientIP(c)
	l["loginLocation"] = "" // pkg.GetLocation(common.GetClientIP(c),gaConfig.ExtConfig.AMap.Key)
	l["loginTime"] = pkg.GetCurrentTime()
	l["status"] = status
	l["remark"] = c.Request.UserAgent()
	browserName, browserVersion := ua.Browser()
	l["browser"] = browserName + " " + browserVersion
	l["os"] = ua.OS()
	l["platform"] = ua.Platform()
	l["username"] = username
	l["msg"] = msg

	q := sdk.Runtime.GetQueuePrefix(c.Request.Host)
	message, err := sdk.Runtime.GetStreamMessage("", global.LoginLog, l)
	if err != nil {
		log.Errorf("GetStreamMessage error, %s", err.Error())
		//日志报错错误，不中断请求
	} else {
		err = q.Append(message)
		if err != nil {
			log.Errorf("Append message error, %s", err.Error())
		}
	}
}

// LogOut
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
	db, err := pkg.GetOrm(c)
	if err != nil {
		response.Error(c, 500, err, "退出失败")
		return
	}
	// Table updates deliberately bypass AuthVersion's create-only model field.
	result := db.WithContext(c.Request.Context()).Table("sys_user").
		Where("user_id = ? AND deleted_at = ? AND status = ?", user.GetUserId(c), 0, "2").
		UpdateColumn("auth_version", gorm.Expr("auth_version + ?", 1))
	if result.Error != nil {
		response.Error(c, 500, result.Error, "退出失败")
		return
	}
	if result.RowsAffected != 1 {
		response.Error(c, 403, nil, "退出失败")
		return
	}
	closeUserWebsockets(Tenant(c.Request.Host), user.GetUserId(c))
	LoginLogToDB(c, "2", "退出成功", user.GetUserName(c))
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "退出成功",
	})

}

// Tenant resolves the same wildcard precedence as the runtime database registry.
// Host aliases sharing the wildcard database belong to one tenant.
func Tenant(host string) string {
	if sdk.Runtime.GetAllDb()["*"] != nil {
		return "*"
	}
	return host
}

// credentialStamp revokes old tokens when credentials or authorization identity
// changes without placing the password hash itself in the readable JWT payload.
func credentialStamp(u SysUser, r SysRole, key []byte) string {
	state, _ := json.Marshal([]interface{}{u.UserId, u.CreatedAt.UnixNano(), u.AuthVersion, u.Username, u.Password, u.RoleId, u.DeptId, r.RoleId, r.CreatedAt.UnixNano(), r.RoleKey, r.DataScope, r.Status})
	mac := hmac.New(sha256.New, key)
	mac.Write(state)
	return hex.EncodeToString(mac.Sum(nil))
}

// Authorizator verifies tenant binding and current account state on every request.
// A signed token alone does not prove that the account is still enabled.
func Authorizator(data interface{}, c *gin.Context) bool {
	return authorize(data, c, []byte(config.JwtConfig.Secret))
}

func authorize(data interface{}, c *gin.Context, key []byte) bool {
	if _, ok := data.(map[string]interface{}); !ok {
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
	db, err := pkg.GetOrm(c)
	if err != nil {
		return false
	}
	id := user.GetUserId(c)
	if id <= 0 {
		return false
	}
	var u SysUser
	if db.Where("user_id = ? AND status = ?", id, "2").First(&u).Error != nil {
		return false
	}
	var r SysRole
	if db.Where("role_id = ? AND status = ?", u.RoleId, "2").First(&r).Error != nil {
		return false
	}
	return hmac.Equal([]byte(stamp), []byte(credentialStamp(u, r, key)))
}

func Unauthorized(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code": code,
		"msg":  message,
	})
}
