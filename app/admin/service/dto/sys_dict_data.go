package dto

import (
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
	common "github.com/gatework/motor/common/models"
)

// SysDictDataGetPageReq carries validated administration request parameters.
type SysDictDataGetPageReq struct {
	dto.Pagination `search:"-"`
	ID             int    `form:"id" search:"type:exact;column:dict_code;table:sys_dict_data" comment:""`
	DictLabel      string `form:"dictLabel" search:"type:contains;column:dict_label;table:sys_dict_data" comment:""`
	DictValue      string `form:"dictValue" search:"type:contains;column:dict_value;table:sys_dict_data" comment:""`
	DictType       string `form:"dictType" search:"type:contains;column:dict_type;table:sys_dict_data" comment:""`
	Status         string `form:"status" search:"type:exact;column:status;table:sys_dict_data" comment:""`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysDictDataGetPageReq) GetNeedSearch() any {
	return *m
}

// SysDictDataGetAllResp carries validated administration request parameters.
type SysDictDataGetAllResp struct {
	DictLabel string `json:"label"`
	DictValue string `json:"value"`
}

// SysDictDataInsertReq carries validated administration request parameters.
type SysDictDataInsertReq struct {
	common.ControlBy
	ID        int    `json:"-" comment:""`
	DictSort  int    `json:"dictSort" comment:""`
	DictLabel string `json:"dictLabel" comment:""`
	DictValue string `json:"dictValue" comment:""`
	DictType  string `json:"dictType" comment:""`
	CSSClass  string `json:"cssClass" comment:""`
	ListClass string `json:"listClass" comment:""`
	IsDefault string `json:"isDefault" comment:""`
	Status    int    `json:"status" comment:""`
	Default   string `json:"default" comment:""`
	Remark    string `json:"remark" comment:""`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysDictDataInsertReq) Generate(model *models.SysDictData) {
	model.DictCode = s.ID
	model.DictSort = s.DictSort
	model.DictLabel = s.DictLabel
	model.DictValue = s.DictValue
	model.DictType = s.DictType
	model.CSSClass = s.CSSClass
	model.ListClass = s.ListClass
	model.IsDefault = s.IsDefault
	model.Status = s.Status
	model.Default = s.Default
	model.Remark = s.Remark
}

// GetID returns the selected resource identity.
func (s *SysDictDataInsertReq) GetID() any {
	return s.ID
}

// SysDictDataUpdateReq carries validated administration request parameters.
type SysDictDataUpdateReq struct {
	common.ControlBy
	ID        int    `uri:"dictCode" comment:""`
	DictSort  int    `json:"dictSort" comment:""`
	DictLabel string `json:"dictLabel" comment:""`
	DictValue string `json:"dictValue" comment:""`
	DictType  string `json:"dictType" comment:""`
	CSSClass  string `json:"cssClass" comment:""`
	ListClass string `json:"listClass" comment:""`
	IsDefault string `json:"isDefault" comment:""`
	Status    int    `json:"status" comment:""`
	Default   string `json:"default" comment:""`
	Remark    string `json:"remark" comment:""`
}

// Generate copies request fields into a new request or supplied model.
func (s *SysDictDataUpdateReq) Generate(model *models.SysDictData) {
	model.DictCode = s.ID
	model.DictSort = s.DictSort
	model.DictLabel = s.DictLabel
	model.DictValue = s.DictValue
	model.DictType = s.DictType
	model.CSSClass = s.CSSClass
	model.ListClass = s.ListClass
	model.IsDefault = s.IsDefault
	model.Status = s.Status
	model.Default = s.Default
	model.Remark = s.Remark
}

// GetID returns the selected resource identity.
func (s *SysDictDataUpdateReq) GetID() any {
	return s.ID
}

// SysDictDataGetReq carries validated administration request parameters.
type SysDictDataGetReq struct {
	ID int `uri:"dictCode"`
}

// GetID returns the selected resource identity.
func (s *SysDictDataGetReq) GetID() any {
	return s.ID
}

// SysDictDataDeleteReq carries validated administration request parameters.
type SysDictDataDeleteReq struct {
	common.ControlBy `json:"-"`
	IDs              []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *SysDictDataDeleteReq) GetID() any {
	return s.IDs
}
