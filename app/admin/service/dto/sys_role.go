package dto

import (
	"github.com/gatework/motor/app/admin/models"
	commonmodels "github.com/gatework/motor/common/models"

	"github.com/gatework/motor/common/dto"
)

// RoleSearch carries validated administration request parameters.
type RoleSearch struct {
	dto.Pagination `search:"-"`
	RoleOrder

	RoleID    int    `form:"roleId" search:"type:exact;column:role_id;table:sys_role" comment:"角色编码"`     // 角色编码
	RoleName  string `form:"roleName" search:"type:exact;column:role_name;table:sys_role" comment:"角色名称"` // 角色名称
	Status    string `form:"status" search:"type:exact;column:status;table:sys_role" comment:"状态"`        // 状态
	RoleKey   string `form:"roleKey" search:"type:exact;column:role_key;table:sys_role" comment:"角色代码"`   // 角色代码
	RoleSort  int    `form:"roleSort" search:"type:exact;column:role_sort;table:sys_role" comment:"角色排序"` // 角色排序
	Flag      string `form:"flag" search:"type:exact;column:flag;table:sys_role" comment:"标记"`            // 标记
	Remark    string `form:"remark" search:"type:exact;column:remark;table:sys_role" comment:"备注"`        // 备注
	Admin     bool   `form:"admin" search:"type:exact;column:admin;table:sys_role" comment:"是否管理员"`
	DataScope string `form:"dataScope" search:"type:exact;column:data_scope;table:sys_role" comment:"是否管理员"`
}

// RoleOrder carries validated administration request parameters.
type RoleOrder struct {
	RoleIDOrder    string `search:"type:order;column:role_id;table:sys_role" form:"roleIdOrder"`
	RoleNameOrder  string `search:"type:order;column:role_name;table:sys_role" form:"roleNameOrder"`
	RoleSortOrder  string `search:"type:order;column:role_sort;table:sys_role" form:"roleSortOrder"`
	StatusOrder    string `search:"type:order;column:status;table:sys_role" form:"statusOrder"`
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_role" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *RoleSearch) GetNeedSearch() any {
	return *m
}

// RoleCreate carries validated administration request parameters.
type RoleCreate struct {
	commonmodels.ControlBy
	RoleID    int              `uri:"id" comment:"角色编码"`        // 角色编码
	RoleName  string           `form:"roleName" comment:"角色名称"` // 角色名称
	Status    string           `form:"status" comment:"状态"`     // 状态 1禁用 2正常
	RoleKey   string           `form:"roleKey" comment:"角色代码"`  // 角色代码
	RoleSort  int              `form:"roleSort" comment:"角色排序"` // 角色排序
	Flag      string           `form:"flag" comment:"标记"`       // 标记
	Remark    string           `form:"remark" comment:"备注"`     // 备注
	Admin     bool             `form:"admin" comment:"是否管理员"`
	DataScope string           `form:"dataScope" vd:"$=='1'||$=='2'||$=='3'||$=='4'||$=='5'"` // must be one of actions.DataScope{All,Custom,Dept,DeptTree,Self}; PRD 006 F14/H2
	SysMenu   []models.SysMenu `form:"sysMenu"`
	MenuIDs   []int            `form:"menuIds"`
	SysDept   []models.SysDept `form:"sysDept"`
	DeptIDs   []int            `form:"deptIds"`
}

// Generate copies request fields into a new request or supplied model.
func (s *RoleCreate) Generate(model *models.SysRole) {
	if s.RoleID != 0 {
		model.RoleID = s.RoleID
	}
	model.RoleName = s.RoleName
	model.Status = s.Status
	model.RoleKey = s.RoleKey
	model.RoleSort = s.RoleSort
	model.Flag = s.Flag
	model.Remark = s.Remark
	model.Admin = s.Admin
	model.DataScope = s.DataScope
	model.SysMenu = &s.SysMenu
	model.SysDept = s.SysDept
}

