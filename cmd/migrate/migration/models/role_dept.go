package models

// SysRoleDept defines the sysroledept contract.
type SysRoleDept struct {
	RoleID int `gorm:"size:11;primaryKey"`
	DeptID int `gorm:"size:11;primaryKey"`
}

// TableName returns the explicit persistence table name.
func (SysRoleDept) TableName() string {
	return "sys_role_dept"
}
