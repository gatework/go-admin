package models

// SysMenu defines the sysmenu contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysMenu struct {
	MenuID     int      `json:"menuId" gorm:"primaryKey;autoIncrement"`
	MenuName   string   `json:"menuName" gorm:"size:128;"`
	Title      string   `json:"title" gorm:"size:128;"`
	Icon       string   `json:"icon" gorm:"size:128;"`
	Path       string   `json:"path" gorm:"size:128;"`
	Paths      string   `json:"paths" gorm:"size:128;"`
	MenuType   string   `json:"menuType" gorm:"size:1;"`
	Action     string   `json:"action" gorm:"size:16;"`
	Permission string   `json:"permission" gorm:"size:255;"`
	ParentID   int      `json:"parentId" gorm:"size:11;"`
	NoCache    bool     `json:"noCache" gorm:"size:8;"`
	Breadcrumb string   `json:"breadcrumb" gorm:"size:255;"`
	Component  string   `json:"component" gorm:"size:255;"`
	Sort       int      `json:"sort" gorm:"size:4;"`
	Visible    string   `json:"visible" gorm:"size:1;"`
	IsFrame    string   `json:"isFrame" gorm:"size:1;DEFAULT:0;"`
	SysAPI     []SysAPI `json:"sysApi" gorm:"many2many:sys_menu_api_rule"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (SysMenu) TableName() string {
	return "sys_menu"
}
