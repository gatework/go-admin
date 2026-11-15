package dto

import contractdto "github.com/gatework/motor-core/sdk/contract/dto"

// Index and Control are thin aliases of motor-core's sdk/contract/dto
// (PRD 006 F2/F5).
type (
	// Index uses the canonical core DTO contract.
	Index = contractdto.Index
	// Control uses the canonical core DTO contract.
	Control = contractdto.Control
)
