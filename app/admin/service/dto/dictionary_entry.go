package dto

import (
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
	commonmodels "github.com/gatework/motor/common/models"
)

// DictionaryEntrySearch carries validated administration request parameters.
type DictionaryEntrySearch struct {
	dto.Pagination `search:"-"`
	ID             int    `form:"id" search:"type:exact;column:dict_code;table:sys_dict_data" comment:""`
	DictLabel      string `form:"dictLabel" search:"type:contains;column:dict_label;table:sys_dict_data" comment:""`
	DictValue      string `form:"dictValue" search:"type:contains;column:dict_value;table:sys_dict_data" comment:""`
	DictType       string `form:"dictType" search:"type:contains;column:dict_type;table:sys_dict_data" comment:""`
	Status         string `form:"status" search:"type:exact;column:status;table:sys_dict_data" comment:""`
}

// GetNeedSearch returns the tagged query parameters.
func (m *DictionaryEntrySearch) GetNeedSearch() any {
	return *m
}

// DictionaryOption carries validated administration request parameters.
type DictionaryOption struct {
	DictLabel string `json:"label"`
	DictValue string `json:"value"`
}

// DictionaryEntryCreate carries validated administration request parameters.
type DictionaryEntryCreate struct {
	commonmodels.ControlBy
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
func (s *DictionaryEntryCreate) Generate(model *models.SysDictData) {
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
func (s *DictionaryEntryCreate) GetID() any {
	return s.ID
}

// DictionaryEntryUpdate carries validated administration request parameters.
type DictionaryEntryUpdate struct {
	commonmodels.ControlBy
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
func (s *DictionaryEntryUpdate) Generate(model *models.SysDictData) {
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
func (s *DictionaryEntryUpdate) GetID() any {
	return s.ID
}

// DictionaryEntryByID carries validated administration request parameters.
type DictionaryEntryByID struct {
	ID int `uri:"dictCode"`
}

// GetID returns the selected resource identity.
func (s *DictionaryEntryByID) GetID() any {
	return s.ID
}

// DictionaryEntryDelete carries validated administration request parameters.
type DictionaryEntryDelete struct {
	commonmodels.ControlBy `json:"-"`
	IDs                    []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *DictionaryEntryDelete) GetID() any {
	return s.IDs
}
