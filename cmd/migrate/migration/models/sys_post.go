package models

// SysPost defines the syspost contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysPost struct {
	PostID   int    `gorm:"primaryKey;autoIncrement" json:"postId"`
	PostName string `gorm:"size:128;" json:"postName"`
	PostCode string `gorm:"size:128;" json:"postCode"`
	Sort     int    `gorm:"size:4;" json:"sort"`
	Status   int    `gorm:"size:4;" json:"status"`
	Remark   string `gorm:"size:255;" json:"remark"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (SysPost) TableName() string {
	return "sys_post"
}
