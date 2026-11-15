package models

import (
	"time"
)

// BaseModel defines the basemodel contract.
type BaseModel struct {
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt"`
}
