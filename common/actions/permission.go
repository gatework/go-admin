package actions

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/config"

	contractactions "github.com/gatework/motor-core/sdk/contract/actions"
)

// DataPermission is a thin alias of motor-core's sdk/contract/actions
// (PRD 006 F3/F5).
type DataPermission = contractactions.DataPermission

// The five values sys_role.data_scope can hold, referenced directly from
// motor-core's sdk/contract/actions rather than restated as literals -
// see that package's DataScope* doc comment and PRD 006's hard constraint 4.
const (
	DataScopeAll      = contractactions.DataScopeAll
	DataScopeCustom   = contractactions.DataScopeCustom
	DataScopeDept     = contractactions.DataScopeDept
	DataScopeDeptTree = contractactions.DataScopeDeptTree
	DataScopeSelf     = contractactions.DataScopeSelf
)

// PermissionAction installs the caller's data permission on the request.
// Permission, GetPermissionFromContext and IsValidDataScope forward to
// motor-core's sdk/contract/actions; the generic actions in generic.go use
// them under these names.
func PermissionAction() gin.HandlerFunc {
	return contractactions.PermissionAction(dataPermissionEnabled)
}

// Permission handles the permission operation.
func Permission(tableName string, p *DataPermission) func(db *gorm.DB) *gorm.DB {
	return contractactions.Permission(tableName, p, dataPermissionEnabled)
}

// GetPermissionFromContext returns the selected data.
func GetPermissionFromContext(c *gin.Context) *DataPermission {
	return contractactions.GetPermissionFromContext(c)
}

// IsValidDataScope reports whether s is one of the five values Permission
// recognizes. See motor-core's sdk/contract/actions.IsValidDataScope.
func IsValidDataScope(s string) bool {
	return contractactions.IsValidDataScope(s)
}

func dataPermissionEnabled() bool { return config.Current().Settings.Application.EnableDP }

// Unscoped marks a route whose table is not subject to data permission, such
// as system-wide reference data (posts, dictionaries, configuration). The
// generic actions apply a data permission to every query and treat a missing
// one as permission to see nothing, so a route over such a table states its
// exemption here instead of being silently empty while data permission is on.
// Route permission (Casbin) still applies.
func Unscoped(c *gin.Context) {
	c.Set(PermissionKey, &DataPermission{DataScope: DataScopeAll})
	c.Next()
}
