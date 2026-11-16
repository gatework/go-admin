package fabric

import (
	"strings"
	"testing"

	"github.com/gatework/motor/app/net/models/fabric"
	"github.com/gatework/motor/common/errors"
	"github.com/gatework/motor/common/paging"
)

func TestPageReqValidate(t *testing.T) {
	r := PageReq{}
	if err := r.Validate(); err != nil || r.PageIndex != 1 || r.PageSize != paging.DefaultSize {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	for name, bad := range map[string]PageReq{
		"page size over the cap": {PageSize: paging.MaxSize + 1},
		"negative page":          {PageIndex: -1},
		"long keyword":           {Keyword: strings.Repeat("k", 513)},
		"long status":            {Status: strings.Repeat("s", 41)},
		"long vrf":               {VRF: strings.Repeat("v", 129)},
		"negative fabric":        {FabricID: -1},
	} {
		if err := bad.Validate(); !errors.Is(err, fabric.InvalidErr) {
			t.Errorf("%s: %v, want fabric.InvalidErr", name, err)
		}
	}
}
