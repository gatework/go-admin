package models

import contractmodels "github.com/gatework/motor-core/sdk/contract/models"

// Response and Page are thin aliases of motor-core's sdk/contract/models
// (PRD 006 F1/F5).
type (
	// Response uses the canonical core model contract.
	Response = contractmodels.Response
	// Page uses the canonical core model contract.
	Page = contractmodels.Page
)
