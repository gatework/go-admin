package models

import (
	"context"
	"encoding/json"
	"fmt"
	vxlan "github.com/gatework/go-vxlan"
	delivery "go-admin/app/delivery/models"
	"gorm.io/gorm/schema"
	"reflect"
)

// Explicit storage records preserve private provenance without making it part of
// the HTTP representation. Ordinary serializer:json drops json:"-" fields.
type collectionStoredTarget struct {
	Public     CollectionTarget
	Device     delivery.DeviceSnapshot
	Connection delivery.Connection
	SourceHash string
}
type changeStoredTarget struct {
	Public                    ChangeTarget
	Device                    delivery.DeviceSnapshot
	Connection                delivery.Connection
	SourceHash, ConfigHash    string
	PreEvidence, PostEvidence vxlan.Snapshot
}
type operationsSerializer struct{}

func init() { schema.RegisterSerializer("fabric_operations", operationsSerializer{}) }
func (operationsSerializer) Value(_ context.Context, _ *schema.Field, _ reflect.Value, value interface{}) (interface{}, error) {
	var data any
	switch rows := value.(type) {
	case []CollectionTarget:
		out := make([]collectionStoredTarget, len(rows))
		for i, r := range rows {
			out[i] = collectionStoredTarget{r, r.Device, r.Connection, r.SourceHash}
		}
		data = out
	case []ChangeTarget:
		out := make([]changeStoredTarget, len(rows))
		for i, r := range rows {
			out[i] = changeStoredTarget{r, r.Device, r.Connection, r.SourceHash, r.ConfigHash, r.PreEvidence, r.PostEvidence}
		}
		data = out
	default:
		return nil, fmt.Errorf("invalid Fabric operations storage")
	}
	b, e := json.Marshal(data)
	return string(b), e
}
func (operationsSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, value interface{}) error {
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	case nil:
		return nil
	default:
		return fmt.Errorf("invalid Fabric operations data")
	}
	switch field.FieldType {
	case reflect.TypeOf([]CollectionTarget{}):
		var rows []collectionStoredTarget
		if e := json.Unmarshal(raw, &rows); e != nil {
			return e
		}
		out := make([]CollectionTarget, len(rows))
		for i, r := range rows {
			out[i] = r.Public
			out[i].Device = r.Device
			out[i].Connection = r.Connection
			out[i].SourceHash = r.SourceHash
		}
		return field.Set(ctx, dst, out)
	case reflect.TypeOf([]ChangeTarget{}):
		var rows []changeStoredTarget
		if e := json.Unmarshal(raw, &rows); e != nil {
			return e
		}
		out := make([]ChangeTarget, len(rows))
		for i, r := range rows {
			out[i] = r.Public
			out[i].Device = r.Device
			out[i].Connection = r.Connection
			out[i].SourceHash = r.SourceHash
			out[i].ConfigHash = r.ConfigHash
			out[i].PreEvidence = r.PreEvidence
			out[i].PostEvidence = r.PostEvidence
		}
		return field.Set(ctx, dst, out)
	}
	return fmt.Errorf("invalid Fabric operations field")
}

// OperationsValue supplies the same private encoding for conditional map updates.
func OperationsValue(value any) (any, error) {
	return (operationsSerializer{}).Value(context.Background(), nil, reflect.Value{}, value)
}

// MarshalJSON exposes receipt metadata while retaining private configuration
// readback in storage for Satisfied and audit verification.
func (t ChangeTarget) MarshalJSON() ([]byte, error) {
	type public ChangeTarget
	return json.Marshal(struct {
		public
		PreEvidence  vxlan.Snapshot `json:"preEvidence"`
		PostEvidence vxlan.Snapshot `json:"postEvidence"`
	}{public(t), safeSnapshot(t.PreEvidence), safeSnapshot(t.PostEvidence)})
}
func safeSnapshot(s vxlan.Snapshot) vxlan.Snapshot {
	out := vxlan.Snapshot{Dimensions: map[string]vxlan.Observation{}, ObservedAt: s.ObservedAt}
	for dimension, o := range s.Dimensions {
		safe := vxlan.Observation{Name: o.Name, Status: o.Status, StartedAt: o.StartedAt, CompletedAt: o.CompletedAt}
		for _, e := range o.Evidence {
			row := vxlan.Record{}
			for _, key := range []string{"command", "sha256", "bytes", "observed_at"} {
				if v, ok := e[key]; ok {
					row[key] = v
				}
			}
			safe.Evidence = append(safe.Evidence, row)
		}
		for _, issue := range o.Issues {
			if code, ok := issue["code"]; ok {
				safe.Issues = append(safe.Issues, vxlan.Record{"code": code})
			}
		}
		out.Dimensions[dimension] = safe
	}
	return out
}
