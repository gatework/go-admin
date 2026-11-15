package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mycasbin "github.com/gatework/motor-core/casbin"
	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/admin/models"
)

// PUT /api/v1/sys-user is in settings.go's CasbinExclude so the
// personal-center screen (go-admin-ui's userInfo.vue) can edit the caller's
// own record without holding a policy grant on this route. AuthCheckRole
// skips Enforce entirely for an excluded route, so this file's job is to pin
// what the handler itself now has to hold shut: the target userId comes from
// the request body, and nothing upstream of the handler ever checked it
// against the caller.

// setupPrivescDB wires an in-memory database and a Casbin enforcer with an
// empty policy - the state of a fresh install for any role but admin - under
// a tenant unique to the calling test, so mycasbin's process-wide enforcer
// cache can't hand one test's database to another.
func setupPrivescDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()

	// Fatalf, not Skipf: this database is in-memory sqlite with no external
	// dependency, so failing to open or migrate it means the environment is
	// actually broken. Skipping here would let these two anti-privesc
	// regression tests silently stop running while CI stays green - a
	// standing assertion that never fires is worse than no assertion.
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&models.SysUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	tenant := "sys-user-privesc-" + t.Name()

	previousInterval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0 // opt out of the background reload goroutine; the test never writes a policy
	t.Cleanup(func() { mycasbin.ReloadInterval = previousInterval })

	e := mycasbin.Setup(db, tenant)
	previousEnforcer := sdk.Runtime.Casbin(tenant)
	sdk.Runtime.SetCasbin(tenant, e)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(tenant, previousEnforcer) })

	return db, tenant
}

// callUpdate drives SysUser.Update the way the router does for an
// authenticated, non-admin caller: JWT claims already decoded into the
// context (that is jwtauth's job, not this handler's) and a database - but
// without AuthCheckRole, since that middleware never runs Enforce for this
// route at all.
func callUpdate(t *testing.T, db *gorm.DB, tenant string, callerID int, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/sys-user", bytes.NewReader(raw))
	c.Request.Host = tenant
	c.Request.Header.Set("Content-Type", "application/json")

	c.Set("db", db)
	c.Set(pkg.LoggerKey, logger.NewHelper(logger.Default()))
	c.Set(jwt.JwtPayloadKey, jwt.MapClaims{
		"identity": float64(callerID),
		"rolekey":  "ordinary-role", // holds no Casbin policy anywhere in this test
	})

	User{}.Update(c)
	return w
}

// TestUpdate_CannotEscalatePrivilegeThroughAnotherUsersRecord is the
// regression for H6. Before the fix, an ordinary authenticated user could PUT
// a body naming another user's id and change that user's roleId - the route
// being Casbin-excluded meant no permission check ever ran, and the data
// permission scope that would otherwise gate this is off by default.
func TestUpdate_CannotEscalatePrivilegeThroughAnotherUsersRecord(t *testing.T) {
	db, tenant := setupPrivescDB(t)

	victim := models.SysUser{Username: "bob", NickName: "Bob", RoleID: 2, DeptID: 1, Status: "1"}
	if err := db.Create(&victim).Error; err != nil {
		t.Fatal(err)
	}
	attacker := models.SysUser{Username: "alice", NickName: "Alice", RoleID: 2, DeptID: 1, Status: "1"}
	if err := db.Create(&attacker).Error; err != nil {
		t.Fatal(err)
	}

	const elevatedRoleID = 1 // a role the attacker does not hold and has no policy for

	callUpdate(t, db, tenant, attacker.UserID, map[string]any{
		"userId":   victim.UserID,
		"username": victim.Username,
		"nickName": "pwned",
		"phone":    "13800000000",
		"email":    "bob@example.com",
		"roleId":   elevatedRoleID,
		"deptId":   victim.DeptID,
		"status":   victim.Status,
	})

	var after models.SysUser
	if err := db.First(&after, victim.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if after.RoleID == elevatedRoleID {
		t.Fatalf("an attacker with no Casbin permission on this route escalated the victim's roleId to %d", after.RoleID)
	}
	if after.NickName == "pwned" {
		t.Fatalf("an attacker with no Casbin permission on this route modified another user's record: %+v", after)
	}
}

// TestUpdate_SelfEditCannotChangePrivilegedFields covers the case the
// CasbinExclude entry exists for: the personal-center screen has to keep
// working for the caller's own record. The fields that screen exposes
// (nickName/phone/email/sex) must still save, while roleId/deptId/status stay
// whatever the database already had even if the request carries something
// else - a compromised or hand-crafted client is the only way that request
// would ever differ from what the honest form sends.
func TestUpdate_SelfEditCannotChangePrivilegedFields(t *testing.T) {
	db, tenant := setupPrivescDB(t)

	self := models.SysUser{Username: "carol", NickName: "Carol", RoleID: 2, DeptID: 1, Status: "1"}
	if err := db.Create(&self).Error; err != nil {
		t.Fatal(err)
	}

	const elevatedRoleID = 1

	callUpdate(t, db, tenant, self.UserID, map[string]any{
		"userId":   self.UserID,
		"username": self.Username,
		"nickName": "Carol Updated",
		"phone":    "13900000000",
		"email":    "carol@example.com",
		"roleId":   elevatedRoleID, // tampered; must not take effect
		"deptId":   self.DeptID,
		"status":   self.Status,
	})

	var after models.SysUser
	if err := db.First(&after, self.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if after.RoleID == elevatedRoleID {
		t.Fatalf("a self-edit changed the caller's own roleId to %d", after.RoleID)
	}
	if after.NickName != "Carol Updated" {
		t.Fatalf("the legitimate personal-center edit did not go through: %+v", after)
	}
}
