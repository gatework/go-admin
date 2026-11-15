package models

import (
	"github.com/gatework/motor/common/models"
)

// SysDictType describes persisted administration data.
type SysDictType struct {
	models.ControlBy
	models.ModelTime
	ID       int    `json:"id" gorm:"primaryKey;column:dict_id;autoIncrement;comment:主键编码"`
	DictName string `json:"dictName" gorm:"size:128;comment:DictName"`
	DictType string `json:"dictType" gorm:"size:128;comment:DictType"`
	Status   int    `json:"status" gorm:"size:4;comment:Status"`
	Remark   string `json:"remark" gorm:"size:255;comment:Remark"`
}

// TableName returns the explicit persistence table name.
func (*SysDictType) TableName() string {
	return "sys_dict_type"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysDictType) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysDictType) GetID() any {
	return e.ID
}
