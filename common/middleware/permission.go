package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	mycasbin "github.com/gatework/motor-core/casbin"
	"github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
)

// AuthCheckRole 权限检查中间件
func AuthCheckRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := api.GetRequestLogger(c)
		data, _ := c.Get(jwtauth.JwtPayloadKey)
		v, ok := data.(jwtauth.MapClaims)
		if !ok {
			response.Error(c, 401, fmt.Errorf("permission: missing claims"), "")
			return
		}
		e := sdk.Runtime.Casbin(c.Request.Host)
		var res, casbinExclude bool
		var err error
		//检查权限
		if v["rolekey"] == "admin" {
			c.Next()
			return
		}
		casbinExclude, err = excludedFromCasbin(c.Request.Method, c.Request.URL.Path)
		if err != nil {
			log.Errorf("AuthCheckRole: %s", err)
		}
		if casbinExclude {
			log.Infof("Casbin exclusion, no validation method:%s path:%s", c.Request.Method, c.Request.URL.Path)
			c.Next()
			return
		}
		res, err = e.Enforce(v["rolekey"], c.Request.URL.Path, c.Request.Method)
		if err != nil {
			log.Errorf("AuthCheckRole error:%s method:%s path:%s", err, c.Request.Method, c.Request.URL.Path)
			response.Error(c, 500, err, "")
			return
		}

		if !res {
			log.Warnf("permission denied: role=%v method=%s path=%s", v["rolekey"], c.Request.Method, c.Request.URL.Path)
			c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "对不起，您没有该接口访问权限，请联系管理员"})
			c.Abort()
			return
		}
		log.Infof("permission accepted: role=%v method=%s path=%s", v["rolekey"], c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}

// EnforceRoleFor reports whether the caller's role has explicit Casbin
// permission to act on path with method.
//
// AuthCheckRole never calls Enforce for a route CasbinExclude lists - that
// is the whole point of the list. A handler on such a route can still need
// the real answer for part of what it does: sys_user.go's Update shares its
// excluded route between the personal-center screen editing the caller's own
// record (which is why the route is excluded at all) and an admin editing
// someone else's, and only the second case is meant to require a policy
// grant. That handler asks here instead of assuming the middleware already
// checked.
func EnforceRoleFor(c *gin.Context, path, method string) (bool, error) {
	data, ok := c.Get(jwtauth.JwtPayloadKey)
	if !ok {
		return false, nil
	}
	v, ok := data.(jwtauth.MapClaims)
	if !ok {
		return false, nil
	}
	if v["rolekey"] == "admin" {
		return true, nil
	}
	e := sdk.Runtime.Casbin(c.Request.Host)
	return e.Enforce(v["rolekey"], path, method)
}

// excludedFromCasbin reports whether the route skips the permission check.
//
// It runs for every non-admin request, so the order matters: the method rules
// out most entries with a string compare, where the path test costs a pattern
// match. mycasbin.KeyMatch2 answers what casbin's util.KeyMatch2 answers
// without recompiling the pattern every time, which is what made this loop
// expensive - about 2,500 allocations per request against a 32-entry list.
//
// A pattern that will not compile is a bug in CasbinExclude rather than in the
// request, so the entry is skipped and the scan continues; the error comes
// back for the caller to log.
func excludedFromCasbin(method, path string) (bool, error) {
	var bad error
	for _, i := range CasbinExclude {
		if method != i.Method {
			continue
		}
		ok, err := mycasbin.KeyMatch2(path, i.URL)
		if err != nil {
			bad = fmt.Errorf("CasbinExclude entry %q is not a valid pattern: %w", i.URL, err)
			continue
		}
		if ok {
			return true, bad
		}
	}
	return false, bad
}
