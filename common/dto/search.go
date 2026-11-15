package dto

import (
	"gorm.io/gorm"

	contractdto "github.com/gatework/motor-core/sdk/contract/dto"
)

// DeleteRequest selects a URI record or a JSON batch for legacy job operations.
type DeleteRequest struct {
	ID  int   `uri:"id" json:"id" validate:"required"`
	IDs []int `json:"ids"`
}

// GetIDs returns positive selected IDs, or the historical zero selector when empty.
func (r DeleteRequest) GetIDs() []int {
	ids := make([]int, 0, len(r.IDs)+1)
	if r.ID > 0 {
		ids = append(ids, r.ID)
	}
	for _, id := range r.IDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, 0)
	}
	return ids
}

// GetRequest selects a job record by URI or JSON identity.
type GetRequest struct {
	ID int `uri:"id" json:"id" validate:"required"`
}

// MakeCondition resolves search tags against the database bound to each query.
func MakeCondition(q any) func(db *gorm.DB) *gorm.DB {
	return contractdto.MakeCondition(q)
}

// Paginate handles the paginate operation.
func Paginate(pageSize, pageIndex int) func(db *gorm.DB) *gorm.DB {
	return contractdto.Paginate(pageSize, pageIndex)
}
