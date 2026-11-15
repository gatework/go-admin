package models

import "github.com/gatework/motor/common/models"

// SysPost describes persisted administration data.
type SysPost struct {
	models.ControlBy
	models.ModelTime
	PostID    int    `gorm:"primaryKey;autoIncrement" json:"postId"` //岗位编号
	PostName  string `gorm:"size:128;" json:"postName"`              //岗位名称
	PostCode  string `gorm:"size:128;" json:"postCode"`              //岗位代码
	Sort      int    `gorm:"size:4;" json:"sort"`                    //岗位排序
	Status    int    `gorm:"size:4;" json:"status"`                  //状态
	Remark    string `gorm:"size:255;" json:"remark"`                //描述
	DataScope string `gorm:"-" json:"dataScope"`
	Params    string `gorm:"-" json:"params"`
}

// TableName returns the explicit persistence table name.
func (*SysPost) TableName() string {
	return "sys_post"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysPost) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysPost) GetID() any {
	return e.PostID
}
