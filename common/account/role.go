package account

// AdminRoleKey is the role key of the built-in administrator, which bypasses
// route (Casbin) and data-permission checks.
const AdminRoleKey = "admin"

// IsAdmin reports whether roleKey is the built-in administrator. Every
// administrator bypass decides through here, so the rule has one spelling.
func IsAdmin(roleKey string) bool { return roleKey == AdminRoleKey }
