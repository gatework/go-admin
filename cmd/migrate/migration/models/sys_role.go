package models

// SysRole defines the sysrole contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysRole struct {
	RoleID    int       `json:"roleId" gorm:"primaryKey;autoIncrement"`
	RoleName  string    `json:"roleName" gorm:"size:128;"`
	Status    string    `json:"status" gorm:"size:4;"`
	RoleKey   string    `json:"roleKey" gorm:"size:128;"`
	RoleSort  int       `json:"roleSort" gorm:""`
	Flag      string    `json:"flag" gorm:"size:128;"`
	Remark    string    `json:"remark" gorm:"size:255;"`
	Admin     bool      `json:"admin" gorm:"size:4;"`
	DataScope string    `json:"dataScope" gorm:"size:128;"`
	SysMenu   []SysMenu `json:"sysMenu" gorm:"many2many:sys_role_menu;foreignKey:RoleID;joinForeignKey:role_id;references:MenuID;joinReferences:menu_id;"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (SysRole) TableName() string {
	return "sys_role"
}
