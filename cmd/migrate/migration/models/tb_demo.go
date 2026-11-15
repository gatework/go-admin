package models

// TbDemo defines the tbdemo contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type TbDemo struct {
	Model
	Name string `json:"name" gorm:"type:varchar(128);comment:名称"`
	ModelTime
	ControlBy
}

// TableName returns the explicit persistence table name.
func (TbDemo) TableName() string {
	return "tb_demo"
}
