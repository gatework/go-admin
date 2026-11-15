package models

import "github.com/gatework/motor/common/models"

// SysDept describes persisted administration data.
type SysDept struct {
	models.ControlBy
	models.ModelTime
	DeptID    int       `json:"deptId" gorm:"primaryKey;autoIncrement;"` //部门编码
	ParentID  int       `json:"parentId" gorm:""`                        //上级部门
	DeptPath  string    `json:"deptPath" gorm:"size:255;"`               //
	DeptName  string    `json:"deptName"  gorm:"size:128;"`              //部门名称
	Sort      int       `json:"sort" gorm:"size:4;"`                     //排序
	Leader    string    `json:"leader" gorm:"size:128;"`                 //负责人
	Phone     string    `json:"phone" gorm:"size:11;"`                   //手机
	Email     string    `json:"email" gorm:"size:64;"`                   //邮箱
	Status    int       `json:"status" gorm:"size:4;"`                   //状态
	DataScope string    `json:"dataScope" gorm:"-"`
	Params    string    `json:"params" gorm:"-"`
	Children  []SysDept `json:"children" gorm:"-"`
}

// TableName returns the explicit persistence table name.
func (*SysDept) TableName() string {
	return "sys_dept"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysDept) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysDept) GetID() any {
	return e.DeptID
}
