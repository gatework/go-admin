package fabric

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor/app/net/models/delivery"
)

// Targets keep their private provenance - device, connection, hashes and the
// raw evidence - through storage, although none of it is in their JSON.
func TestOperationsStoragePreservesPrivateProvenance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Collection{}, &Change{}); err != nil {
		t.Fatal(err)
	}
	device := delivery.DeviceSnapshot{ID: 3, IP: "192.0.2.3", Name: "leaf-1"}
	connection := delivery.Connection{DeviceID: 3, Profile: "cisco_nxos", Protocol: "ssh", Port: 22}
	evidence := vxlan.Snapshot{Dimensions: map[string]vxlan.Observation{"configuration": {Name: "configuration", Status: "complete", Records: []vxlan.Record{{"line": "secret config"}}}}}

	collection := Collection{FabricID: 1, Targets: []CollectionTarget{{DeviceID: 3, Device: device, Connection: connection, SourceHash: "src"}}}
	if err := db.Create(&collection).Error; err != nil {
		t.Fatal(err)
	}
	change := Change{FabricID: 1, Targets: []ChangeTarget{{DeviceID: 3, Device: device, Connection: connection, SourceHash: "src", ConfigHash: "cfg", PreEvidence: evidence}}}
	if err := db.Create(&change).Error; err != nil {
		t.Fatal(err)
	}

	var gotCollection Collection
	if err := db.First(&gotCollection, collection.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ct := gotCollection.Targets[0]; ct.Device != device || ct.Connection.Profile != "cisco_nxos" || ct.SourceHash != "src" {
		t.Fatalf("collection target lost provenance: %+v", ct)
	}
	var gotChange Change
	if err := db.First(&gotChange, change.ID).Error; err != nil {
		t.Fatal(err)
	}
	ch := gotChange.Targets[0]
	if ch.Device != device || ch.SourceHash != "src" || ch.ConfigHash != "cfg" || len(ch.PreEvidence.Dimensions["configuration"].Records) != 1 {
		t.Fatalf("change target lost provenance: %+v", ch)
	}
}

// The HTTP form of a change target carries evidence metadata only: command,
// digest, size and time, and issue codes - never device configuration.
func TestChangeTargetJSONOmitsConfiguration(t *testing.T) {
	now := time.Now().UTC()
	target := ChangeTarget{
		DeviceID: 3, SourceHash: "src", ConfigHash: "cfg",
		Device: delivery.DeviceSnapshot{IP: "192.0.2.3"},
		PreEvidence: vxlan.Snapshot{ObservedAt: &now, Dimensions: map[string]vxlan.Observation{"configuration": {
			Name: "configuration", Status: "complete",
			Records:  []vxlan.Record{{"line": "username admin password secret"}},
			Evidence: []vxlan.Record{{"command": "show running-config", "sha256": "abc", "bytes": 42, "output": "username admin password secret"}},
			Issues:   []vxlan.Record{{"code": "drift", "detail": "password secret"}},
		}}},
	}
	b, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, leaked := range []string{"secret", "192.0.2.3", `"src"`, `"cfg"`} {
		if strings.Contains(body, leaked) {
			t.Errorf("JSON carries %s: %s", leaked, body)
		}
	}
	for _, kept := range []string{"show running-config", `"sha256":"abc"`, `"bytes":42`, `"code":"drift"`} {
		if !strings.Contains(body, kept) {
			t.Errorf("JSON lost %s: %s", kept, body)
		}
	}
}
