package dto

import (
	"gorm.io/gorm"

	contractdto "github.com/gatework/motor-core/sdk/contract/dto"
)

// OrderDest forwards to motor-core's sdk/contract/dto (PRD 006 F2/F5). A
// function cannot be aliased the way a type can, so this is a pure
// pass-through rather than a `func X = pkg.X` form Go does not have.
func OrderDest(sort string, bl bool) func(db *gorm.DB) *gorm.DB {
	return contractdto.OrderDest(sort, bl)
}
