package models

// DictType defines the dicttype contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type DictType struct {
	DictID   int    `gorm:"primaryKey;autoIncrement;" json:"dictId"`
	DictName string `gorm:"size:128;" json:"dictName"`
	DictType string `gorm:"size:128;" json:"dictType"`
	Status   int    `gorm:"size:4;" json:"status"`
	Remark   string `gorm:"size:255;" json:"remark"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (DictType) TableName() string {
	return "sys_dict_type"
}
