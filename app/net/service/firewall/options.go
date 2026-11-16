package firewall

import (
	"context"

	"github.com/gatework/firemon/config"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

// nativeVendors are the vendors firemon parses and renders natively. firemon's
// vendor registry holds the same list but does not export it; read it from
// there once it does.
var nativeVendors = []string{"paloalto", "srx", "huawei", "hillstone", "fortinet", "netscreen", "asa"}

// Options is what the workbench offers and the bounds it enforces.
type Options struct {
	Domains         []models.Domain `json:"domains"`
	Vendors         []string        `json:"vendors"`
	Limits          Limits          `json:"limits"`
	DefaultProtocol string          `json:"defaultProtocol"`
}

// Limits are the workbench's input and concurrency bounds.
type Limits struct {
	MaxRows               int `json:"maxRows"`
	MaxDesignRows         int `json:"maxDesignRows"`
	MaxFileBytes          int `json:"maxFileBytes"`
	MaxCollectionTargets  int `json:"maxCollectionTargets"`
	CollectionConcurrency int `json:"collectionConcurrency"`
}

// Options returns the workbench capabilities, with the domains to choose from.
func (s *Firewall) Options(ctx context.Context) (Options, error) {
	domains, err := s.Domains(ctx)
	return Options{
		Domains: domains,
		Vendors: nativeVendors,
		Limits: Limits{
			MaxRows: engine.MaxRows, MaxDesignRows: engine.MaxDesignRows, MaxFileBytes: config.DefaultMaxBytes,
			MaxCollectionTargets: MaxCollectionTargets, CollectionConcurrency: CollectionConcurrency,
		},
		DefaultProtocol: "tcp",
	}, err
}

// Normalize previews structured input without saving anything.
func (s *Firewall) Normalize(in types.Input) (types.Normalized, error) { return engine.Normalize(in) }
