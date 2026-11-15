package models

import "github.com/gatework/motor/common/models"

// SysAPI describes persisted administration data.
type SysAPI struct {
	models.ModelTime
	models.ControlBy
	ID     int    `json:"id" gorm:"primaryKey;autoIncrement;comment:主键编码"`
	Handle string `json:"handle" gorm:"size:128;comment:handle"`
	Title  string `json:"title" gorm:"size:128;comment:标题"`
	Path   string `json:"path" gorm:"size:128;comment:地址"`
	Action string `json:"action" gorm:"size:16;comment:请求类型"`
	Type   string `json:"type" gorm:"size:16;comment:接口类型"`
	// AppCode identifies which application's seed.SeedMenus call wrote this
	// row; empty for the host's own built-in APIs. Same NOT NULL DEFAULT ''
	// reasoning as SysMenu.AppCode.
	AppCode string `json:"appCode" gorm:"type:varchar(64);not null;default:'';index:idx_sys_api_app_code;comment:AppCode"`
}

// TableName returns the explicit persistence table name.
func (*SysAPI) TableName() string {
	return "sys_api"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysAPI) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysAPI) GetID() any {
	return e.ID
}
