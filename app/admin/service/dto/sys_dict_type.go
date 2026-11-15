package dto

import (
	"github.com/gatework/motor/app/admin/models"

	"github.com/gatework/motor/common/dto"
	common "github.com/gatework/motor/common/models"
)

// DictionarySearch carries validated administration request parameters.
type DictionarySearch struct {
	dto.Pagination `search:"-"`
	DictionaryOrder
	DictID   []int  `form:"dictId" search:"type:in;column:dict_id;table:sys_dict_type"`
	DictName string `form:"dictName" search:"type:icontains;column:dict_name;table:sys_dict_type"`
	DictType string `form:"dictType" search:"type:icontains;column:dict_type;table:sys_dict_type"`
	Status   int    `form:"status" search:"type:exact;column:status;table:sys_dict_type"`
}

// DictionaryOrder carries validated administration request parameters.
type DictionaryOrder struct {
	DictIDOrder string `search:"type:order;column:dict_id;table:sys_dict_type" form:"dictIdOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *DictionarySearch) GetNeedSearch() any {
	return *m
}

// DictionaryCreate carries validated administration request parameters.
type DictionaryCreate struct {
	common.ControlBy
	ID       int    `uri:"id"`
	DictName string `json:"dictName"`
	DictType string `json:"dictType"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Generate copies request fields into a new request or supplied model.
func (s *DictionaryCreate) Generate(model *models.SysDictType) {
	if s.ID != 0 {
		model.ID = s.ID
	}
	model.DictName = s.DictName
	model.DictType = s.DictType
	model.Status = s.Status
	model.Remark = s.Remark

}

// GetID returns the selected resource identity.
func (s *DictionaryCreate) GetID() any {
	return s.ID
}

// DictionaryUpdate carries validated administration request parameters.
type DictionaryUpdate struct {
	common.ControlBy
	ID       int    `uri:"id"`
	DictName string `json:"dictName"`
	DictType string `json:"dictType"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Generate copies request fields into a new request or supplied model.
func (s *DictionaryUpdate) Generate(model *models.SysDictType) {
	if s.ID != 0 {
		model.ID = s.ID
	}
	model.DictName = s.DictName
	model.DictType = s.DictType
	model.Status = s.Status
	model.Remark = s.Remark

}

// GetID returns the selected resource identity.
func (s *DictionaryUpdate) GetID() any {
	return s.ID
}

// DictionaryByID carries validated administration request parameters.
type DictionaryByID struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *DictionaryByID) GetID() any {
	return s.ID
}

// DictionaryDelete carries validated administration request parameters.
type DictionaryDelete struct {
	common.ControlBy
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *DictionaryDelete) GetID() any {
	return s.IDs
}
