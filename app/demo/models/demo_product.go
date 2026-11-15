package models

import (
	"github.com/gatework/motor/common/models"
)

// DemoProduct 示例模型
//
// 内嵌 ControlBy 与 ModelTime 后，创建人/更新人与时间戳由框架自动维护；
// 数据权限（actions.Permission）正是按 create_by 过滤，缺少 ControlBy 会使其失效。
type DemoProduct struct {
	models.Model
	models.ControlBy
	models.ModelTime
	Name   string  `json:"name" gorm:"size:128;comment:名称"`
	Code   string  `json:"code" gorm:"size:64;comment:编码"`
	Price  float64 `json:"price" gorm:"comment:单价"`
	Status string  `json:"status" gorm:"size:4;comment:状态"`
	Remark string  `json:"remark" gorm:"size:255;comment:备注"`
}

// TableName returns the explicit persistence table name.
func (*DemoProduct) TableName() string {
	return "demo_product"
}

// GetID returns the selected resource identity.
func (e *DemoProduct) GetID() any {
	return e.ID
}
