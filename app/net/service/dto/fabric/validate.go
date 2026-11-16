package fabric

import (
	"github.com/gatework/motor/app/net/models/fabric"
	"github.com/gatework/motor/common/paging"
)

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *PageReq) Validate() error {
	if !paging.Normalize(&r.PageIndex, &r.PageSize) || len(r.Keyword) > 512 || len(r.Status) > 40 || len(r.VRF) > 128 || r.FabricID < 0 {
		return fabric.InvalidErr
	}
	return nil
}
