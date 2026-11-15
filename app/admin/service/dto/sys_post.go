package dto

import (
	"github.com/gatework/motor/app/admin/models"
	common "github.com/gatework/motor/common/models"

	"github.com/gatework/motor/common/dto"
)

// SysPostPageReq 列表或者搜索使用结构体
type SysPostPageReq struct {
	dto.Pagination `search:"-"`
	PostID         int    `form:"postId" search:"type:exact;column:post_id;table:sys_post" comment:"id"`        // id
	PostName       string `form:"postName" search:"type:contains;column:post_name;table:sys_post" comment:"名称"` // 名称
	PostCode       string `form:"postCode" search:"type:contains;column:post_code;table:sys_post" comment:"编码"` // 编码
	Sort           int    `form:"sort" search:"type:exact;column:sort;table:sys_post" comment:"排序"`             // 排序
	Status         int    `form:"status" search:"type:exact;column:status;table:sys_post" comment:"状态"`         // 状态
	Remark         string `form:"remark" search:"type:exact;column:remark;table:sys_post" comment:"备注"`         // 备注
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysPostPageReq) GetNeedSearch() any {
	return *m
}

// SysPostInsertReq 增使用的结构体
type SysPostInsertReq struct {
	common.ControlBy
	PostID   int    `uri:"id"  comment:"id"`
	PostName string `form:"postName"  comment:"名称"`
	PostCode string `form:"postCode" comment:"编码"`
	Sort     int    `form:"sort" comment:"排序"`
	Status   int    `form:"status"   comment:"状态"`
	Remark   string `form:"remark"   comment:"备注"`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysPostInsertReq) Generate(model *models.SysPost) {
	model.PostName = s.PostName
	model.PostCode = s.PostCode
	model.Sort = s.Sort
	model.Status = s.Status
	model.Remark = s.Remark
	if s.UpdateBy != 0 {
		model.UpdateBy = s.UpdateBy
	}
	if s.CreateBy != 0 {
		model.CreateBy = s.CreateBy
	}
}

// GetID returns the selected resource identity.
func (s *SysPostInsertReq) GetID() any {
	return s.PostID
}

// SysPostUpdateReq 改使用的结构体
type SysPostUpdateReq struct {
	common.ControlBy
	PostID   int    `uri:"id"  comment:"id"`
	PostName string `form:"postName"  comment:"名称"`
	PostCode string `form:"postCode" comment:"编码"`
	Sort     int    `form:"sort" comment:"排序"`
	Status   int    `form:"status"   comment:"状态"`
	Remark   string `form:"remark"   comment:"备注"`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysPostUpdateReq) Generate(model *models.SysPost) {
	model.PostID = s.PostID
	model.PostName = s.PostName
	model.PostCode = s.PostCode
	model.Sort = s.Sort
	model.Status = s.Status
	model.Remark = s.Remark
	if s.UpdateBy != 0 {
		model.UpdateBy = s.UpdateBy
	}
	if s.CreateBy != 0 {
		model.CreateBy = s.CreateBy
	}
}

// GetID returns the selected resource identity.
func (s *SysPostUpdateReq) GetID() any {
	return s.PostID
}

// SysPostGetReq 获取单个的结构体
type SysPostGetReq struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *SysPostGetReq) GetID() any {
	return s.ID
}

// SysPostDeleteReq 删除的结构体
type SysPostDeleteReq struct {
	common.ControlBy
	IDs []int `json:"ids"`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysPostDeleteReq) Generate(model *models.SysPost) {
	if s.UpdateBy != 0 {
		model.UpdateBy = s.UpdateBy
	}
	if s.CreateBy != 0 {
		model.CreateBy = s.CreateBy
	}
}

// GetID returns the selected resource identity.
func (s *SysPostDeleteReq) GetID() any {
	return s.IDs
}
