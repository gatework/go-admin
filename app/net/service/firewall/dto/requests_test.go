package dto

import (
	"strings"
	"testing"

	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/common/errors"
	"github.com/gatework/motor/common/paging"
)

func TestPageValidate(t *testing.T) {
	p := Page{}
	if err := p.Validate(); err != nil || p.PageIndex != 1 || p.PageSize != paging.DefaultSize {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	minus := -1
	for name, bad := range map[string]Page{
		"page size over the cap": {PageSize: paging.MaxSize + 1},
		"long keyword":           {Keyword: strings.Repeat("k", 513)},
		"negative domain":        {DomainID: -1},
		"negative group":         {GroupID: &minus},
	} {
		if err := bad.Validate(); !errors.Is(err, types.InvalidErr) {
			t.Errorf("%s: %v, want types.InvalidErr", name, err)
		}
	}
}
