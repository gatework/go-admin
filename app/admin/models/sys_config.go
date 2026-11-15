package models

import (
	"github.com/gatework/motor/common/models"
)

// SysConfig describes persisted administration data.
type SysConfig struct {
	models.Model
	models.ControlBy
	models.ModelTime
	ConfigName  string `json:"configName" gorm:"size:128;comment:ConfigName"`   //
	ConfigKey   string `json:"configKey" gorm:"size:128;comment:ConfigKey"`     //
	ConfigValue string `json:"configValue" gorm:"size:255;comment:ConfigValue"` //
	ConfigType  string `json:"configType" gorm:"size:64;comment:ConfigType"`
	IsFrontend  string `json:"isFrontend" gorm:"size:64;comment:是否前台"` //
	Remark      string `json:"remark" gorm:"size:128;comment:Remark"`  //
}

// TableName returns the explicit persistence table name.
func (*SysConfig) TableName() string {
	return "sys_config"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysConfig) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysConfig) GetID() any {
	return e.ID
}
