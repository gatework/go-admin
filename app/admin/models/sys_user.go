package models

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/gatework/motor/common/models"
)

// SysUser describes persisted administration data.
type SysUser struct {
	models.ControlBy
	models.ModelTime
	// AuthVersion is advanced only by the session revocation path, never DTO updates.
	AuthVersion int64    `json:"-" gorm:"not null;default:0;<-:create"`
	UserID      int      `gorm:"primaryKey;autoIncrement;comment:编码"  json:"userId"`
	Username    string   `json:"username" gorm:"size:64;comment:用户名"`
	Password    string   `json:"-" gorm:"size:128;comment:密码"`
	NickName    string   `json:"nickName" gorm:"size:128;comment:昵称"`
	Phone       string   `json:"phone" gorm:"size:11;comment:手机号"`
	RoleID      int      `json:"roleId" gorm:"size:20;comment:角色ID"`
	Salt        string   `json:"-" gorm:"size:255;comment:加盐"`
	Avatar      string   `json:"avatar" gorm:"size:255;comment:头像"`
	Sex         string   `json:"sex" gorm:"size:255;comment:性别"`
	Email       string   `json:"email" gorm:"size:128;comment:邮箱"`
	DeptID      int      `json:"deptId" gorm:"size:20;comment:部门"`
	PostID      int      `json:"postId" gorm:"size:20;comment:岗位"`
	Remark      string   `json:"remark" gorm:"size:255;comment:备注"`
	Status      string   `json:"status" gorm:"size:4;comment:状态"`
	DeptIDs     []int    `json:"deptIds" gorm:"-"`
	PostIDs     []int    `json:"postIds" gorm:"-"`
	RoleIDs     []int    `json:"roleIds" gorm:"-"`
	Dept        *SysDept `json:"dept"`
}

// TableName returns the explicit persistence table name.
func (*SysUser) TableName() string {
	return "sys_user"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysUser) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysUser) GetID() any {
	return e.UserID
}

// Encrypt hashes Password, unless it already holds a hash.
//
// The hooks below run on whatever is in the struct, and a user read from the
// database carries the stored hash in that field. Hashing it again produces a
// hash of a hash, and the password that user knows no longer matches anything:
// they cannot log in, and nothing reports an error. The only thing preventing
// that today is an Omit("password") on the one update that loads a user first,
// which makes every other write to this model one line away from destroying
// credentials.
//
// bcrypt.Cost parses a hash and fails on anything else, so it distinguishes
// the two cases without the call site having to say which it is. The cost is
// that a password which is itself a well-formed bcrypt hash would be stored
// unchanged - a 60-character string beginning "$2a$", not something a person
// types, and it grants whoever set it no access they did not already have.
func (e *SysUser) Encrypt() error {
	if e.Password == "" {
		return nil
	}
	if _, err := bcrypt.Cost([]byte(e.Password)); err == nil {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(e.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	e.Password = string(hash)
	return nil
}

// BeforeCreate performs the before create administration operation.
func (e *SysUser) BeforeCreate(_ *gorm.DB) error {
	return e.Encrypt()
}

// BeforeUpdate performs the before update administration operation.
func (e *SysUser) BeforeUpdate(_ *gorm.DB) error {
	return e.Encrypt()
}

// AfterFind performs the after find administration operation.
func (e *SysUser) AfterFind(_ *gorm.DB) error {
	e.DeptIDs = []int{e.DeptID}
	e.PostIDs = []int{e.PostID}
	e.RoleIDs = []int{e.RoleID}
	return nil
}
