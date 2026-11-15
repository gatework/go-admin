package dto

import (
	"github.com/gatework/motor/app/admin/models"
	commonmodels "github.com/gatework/motor/common/models"
)

// DepartmentSearch 列表或者搜索使用结构体
type DepartmentSearch struct {
	DeptID   int    `form:"deptId" search:"type:exact;column:dept_id;table:sys_dept" comment:"id"`       //id
	ParentID int    `form:"parentId" search:"type:exact;column:parent_id;table:sys_dept" comment:"上级部门"` //上级部门
	DeptPath string `form:"deptPath" search:"type:exact;column:dept_path;table:sys_dept" comment:""`     //路径
	DeptName string `form:"deptName" search:"type:exact;column:dept_name;table:sys_dept" comment:"部门名称"` //部门名称
	Sort     int    `form:"sort" search:"type:exact;column:sort;table:sys_dept" comment:"排序"`            //排序
	Leader   string `form:"leader" search:"type:exact;column:leader;table:sys_dept" comment:"负责人"`       //负责人
	Phone    string `form:"phone" search:"type:exact;column:phone;table:sys_dept" comment:"手机"`          //手机
	Email    string `form:"email" search:"type:exact;column:email;table:sys_dept" comment:"邮箱"`          //邮箱
	Status   string `form:"status" search:"type:exact;column:status;table:sys_dept" comment:"状态"`        //状态
}

// GetNeedSearch returns the tagged query parameters.
func (m *DepartmentSearch) GetNeedSearch() any {
	return *m
}

// DepartmentCreate carries validated administration request parameters.
type DepartmentCreate struct {
	commonmodels.ControlBy
	DeptID   int    `uri:"id" comment:"编码"`                                         // 编码
	ParentID int    `json:"parentId" comment:"上级部门" vd:"?"`                         //上级部门
	DeptPath string `json:"deptPath" comment:""`                                    //路径
	DeptName string `json:"deptName" comment:"部门名称" vd:"len($)>0"`                  //部门名称
	Sort     int    `json:"sort" comment:"排序" vd:"?"`                               //排序
	Leader   string `json:"leader" comment:"负责人" vd:"@:len($)>0; msg:'leader不能为空'"` //负责人
	Phone    string `json:"phone" comment:"手机" vd:"?"`                              //手机
	Email    string `json:"email" comment:"邮箱" vd:"?"`                              //邮箱
	Status   int    `json:"status" comment:"状态" vd:"$>0"`                           //状态
}

// Generate copies request fields into a new request or supplied model.
func (s *DepartmentCreate) Generate(model *models.SysDept) {
	if s.DeptID != 0 {
		model.DeptID = s.DeptID
	}
	model.DeptName = s.DeptName
	model.ParentID = s.ParentID
	model.DeptPath = s.DeptPath
	model.Sort = s.Sort
	model.Leader = s.Leader
	model.Phone = s.Phone
	model.Email = s.Email
	model.Status = s.Status
}

// GetID returns the selected resource identity.
func (s *DepartmentCreate) GetID() any {
	return s.DeptID
}

// DepartmentUpdate carries validated administration request parameters.
type DepartmentUpdate struct {
	commonmodels.ControlBy
	DeptID   int    `uri:"id" comment:"编码"`                                         // 编码
	ParentID int    `json:"parentId" comment:"上级部门" vd:"?"`                         //上级部门
	DeptPath string `json:"deptPath" comment:""`                                    //路径
	DeptName string `json:"deptName" comment:"部门名称" vd:"len($)>0"`                  //部门名称
	Sort     int    `json:"sort" comment:"排序" vd:"?"`                               //排序
	Leader   string `json:"leader" comment:"负责人" vd:"@:len($)>0; msg:'leader不能为空'"` //负责人
	Phone    string `json:"phone" comment:"手机" vd:"?"`                              //手机
	Email    string `json:"email" comment:"邮箱" vd:"?"`                              //邮箱
	Status   int    `json:"status" comment:"状态" vd:"$>0"`                           //状态
}

// Generate 结构体数据转化 从 SysDeptControl 至 SysDept 对应的模型
func (s *DepartmentUpdate) Generate(model *models.SysDept) {
	if s.DeptID != 0 {
		model.DeptID = s.DeptID
	}
	model.DeptName = s.DeptName
	model.ParentID = s.ParentID
	model.DeptPath = s.DeptPath
	model.Sort = s.Sort
	model.Leader = s.Leader
	model.Phone = s.Phone
	model.Email = s.Email
	model.Status = s.Status
}

// GetID returns the selected resource identity.
func (s *DepartmentUpdate) GetID() any {
	return s.DeptID
}

// DepartmentByID carries validated administration request parameters.
type DepartmentByID struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *DepartmentByID) GetID() any {
	return s.ID
}

// DepartmentDelete carries validated administration request parameters.
type DepartmentDelete struct {
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *DepartmentDelete) GetID() any {
	return s.IDs
}

// DepartmentLabel carries validated administration request parameters.
type DepartmentLabel struct {
	ID       int               `gorm:"-" json:"id"`
	Label    string            `gorm:"-" json:"label"`
	Children []DepartmentLabel `gorm:"-" json:"children"`
}
