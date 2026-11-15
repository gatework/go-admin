package models

import "github.com/gatework/motor/common/models"

// SysRole describes persisted administration data.
type SysRole struct {
	models.ControlBy
	models.ModelTime
	RoleID    int        `json:"roleId" gorm:"primaryKey;autoIncrement"` // 角色编码
	RoleName  string     `json:"roleName" gorm:"size:128;"`              // 角色名称
	Status    string     `json:"status" gorm:"size:4;"`                  // 状态 1禁用 2正常
	RoleKey   string     `json:"roleKey" gorm:"size:128;"`               //角色代码
	RoleSort  int        `json:"roleSort" gorm:""`                       //角色排序
	Flag      string     `json:"flag" gorm:"size:128;"`                  //
	Remark    string     `json:"remark" gorm:"size:255;"`                //备注
	Admin     bool       `json:"admin" gorm:"size:4;"`
	DataScope string     `json:"dataScope" gorm:"size:128;"`
	Params    string     `json:"params" gorm:"-"`
	MenuIDs   []int      `json:"menuIds" gorm:"-"`
	DeptIDs   []int      `json:"deptIds" gorm:"-"`
	SysDept   []SysDept  `json:"sysDept" gorm:"many2many:sys_role_dept;foreignKey:RoleID;joinForeignKey:role_id;references:DeptID;joinReferences:dept_id;"`
	SysMenu   *[]SysMenu `json:"sysMenu" gorm:"many2many:sys_role_menu;foreignKey:RoleID;joinForeignKey:role_id;references:MenuID;joinReferences:menu_id;"`
}

// TableName returns the explicit persistence table name.
func (*SysRole) TableName() string {
	return "sys_role"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysRole) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysRole) GetID() any {
	return e.RoleID
}
