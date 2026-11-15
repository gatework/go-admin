package models

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SysUser defines the sysuser contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysUser struct {
	UserID   int    `gorm:"primaryKey;autoIncrement;comment:编码"  json:"userId"`
	Username string `json:"username" gorm:"type:varchar(64);comment:用户名"`
	Password string `json:"-" gorm:"type:varchar(128);comment:密码"`
	NickName string `json:"nickName" gorm:"type:varchar(128);comment:昵称"`
	Phone    string `json:"phone" gorm:"type:varchar(11);comment:手机号"`
	RoleID   int    `json:"roleId" gorm:"type:bigint;comment:角色ID"`
	Salt     string `json:"-" gorm:"type:varchar(255);comment:加盐"`
	Avatar   string `json:"avatar" gorm:"type:varchar(255);comment:头像"`
	Sex      string `json:"sex" gorm:"type:varchar(255);comment:性别"`
	Email    string `json:"email" gorm:"type:varchar(128);comment:邮箱"`
	DeptID   int    `json:"deptId" gorm:"type:bigint;comment:部门"`
	PostID   int    `json:"postId" gorm:"type:bigint;comment:岗位"`
	Remark   string `json:"remark" gorm:"type:varchar(255);comment:备注"`
	Status   string `json:"status" gorm:"type:varchar(4);comment:状态"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (*SysUser) TableName() string {
	return "sys_user"
}

// Encrypt 加密
func (e *SysUser) Encrypt() (err error) {
	if e.Password == "" {
		return
	}

	var hash []byte
	if hash, err = bcrypt.GenerateFromPassword([]byte(e.Password), bcrypt.DefaultCost); err != nil {
		return
	}
	e.Password = string(hash)
	return
}

// BeforeCreate handles the beforecreate operation.
func (e *SysUser) BeforeCreate(_ *gorm.DB) error {
	return e.Encrypt()
}
