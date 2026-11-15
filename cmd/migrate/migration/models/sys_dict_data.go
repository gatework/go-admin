package models

// DictData defines the dictdata contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type DictData struct {
	DictCode  int    `gorm:"primaryKey;autoIncrement;" json:"dictCode" example:"1"`
	DictSort  int    `gorm:"" json:"dictSort"`
	DictLabel string `gorm:"size:128;" json:"dictLabel"`
	DictValue string `gorm:"size:255;" json:"dictValue"`
	DictType  string `gorm:"size:64;" json:"dictType"`
	CSSClass  string `gorm:"size:128;" json:"cssClass"`
	ListClass string `gorm:"size:128;" json:"listClass"`
	IsDefault string `gorm:"size:8;" json:"isDefault"`
	Status    int    `gorm:"size:4;" json:"status"`
	Default   string `gorm:"size:8;" json:"default"`
	Remark    string `gorm:"size:255;" json:"remark"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (DictData) TableName() string {
	return "sys_dict_data"
}
