package dto

import (
	"strings"
	"testing"
)

func circuitRequest() CircuitControlReq {
	return CircuitControlReq{CircuitNo: " ab/01 ", Name: " Branch uplink ", Provider: " Carrier ", Fee: "12.5"}
}

func TestCircuitNormalizationAndBounds(t *testing.T) {
	r := circuitRequest()
	if err := r.Validate(false); err != nil {
		t.Fatal(err)
	}
	if r.CircuitNo != "AB/01" || r.Name != "Branch uplink" || r.Fee != "12.50" || r.Status != "planned" || r.Currency != "CNY" || r.Category != "other" || r.Purpose != "other" || r.Payer != "us" {
		t.Fatalf("normalization lost contract: %+v", r)
	}
	for _, mutate := range []func(*CircuitControlReq){
		func(r *CircuitControlReq) { r.CircuitNo = "-bad" },
		func(r *CircuitControlReq) { r.CircuitNo = "中文" },
		func(r *CircuitControlReq) { r.Name = "" },
		func(r *CircuitControlReq) { r.Provider = strings.Repeat("名", 161) },
		func(r *CircuitControlReq) { r.Notes = "x\x00y" },
		func(r *CircuitControlReq) { r.Fee = "10000000000.00" },
		func(r *CircuitControlReq) { r.Fee = "1.001" },
		func(r *CircuitControlReq) { r.Fee = "1e3" },
		func(r *CircuitControlReq) { r.Fee = "-1" },
		func(r *CircuitControlReq) { r.Currency = "cny" },
		func(r *CircuitControlReq) { r.InstalledOn = "2026-02-30" },
		func(r *CircuitControlReq) { r.InstalledOn = "2026-10-26"; r.ExpiresOn = "2026-10-25" },
		func(r *CircuitControlReq) { r.AEndpoint.IP = "not-ip" },
		func(r *CircuitControlReq) { r.AEndpoint.Monitoring.InItemID = "12" },
		func(r *CircuitControlReq) {
			r.AEndpoint.Monitoring.HostID = "1"
			r.AEndpoint.Monitoring.InItemID = "2"
			r.AEndpoint.Monitoring.OutItemID = "2"
		},
		func(r *CircuitControlReq) { v := 0; r.BandwidthMbps = &v },
		func(r *CircuitControlReq) { v := 1000001; r.BandwidthMbps = &v },
		func(r *CircuitControlReq) { r.Status = "deleted" },
		func(r *CircuitControlReq) { r.BillingCycle = "weekly" },
	} {
		r = circuitRequest()
		mutate(&r)
		if r.Validate(false) == nil {
			t.Errorf("invalid input accepted: %+v", r)
		}
	}
	r = circuitRequest()
	r.Fee = "9999999999.99"
	r.InstalledOn, r.ExpiresOn = "2026-10-26", "2026-10-26"
	r.AEndpoint.IP = "2001:db8::1"
	if err := r.Validate(false); err != nil {
		t.Fatalf("valid boundary rejected: %v", err)
	}
	if r.Validate(true) == nil {
		t.Fatal("edit accepted absent version")
	}
}

func TestCircuitFiltersRejectInvalidEnumAndUnboundedInput(t *testing.T) {
	for _, r := range []CircuitPageReq{{Monitored: "yes"}, {Status: "bogus"}, {Category: "bogus"}, {Keyword: "a\x00b"}} {
		if r.Validate() == nil {
			t.Fatalf("invalid filter accepted: %+v", r)
		}
	}
	r := CircuitPageReq{}
	r.PageSize = 201
	if r.Validate() == nil {
		t.Fatal("unbounded page")
	}
}