// GetID returns the selected resource identity.
func (s *RoleCreate) GetID() any {
	return s.RoleID
}

// RoleUpdate carries validated administration request parameters.
type RoleUpdate struct {
	commonmodels.ControlBy
	RoleID    int              `uri:"id" comment:"角色编码"`        // 角色编码
	RoleName  string           `form:"roleName" comment:"角色名称"` // 角色名称
	Status    string           `form:"status" comment:"状态"`     // 状态
	RoleKey   string           `form:"roleKey" comment:"角色代码"`  // 角色代码
	RoleSort  int              `form:"roleSort" comment:"角色排序"` // 角色排序
	Flag      string           `form:"flag" comment:"标记"`       // 标记
	Remark    string           `form:"remark" comment:"备注"`     // 备注
	Admin     bool             `form:"admin" comment:"是否管理员"`
	DataScope string           `form:"dataScope" vd:"$=='1'||$=='2'||$=='3'||$=='4'||$=='5'"` // must be one of actions.DataScope{All,Custom,Dept,DeptTree,Self}; PRD 006 F14/H2
	SysMenu   []models.SysMenu `form:"sysMenu"`
	MenuIDs   []int            `form:"menuIds"`
	SysDept   []models.SysDept `form:"sysDept"`
	DeptIDs   []int            `form:"deptIds"`
}

// Generate copies request fields into a new request or supplied model.
func (s *RoleUpdate) Generate(model *models.SysRole) {
	if s.RoleID != 0 {
		model.RoleID = s.RoleID
	}
	model.RoleName = s.RoleName
	model.Status = s.Status
	model.RoleKey = s.RoleKey
	model.RoleSort = s.RoleSort
	model.Flag = s.Flag
	model.Remark = s.Remark
	model.Admin = s.Admin
	model.DataScope = s.DataScope
	model.SysMenu = &s.SysMenu
	model.SysDept = s.SysDept
}

// GetID returns the selected resource identity.
func (s *RoleUpdate) GetID() any {
	return s.RoleID
}

// RoleStatusUpdate carries validated administration request parameters.
type RoleStatusUpdate struct {
	commonmodels.ControlBy
	RoleID int    `form:"roleId" comment:"角色编码"` // 角色编码
	Status string `form:"status" comment:"状态"`   // 状态
}

// Generate copies request fields into a new request or supplied model.
func (s *RoleStatusUpdate) Generate(model *models.SysRole) {
	if s.RoleID != 0 {
		model.RoleID = s.RoleID
	}
	model.Status = s.Status
}

// GetID returns the selected resource identity.
func (s *RoleStatusUpdate) GetID() any {
	return s.RoleID
}

// RoleByID carries validated administration request parameters.
type RoleByID struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *RoleByID) GetID() any {
	return s.ID
}

// RoleDelete carries validated administration request parameters.
type RoleDelete struct {
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *RoleDelete) GetID() any {
	return s.IDs
}

// RoleDataScopeUpdate 角色数据权限修改
type RoleDataScopeUpdate struct {
	RoleID    int    `json:"roleId" binding:"required"`
	DataScope string `json:"dataScope" binding:"required" vd:"$=='1'||$=='2'||$=='3'||$=='4'||$=='5'"` // must be one of actions.DataScope{All,Custom,Dept,DeptTree,Self}; PRD 006 F14/H2
	DeptIDs   []int  `json:"deptIds"`
}

// Generate copies request fields into a new request or supplied model.
func (s *RoleDataScopeUpdate) Generate(model *models.SysRole) {
	if s.RoleID != 0 {
		model.RoleID = s.RoleID
	}
	model.DataScope = s.DataScope
	model.DeptIDs = s.DeptIDs
}

// DepartmentRef carries validated administration request parameters.
type DepartmentRef struct {
	DeptID int `json:"DeptId"`
}
