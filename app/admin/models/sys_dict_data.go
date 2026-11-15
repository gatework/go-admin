package models

import (
	"github.com/gatework/motor/common/models"
)

// SysDictData describes persisted administration data.
type SysDictData struct {
	models.ControlBy
	models.ModelTime
	DictCode  int    `json:"dictCode" gorm:"primaryKey;column:dict_code;autoIncrement;comment:主键编码"`
	DictSort  int    `json:"dictSort" gorm:"size:20;comment:DictSort"`
	DictLabel string `json:"dictLabel" gorm:"size:128;comment:DictLabel"`
	DictValue string `json:"dictValue" gorm:"size:255;comment:DictValue"`
	DictType  string `json:"dictType" gorm:"size:64;comment:DictType"`
	CSSClass  string `json:"cssClass" gorm:"size:128;comment:CssClass"`
	ListClass string `json:"listClass" gorm:"size:128;comment:ListClass"`
	IsDefault string `json:"isDefault" gorm:"size:8;comment:IsDefault"`
	Status    int    `json:"status" gorm:"size:4;comment:Status"`
	Default   string `json:"default" gorm:"size:8;comment:Default"`
	Remark    string `json:"remark" gorm:"size:255;comment:Remark"`
}

// TableName returns the explicit persistence table name.
func (*SysDictData) TableName() string {
	return "sys_dict_data"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysDictData) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysDictData) GetID() any {
	return e.DictCode
}
