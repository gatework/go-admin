package fabric

import "fmt"

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *PageReq) Validate() error {
	if r.PageIndex < 0 || r.PageIndex > 1000000 || r.PageSize < 0 || r.PageSize > 200 || len(r.Keyword) > 512 || len(r.Status) > 40 || len(r.VRF) > 128 || r.FabricID < 0 {
		return fmt.Errorf("invalid Fabric pagination")
	}
	if r.PageIndex == 0 {
		r.PageIndex = 1
	}
	if r.PageSize == 0 {
		r.PageSize = 20
	}
	return nil
}
