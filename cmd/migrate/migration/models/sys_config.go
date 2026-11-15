package models

// SysConfig defines the sysconfig contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysConfig struct {
	Model
	ConfigName  string `json:"configName" gorm:"type:varchar(128);comment:ConfigName"`
	ConfigKey   string `json:"configKey" gorm:"type:varchar(128);comment:ConfigKey"`
	ConfigValue string `json:"configValue" gorm:"type:varchar(255);comment:ConfigValue"`
	ConfigType  string `json:"configType" gorm:"type:varchar(64);comment:ConfigType"`
	IsFrontend  int    `json:"isFrontend" gorm:"type:varchar(64);comment:是否前台"`
	Remark      string `json:"remark" gorm:"type:varchar(128);comment:Remark"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (SysConfig) TableName() string {
	return "sys_config"
}
