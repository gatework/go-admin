package dto

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
)

// ConfigSearch filters the configuration list.
type ConfigSearch struct {
	dto.Pagination `search:"-"`
	ConfigOrder
	ConfigName string `form:"configName" search:"type:contains;column:config_name;table:sys_config"`
	ConfigKey  string `form:"configKey" search:"type:contains;column:config_key;table:sys_config"`
	ConfigType string `form:"configType" search:"type:exact;column:config_type;table:sys_config"`
	IsFrontend string `form:"isFrontend" search:"type:exact;column:is_frontend;table:sys_config"`
}

// ConfigOrder sorts the configuration list.
type ConfigOrder struct {
	IDOrder         string `search:"type:order;column:id;table:sys_config" form:"idOrder"`
	ConfigNameOrder string `search:"type:order;column:config_name;table:sys_config" form:"configNameOrder"`
	ConfigKeyOrder  string `search:"type:order;column:config_key;table:sys_config" form:"configKeyOrder"`
	ConfigTypeOrder string `search:"type:order;column:config_type;table:sys_config" form:"configTypeOrder"`
	CreatedAtOrder  string `search:"type:order;column:created_at;table:sys_config" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (s *ConfigSearch) GetNeedSearch() any { return *s }

// Bind decodes the query.
func (s *ConfigSearch) Bind(c *gin.Context) error { return c.ShouldBind(s) }

// SysConfigGetToSysAppReq carries validated administration request parameters.
type SysConfigGetToSysAppReq struct {
	IsFrontend string `form:"isFrontend" search:"type:exact;column:is_frontend;table:sys_config"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysConfigGetToSysAppReq) GetNeedSearch() any {
	return *m
}

// ConfigControl is the body of creating a configuration entry and of
// replacing one. On update the key in the path wins over any id in the body.
type ConfigControl struct {
	ID          int    `uri:"id" json:"-"`
	ConfigName  string `json:"configName"`
	ConfigKey   string `json:"configKey"`
	ConfigValue string `json:"configValue"`
	ConfigType  string `json:"configType"`
	IsFrontend  string `json:"isFrontend"`
	Remark      string `json:"remark"`
}

// Bind decodes the body and, on update, the key from the path.
func (s *ConfigControl) Bind(c *gin.Context) error {
	if err := c.ShouldBindJSON(s); err != nil {
		return err
	}
	if c.Request.Method == http.MethodPost {
		s.ID = 0
	} else {
		if err := c.ShouldBindUri(s); err != nil {
			return err
		}
		if s.ID <= 0 {
			return errors.New("id must be positive")
		}
	}
	if s.ConfigKey == "" {
		return errors.New("configKey is required")
	}
	return nil
}

// UpdateFields makes an update a full replace of the writable columns: the
// form sends every field, and an emptied value must be written as empty.
func (*ConfigControl) UpdateFields() []string {
	return []string{"config_name", "config_key", "config_value", "config_type", "is_frontend", "remark"}
}

// ToModel builds the row to write; the actions stamp creator and updater.
func (s *ConfigControl) ToModel() (*models.SysConfig, error) {
	row := &models.SysConfig{
		ConfigName:  s.ConfigName,
		ConfigKey:   s.ConfigKey,
		ConfigValue: s.ConfigValue,
		ConfigType:  s.ConfigType,
		IsFrontend:  s.IsFrontend,
		Remark:      s.Remark,
	}
	row.ID = s.ID
	return row, nil
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

// ConfigByID names one configuration entry in the URI, or several in a
// DELETE body.
type ConfigByID struct {
	dto.ObjectByID
}
