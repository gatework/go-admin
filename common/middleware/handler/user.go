package handler

import (
	"gorm.io/gorm"

	"github.com/gatework/motor/common/models"
)

// SysUser defines the sysuser contract.
type SysUser struct {
	//Dept     *SysDept `json:"dept"`
	models.ControlBy
	models.ModelTime
	// AuthVersion is advanced only by the session revocation path, never DTO updates.
	AuthVersion int64  `json:"-" gorm:"not null;default:0;<-:create"`
	UserID      int    `gorm:"primaryKey;autoIncrement;comment:编码"  json:"userId"`
	Username    string `json:"username" gorm:"size:64;comment:用户名"`
	Password    string `json:"-" gorm:"size:128;comment:密码"`
	NickName    string `json:"nickName" gorm:"size:128;comment:昵称"`
	Phone       string `json:"phone" gorm:"size:11;comment:手机号"`
	RoleID      int    `json:"roleId" gorm:"size:20;comment:角色ID"`
	Salt        string `json:"-" gorm:"size:255;comment:加盐"`
	Avatar      string `json:"avatar" gorm:"size:255;comment:头像"`
	Sex         string `json:"sex" gorm:"size:255;comment:性别"`
	Email       string `json:"email" gorm:"size:128;comment:邮箱"`
	DeptID      int    `json:"deptId" gorm:"size:20;comment:部门"`
	PostID      int    `json:"postId" gorm:"size:20;comment:岗位"`
	Remark      string `json:"remark" gorm:"size:255;comment:备注"`
	Status      string `json:"status" gorm:"size:4;comment:状态"`
	DeptIDs     []int  `json:"deptIds" gorm:"-"`
	PostIDs     []int  `json:"postIds" gorm:"-"`
	RoleIDs     []int  `json:"roleIds" gorm:"-"`
}

// TableName returns the explicit persistence table name.
func (*SysUser) TableName() string {
	return "sys_user"
}

// AfterFind handles the afterfind operation.
func (e *SysUser) AfterFind(_ *gorm.DB) error {
	e.DeptIDs = []int{e.DeptID}
	e.PostIDs = []int{e.PostID}
	e.RoleIDs = []int{e.RoleID}
	return nil
}
