package dto

import (
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
	common "github.com/gatework/motor/common/models"
)

// SysConfigGetPageReq 列表或者搜索使用结构体
type SysConfigGetPageReq struct {
	dto.Pagination `search:"-"`
	SysConfigOrder
	ConfigName string `form:"configName" search:"type:contains;column:config_name;table:sys_config"`
	ConfigKey  string `form:"configKey" search:"type:contains;column:config_key;table:sys_config"`
	ConfigType string `form:"configType" search:"type:exact;column:config_type;table:sys_config"`
	IsFrontend string `form:"isFrontend" search:"type:exact;column:is_frontend;table:sys_config"`
}

// SysConfigOrder carries validated administration request parameters.
type SysConfigOrder struct {
	IDOrder         string `search:"type:order;column:id;table:sys_config" form:"idOrder"`
	ConfigNameOrder string `search:"type:order;column:config_name;table:sys_config" form:"configNameOrder"`
	ConfigKeyOrder  string `search:"type:order;column:config_key;table:sys_config" form:"configKeyOrder"`
	ConfigTypeOrder string `search:"type:order;column:config_type;table:sys_config" form:"configTypeOrder"`
	CreatedAtOrder  string `search:"type:order;column:created_at;table:sys_config" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysConfigGetPageReq) GetNeedSearch() any {
	return *m
}

// SysConfigGetToSysAppReq carries validated administration request parameters.
type SysConfigGetToSysAppReq struct {
	IsFrontend string `form:"isFrontend" search:"type:exact;column:is_frontend;table:sys_config"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysConfigGetToSysAppReq) GetNeedSearch() any {
	return *m
}

// SysConfigControl 增、改使用的结构体
type SysConfigControl struct {
	common.ControlBy
	ID          int    `uri:"id" json:"-" comment:"编码"` // 编码
	ConfigName  string `json:"configName" comment:""`
	ConfigKey   string `uri:"configKey" json:"configKey" comment:""`
	ConfigValue string `json:"configValue" comment:""`
	ConfigType  string `json:"configType" comment:""`
	IsFrontend  string `json:"isFrontend"`
	Remark      string `json:"remark" comment:""`
}

// Generate 结构体数据转化 从 SysConfigControl 至 system.SysConfig 对应的模型
func (s *SysConfigControl) Generate(model *models.SysConfig) {
	if s.ID != 0 {
		model.Model = common.Model{ID: s.ID}
	}
	model.ConfigName = s.ConfigName
	model.ConfigKey = s.ConfigKey
	model.ConfigValue = s.ConfigValue
	model.ConfigType = s.ConfigType
	model.IsFrontend = s.IsFrontend
	model.Remark = s.Remark

}

// GetID returns the selected resource identity.
func (s *SysConfigControl) GetID() any {
	return s.ID
}

// GetSetSysConfigReq 增、改使用的结构体
type GetSetSysConfigReq struct {
	ConfigKey   string `json:"configKey" comment:""`
	ConfigValue string `json:"configValue" comment:""`
}

// Generate 结构体数据转化 从 SysConfigControl 至 system.SysConfig 对应的模型
func (s *GetSetSysConfigReq) Generate(model *models.SysConfig) {
	model.ConfigValue = s.ConfigValue
}

// UpdateSetSysConfigReq carries validated administration request parameters.
type UpdateSetSysConfigReq map[string]string

// SysConfigByKeyReq 根据Key获取配置
type SysConfigByKeyReq struct {
	ConfigKey string `uri:"configKey" search:"type:contains;column:config_key;table:sys_config"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysConfigByKeyReq) GetNeedSearch() any {
	return *m
}

// GetSysConfigByKEYForServiceResp carries validated administration request parameters.
type GetSysConfigByKEYForServiceResp struct {
	ConfigKey   string `json:"configKey" comment:""`
	ConfigValue string `json:"configValue" comment:""`
	// IsFrontend decides who may read the value; it is not part of the response.
	IsFrontend string `json:"-"`
}

// SysConfigGetReq carries validated administration request parameters.
type SysConfigGetReq struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *SysConfigGetReq) GetID() any {
	return s.ID
}

// SysConfigDeleteReq carries validated administration request parameters.
type SysConfigDeleteReq struct {
	common.ControlBy
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *SysConfigDeleteReq) GetID() any {
	return s.IDs
}
