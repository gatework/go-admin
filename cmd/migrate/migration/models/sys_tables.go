package models

// SysTables defines the systables contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysTables struct {
	TableID             int    `gorm:"primaryKey;autoIncrement" json:"tableId"`
	TBName              string `gorm:"column:table_name;size:255;" json:"tableName"`
	TableComment        string `gorm:"size:255;" json:"tableComment"`
	ClassName           string `gorm:"size:255;" json:"className"`
	TplCategory         string `gorm:"size:255;" json:"tplCategory"`
	PackageName         string `gorm:"size:255;" json:"packageName"`
	ModuleName          string `gorm:"size:255;" json:"moduleName"`
	ModuleFrontName     string `gorm:"size:255;comment:前端文件名;" json:"moduleFrontName"`
	BusinessName        string `gorm:"size:255;" json:"businessName"`
	FunctionName        string `gorm:"size:255;" json:"functionName"`
	FunctionAuthor      string `gorm:"size:255;" json:"functionAuthor"`
	PKColumn            string `gorm:"size:255;" json:"pkColumn"`
	PKGoField           string `gorm:"size:255;" json:"pkGoField"`
	PKJSONField         string `gorm:"size:255;" json:"pkJSONField"`
	Options             string `gorm:"size:255;" json:"options"`
	TreeCode            string `gorm:"size:255;" json:"treeCode"`
	TreeParentCode      string `gorm:"size:255;" json:"treeParentCode"`
	TreeName            string `gorm:"size:255;" json:"treeName"`
	Tree                bool   `gorm:"size:1;default:0;" json:"tree"`
	Crud                bool   `gorm:"size:1;default:1;" json:"crud"`
	Remark              string `gorm:"size:255;" json:"remark"`
	IsDataScope         int    `gorm:"size:1;" json:"isDataScope"`
	IsActions           int    `gorm:"size:1;" json:"isActions"`
	IsAuth              int    `gorm:"size:1;" json:"isAuth"`
	IsLogicalDelete     string `gorm:"size:1;" json:"isLogicalDelete"`
	LogicalDelete       bool   `gorm:"size:1;" json:"logicalDelete"`
	LogicalDeleteColumn string `gorm:"size:128;" json:"logicalDeleteColumn"`
	ModelTime
	ControlBy
}

// TableName returns the explicit persistence table name.
func (SysTables) TableName() string {
	return "sys_tables"
}
