package deviceaccess

import (
	"testing"

	"github.com/gatework/motor/app/net/models/connection"
	"github.com/gatework/motor/app/net/models/inventory"
)

func TestReadAllowsPendingDevicesAndWriteOnlyActiveOnes(t *testing.T) {
	for status, want := range map[string][2]bool{
		"pending":     {true, false},
		"active":      {true, true},
		"maintenance": {false, false},
		"disabled":    {false, false},
		"retired":     {false, false},
		"":            {false, false},
	} {
		if got := [2]bool{Read.Allows(status), Write.Allows(status)}; got != want {
			t.Errorf("%q: read, write = %v, want %v", status, got, want)
		}
	}
	if Use(0).Allows("active") {
		t.Error("the zero Use allowed a session")
	}
}

func TestEffectiveReportsTheFirstReasonASessionCannotOpen(t *testing.T) {
	configured := Config{Shared: Credentials{Username: "u", Password: "p"}, HostKeyPolicy: "strict"}
	device := inventory.Device{IP: "192.0.2.10", Status: "pending", Vendor: "H3C", OS: "Comware"}
	ssh := connection.Settings{Protocol: "ssh", Port: 22}
	for _, tc := range []struct {
		name   string
		config Config
		edit   func(*inventory.Device, *connection.Settings)
		use    Use
		want   string
	}{
		{"a pending device cannot be written", configured, nil, Write, "device_unavailable"},
		{"a retired device cannot be read", configured, func(d *inventory.Device, _ *connection.Settings) { d.Status = "retired" }, Read, "device_unavailable"},
		{"status comes before the profile", configured, func(d *inventory.Device, _ *connection.Settings) { d.Status, d.Vendor, d.OS = "retired", "", "" }, Read, "device_unavailable"},
		{"no profile", configured, func(d *inventory.Device, _ *connection.Settings) { d.Vendor, d.OS = "", "" }, Read, "profile_unknown"},
		{"bad port", configured, func(_ *inventory.Device, s *connection.Settings) { s.Port = 0 }, Read, "connection_invalid"},
		{"bad address", configured, func(d *inventory.Device, _ *connection.Settings) { d.IP = "core-1" }, Read, "address_invalid"},
		{"no credentials", Config{HostKeyPolicy: "strict"}, nil, Read, "credentials_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, s := device, ssh
			if tc.edit != nil {
				tc.edit(&d, &s)
			}
			got := tc.config.Effective(d, s, tc.use)
			if got.Reason != tc.want || got.Ready {
				t.Errorf("reason %q ready %v, want %q", got.Reason, got.Ready, tc.want)
			}
		})
	}
}
