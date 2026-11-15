package models

// SysColumns defines the syscolumns contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysColumns struct {
	ColumnID           int    `gorm:"primaryKey;autoIncrement" json:"columnId"`
	TableID            int    `gorm:"" json:"tableId"`
	ColumnName         string `gorm:"size:128;" json:"columnName"`
	ColumnComment      string `gorm:"column:column_comment;size:128;" json:"columnComment"`
	ColumnType         string `gorm:"column:column_type;size:128;" json:"columnType"`
	GoType             string `gorm:"column:go_type;size:128;" json:"goType"`
	GoField            string `gorm:"column:go_field;size:128;" json:"goField"`
	JSONField          string `gorm:"column:json_field;size:128;" json:"jsonField"`
	IsPK               string `gorm:"column:is_pk;size:4;" json:"isPk"`
	IsIncrement        string `gorm:"column:is_increment;size:4;" json:"isIncrement"`
	IsRequired         string `gorm:"column:is_required;size:4;" json:"isRequired"`
	IsInsert           string `gorm:"column:is_insert;size:4;" json:"isInsert"`
	IsEdit             string `gorm:"column:is_edit;size:4;" json:"isEdit"`
	IsList             string `gorm:"column:is_list;size:4;" json:"isList"`
	IsQuery            string `gorm:"column:is_query;size:4;" json:"isQuery"`
	QueryType          string `gorm:"column:query_type;size:128;" json:"queryType"`
	HTMLType           string `gorm:"column:html_type;size:128;" json:"htmlType"`
	DictType           string `gorm:"column:dict_type;size:128;" json:"dictType"`
	Sort               int    `gorm:"column:sort;" json:"sort"`
	List               string `gorm:"column:list;size:1;" json:"list"`
	PK                 bool   `gorm:"column:pk;size:1;" json:"pk"`
	Required           bool   `gorm:"column:required;size:1;" json:"required"`
	SuperColumn        bool   `gorm:"column:super_column;size:1;" json:"superColumn"`
	UsableColumn       bool   `gorm:"column:usable_column;size:1;" json:"usableColumn"`
	Increment          bool   `gorm:"column:increment;size:1;" json:"increment"`
	Insert             bool   `gorm:"column:insert;size:1;" json:"insert"`
	Edit               bool   `gorm:"column:edit;size:1;" json:"edit"`
	Query              bool   `gorm:"column:query;size:1;" json:"query"`
	Remark             string `gorm:"column:remark;size:255;" json:"remark"`
	FKTableName        string `gorm:"" json:"fkTableName"`
	FKTableNameClass   string `gorm:"" json:"fkTableNameClass"`
	FKTableNamePackage string `gorm:"" json:"fkTableNamePackage"`
	FKLabelID          string `gorm:"" json:"fkLabelId"`
	FKLabelName        string `gorm:"size:255;" json:"fkLabelName"`
	ModelTime
	ControlBy
}

// TableName returns the explicit persistence table name.
func (SysColumns) TableName() string {
	return "sys_columns"
}
