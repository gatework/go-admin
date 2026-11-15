package dto

import (
	"github.com/gatework/motor/app/admin/models"

	"github.com/gatework/motor/common/dto"
	common "github.com/gatework/motor/common/models"
)

// SysDictTypeGetPageReq carries validated administration request parameters.
type SysDictTypeGetPageReq struct {
	dto.Pagination `search:"-"`
	DictID         []int  `form:"dictId" search:"type:in;column:dict_id;table:sys_dict_type"`
	DictName       string `form:"dictName" search:"type:icontains;column:dict_name;table:sys_dict_type"`
	DictType       string `form:"dictType" search:"type:icontains;column:dict_type;table:sys_dict_type"`
	Status         int    `form:"status" search:"type:exact;column:status;table:sys_dict_type"`
}

// SysDictTypeOrder carries validated administration request parameters.
type SysDictTypeOrder struct {
	DictIDOrder string `search:"type:order;column:dict_id;table:sys_dict_type" form:"dictIdOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysDictTypeGetPageReq) GetNeedSearch() any {
	return *m
}

// SysDictTypeInsertReq carries validated administration request parameters.
type SysDictTypeInsertReq struct {
	common.ControlBy
	ID       int    `uri:"id"`
	DictName string `json:"dictName"`
	DictType string `json:"dictType"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysDictTypeInsertReq) Generate(model *models.SysDictType) {
	if s.ID != 0 {
		model.ID = s.ID
	}
	model.DictName = s.DictName
	model.DictType = s.DictType
	model.Status = s.Status
	model.Remark = s.Remark

}

// GetID returns the selected resource identity.
func (s *SysDictTypeInsertReq) GetID() any {
	return s.ID
}

// SysDictTypeUpdateReq carries validated administration request parameters.
type SysDictTypeUpdateReq struct {
	common.ControlBy
	ID       int    `uri:"id"`
	DictName string `json:"dictName"`
	DictType string `json:"dictType"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysDictTypeUpdateReq) Generate(model *models.SysDictType) {
	if s.ID != 0 {
		model.ID = s.ID
	}
	model.DictName = s.DictName
	model.DictType = s.DictType
	model.Status = s.Status
	model.Remark = s.Remark

}

// GetID returns the selected resource identity.
func (s *SysDictTypeUpdateReq) GetID() any {
	return s.ID
}

// SysDictTypeGetReq carries validated administration request parameters.
type SysDictTypeGetReq struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *SysDictTypeGetReq) GetID() any {
	return s.ID
}

// SysDictTypeDeleteReq carries validated administration request parameters.
type SysDictTypeDeleteReq struct {
	common.ControlBy
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *SysDictTypeDeleteReq) GetID() any {
	return s.IDs
}
