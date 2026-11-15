package dto

import (
	"github.com/gatework/motor/app/admin/models"

	"github.com/gatework/motor/common/dto"
	commonmodels "github.com/gatework/motor/common/models"
)

// UserSearch carries validated administration request parameters.
type UserSearch struct {
	dto.Pagination   `search:"-"`
	DepartmentFilter `search:"type:left;on:dept_id:dept_id;table:sys_user;join:sys_dept"`
	UserOrder
	UserID   int    `form:"userId" search:"type:exact;column:user_id;table:sys_user" comment:"用户ID"`
	Username string `form:"username" search:"type:contains;column:username;table:sys_user" comment:"用户名"`
	NickName string `form:"nickName" search:"type:contains;column:nick_name;table:sys_user" comment:"昵称"`
	Phone    string `form:"phone" search:"type:contains;column:phone;table:sys_user" comment:"手机号"`
	RoleID   string `form:"roleId" search:"type:exact;column:role_id;table:sys_user" comment:"角色ID"`
	Sex      string `form:"sex" search:"type:exact;column:sex;table:sys_user" comment:"性别"`
	Email    string `form:"email" search:"type:contains;column:email;table:sys_user" comment:"邮箱"`
	PostID   string `form:"postId" search:"type:exact;column:post_id;table:sys_user" comment:"岗位"`
	Status   string `form:"status" search:"type:exact;column:status;table:sys_user" comment:"状态"`
}

// UserOrder carries validated administration request parameters.
type UserOrder struct {
	UserIDOrder    string `search:"type:order;column:user_id;table:sys_user" form:"userIdOrder"`
	UsernameOrder  string `search:"type:order;column:username;table:sys_user" form:"usernameOrder"`
	StatusOrder    string `search:"type:order;column:status;table:sys_user" form:"statusOrder"`
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_user" form:"createdAtOrder"`
}

// DepartmentFilter carries validated administration request parameters.
type DepartmentFilter struct {
	DeptID string `search:"type:contains;column:dept_path;table:sys_dept" form:"deptId"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *UserSearch) GetNeedSearch() any {
	return *m
}

// PasswordReset carries validated administration request parameters.
type PasswordReset struct {
	commonmodels.ControlBy
	UserID   int    `json:"userId" comment:"用户ID" vd:"$>0"` // 用户ID
	Password string `json:"password" comment:"密码" vd:"len($)>0"`
}

// GetID returns the selected resource identity.
func (s *PasswordReset) GetID() any {
	return s.UserID
}

// Generate copies request fields into a new request or supplied model.
func (s *PasswordReset) Generate(model *models.SysUser) {
	if s.UserID != 0 {
		model.UserID = s.UserID
	}
	model.Password = s.Password
}

// UserStatusUpdate carries validated administration request parameters.
type UserStatusUpdate struct {
	commonmodels.ControlBy
	UserID int    `json:"userId" comment:"用户ID" vd:"$>0"` // 用户ID
	Status string `json:"status" comment:"状态" vd:"len($)>0"`
}

// GetID returns the selected resource identity.
func (s *UserStatusUpdate) GetID() any {
	return s.UserID
}

// Generate copies request fields into a new request or supplied model.
func (s *UserStatusUpdate) Generate(model *models.SysUser) {
	if s.UserID != 0 {
		model.UserID = s.UserID
	}
	model.Status = s.Status
}

// UserCreate carries validated administration request parameters.
type UserCreate struct {
	commonmodels.ControlBy
	UserID   int    `json:"userId" comment:"用户ID"` // 用户ID
	Username string `json:"username" comment:"用户名" vd:"len($)>0"`
	Password string `json:"password" comment:"密码"`
	NickName string `json:"nickName" comment:"昵称" vd:"len($)>0"`
	Phone    string `json:"phone" comment:"手机号" vd:"len($)>0"`
	RoleID   int    `json:"roleId" comment:"角色ID"`
	Avatar   string `json:"avatar" comment:"头像"`
	Sex      string `json:"sex" comment:"性别"`
	Email    string `json:"email" comment:"邮箱" vd:"len($)>0,email"`
	DeptID   int    `json:"deptId" comment:"部门" vd:"$>0"`
	PostID   int    `json:"postId" comment:"岗位"`
	Remark   string `json:"remark" comment:"备注"`
	Status   string `json:"status" comment:"状态" vd:"len($)>0" default:"1"`
}

// Generate copies request fields into a new request or supplied model.
func (s *UserCreate) Generate(model *models.SysUser) {
	if s.UserID != 0 {
		model.UserID = s.UserID
	}
	model.Username = s.Username
	model.Password = s.Password
	model.NickName = s.NickName
	model.Phone = s.Phone
	model.RoleID = s.RoleID
	model.Avatar = s.Avatar
	model.Sex = s.Sex
	model.Email = s.Email
	model.DeptID = s.DeptID
	model.PostID = s.PostID
	model.Remark = s.Remark
	model.Status = s.Status
	model.CreateBy = s.CreateBy
}

// GetID returns the selected resource identity.
func (s *UserCreate) GetID() any {
	return s.UserID
}

// UserUpdate carries validated administration request parameters.
type UserUpdate struct {
	commonmodels.ControlBy
	UserID   int    `json:"userId" comment:"用户ID"` // 用户ID
	Username string `json:"username" comment:"用户名" vd:"len($)>0"`
	NickName string `json:"nickName" comment:"昵称" vd:"len($)>0"`
	Phone    string `json:"phone" comment:"手机号" vd:"len($)>0"`
	RoleID   int    `json:"roleId" comment:"角色ID"`
	Avatar   string `json:"avatar" comment:"头像"`
	Sex      string `json:"sex" comment:"性别"`
	Email    string `json:"email" comment:"邮箱" vd:"len($)>0,email"`
	DeptID   int    `json:"deptId" comment:"部门" vd:"$>0"`
	PostID   int    `json:"postId" comment:"岗位"`
	Remark   string `json:"remark" comment:"备注"`
	Status   string `json:"status" comment:"状态" default:"1"`
}

// Generate copies request fields into a new request or supplied model.
func (s *UserUpdate) Generate(model *models.SysUser) {
	if s.UserID != 0 {
		model.UserID = s.UserID
	}
	model.Username = s.Username
	model.NickName = s.NickName
	model.Phone = s.Phone
	model.RoleID = s.RoleID
	model.Avatar = s.Avatar
	model.Sex = s.Sex
	model.Email = s.Email
	model.DeptID = s.DeptID
	model.PostID = s.PostID
	model.Remark = s.Remark
	model.Status = s.Status
}

// GetID returns the selected resource identity.
func (s *UserUpdate) GetID() any {
	return s.UserID
}

// UserByID carries validated administration request parameters.
type UserByID struct {
	dto.ObjectByID
	commonmodels.ControlBy
}

// GetID returns the selected resource identity.
func (s *UserByID) GetID() any {
	if len(s.IDs) > 0 {
		s.IDs = append(s.IDs, s.ID)
		return s.IDs
	}
	return s.ID
}

// GenerateM copies request fields into the supplied model.
func (s *UserByID) GenerateM() (commonmodels.ActiveRecord, error) {
	return &models.SysUser{}, nil
}

// PasswordChange 密码
type PasswordChange struct {
	NewPassword string `json:"newPassword" vd:"len($)>0"`
	OldPassword string `json:"oldPassword" vd:"len($)>0"`
}
