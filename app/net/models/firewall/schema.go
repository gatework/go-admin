// Package firewall owns manual workflow state and rebuildable native projections.
package firewall

// Schema stores schema data in the database.
func Schema() []any {
	return []any{&Domain{}, &Instance{}, &ScopeBinding{}, &ZoneBinding{}, &NetworkBinding{}, &Import{}, &Snapshot{}, &Coverage{}, &Interface{}, &Route{}, &Zone{}, &Address{}, &AddressGroup{}, &Service{}, &ServiceGroup{}, &Schedule{}, &Rule{}, &RuleBinding{}, &StaticNAT{}, &DynamicNAT{}, &NATPool{}, &Reference{}, &AddressRange{}, &ServiceTerm{}, &Request{}, &RequestRow{}, &Analysis{}, &AnalysisResult{}, &AnalysisTarget{}, &Draft{}, &CollectionBatch{}, &CollectionTarget{}}
}
