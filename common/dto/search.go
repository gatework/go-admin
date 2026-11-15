package dto

import (
	"gorm.io/gorm"

	contractdto "github.com/gatework/motor-core/sdk/contract/dto"
)

// MakeCondition resolves search tags against the database bound to each query.
func MakeCondition(q any) func(db *gorm.DB) *gorm.DB {
	return contractdto.MakeCondition(q)
}

// Paginate handles the paginate operation.
func Paginate(pageSize, pageIndex int) func(db *gorm.DB) *gorm.DB {
	return contractdto.Paginate(pageSize, pageIndex)
}
