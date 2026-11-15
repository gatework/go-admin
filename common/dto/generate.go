package dto

import contractdto "github.com/gatework/motor-core/sdk/contract/dto"

// ObjectById, ObjectGetReq and ObjectDeleteReq are thin aliases of
// motor-core's sdk/contract/dto (PRD 006 F2/F5).
type (
	// ObjectByID uses the canonical core DTO contract.
	ObjectByID = contractdto.ObjectByID
	// ObjectGetReq uses the canonical core DTO contract.
	ObjectGetReq = contractdto.ObjectGetReq
	// ObjectDeleteReq uses the canonical core DTO contract.
	ObjectDeleteReq = contractdto.ObjectDeleteReq
)
