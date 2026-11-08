package dto

import (
	"errors"
	"strings"
	"testing"

	"go-admin/app/network"
)

func TestGroupValidationPreservesDomainBounds(t *testing.T) {
	valid := DeviceGroupReq{Code: "  SITE.Example  ", Name: " 分组 ", Enabled: true, Version: 1}
	if err := valid.Validate(true); err != nil || valid.Code != "site.example" || valid.Name != "分组" {
		t.Fatalf("normalization: %+v %v", valid, err)
	}
	for _, edit := range []func(*DeviceGroupReq){
		func(r *DeviceGroupReq) { r.Code = "1invalid" }, func(r *DeviceGroupReq) { r.Code = "has spaces" }, func(r *DeviceGroupReq) { r.Code = strings.Repeat("a", 101) },
		func(r *DeviceGroupReq) { r.Name = " " }, func(r *DeviceGroupReq) { r.Name = strings.Repeat("区", 129) }, func(r *DeviceGroupReq) { r.Description = strings.Repeat("a", 4001) },
		func(r *DeviceGroupReq) { r.Position = -1 }, func(r *DeviceGroupReq) { r.Version = 0 }, func(r *DeviceGroupReq) { id := 0; r.ParentID = &id },
	} {
		row := valid
		edit(&row)
		if err := row.Validate(true); !errors.Is(err, network.ErrInvalidInput) {
			t.Errorf("accepted invalid group: %+v %v", row, err)
		}
	}
}

func TestAssignmentRequiresDistinctBoundedVersionedSelection(t *testing.T) {
	valid := DeviceGroupAssignReq{GroupSource: "manual", Devices: []DeviceVersion{{ID: 1, Version: 1}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("manual ungrouped: %v", err)
	}
	for _, request := range []DeviceGroupAssignReq{
		{GroupSource: "auto"}, {GroupSource: "unknown", Devices: valid.Devices}, {GroupSource: "manual", Devices: []DeviceVersion{{ID: 1, Version: 0}}},
		{GroupSource: "auto", Devices: []DeviceVersion{{ID: 1, Version: 1}, {ID: 1, Version: 1}}},
	} {
		if err := request.Validate(); !errors.Is(err, network.ErrInvalidInput) {
			t.Errorf("accepted invalid assignment: %+v %v", request, err)
		}
	}
	request := DeviceGroupAssignReq{GroupSource: "auto"}
	for i := 1; i <= 200; i++ {
		request.Devices = append(request.Devices, DeviceVersion{ID: i, Version: 1})
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("maximum selection: %v", err)
	}
	request.Devices = append(request.Devices, DeviceVersion{ID: 201, Version: 1})
	if err := request.Validate(); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("unbounded assignment: %v", err)
	}
}
