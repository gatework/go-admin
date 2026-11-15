package models

import contractmodels "github.com/gatework/motor-core/sdk/contract/models"

// ControlBy, Model and ModelTime are thin aliases of motor-core's
// sdk/contract/models (PRD 006 F1/F5). A type alias is the same type, not a
// new one, so every model that embeds these keeps its GORM tags, JSON tags
// and method set untouched.
type (
	// ControlBy uses the canonical core model contract.
	ControlBy = contractmodels.ControlBy
	// Model uses the canonical core model contract.
	Model = contractmodels.Model
	// ModelTime uses the canonical core model contract.
	ModelTime = contractmodels.ModelTime
)
