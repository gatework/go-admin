package service

import (
	vxlan "github.com/gatework/go-vxlan"
	"testing"
	"time"
)

func TestObservationRejectsWrongSourceAndTime(t *testing.T) {
	now := time.Now().UTC()
	o := vxlan.Observation{Name: "configuration", Status: "complete", CompletedAt: &now}
	if validObservation(o, "runtime", now, now) {
		t.Fatal("dimension mismatch accepted")
	}
	future := now.Add(time.Hour)
	o.CompletedAt = &future
	if validObservation(o, "configuration", now, now) {
		t.Fatal("future evidence accepted")
	}
}
func TestRuntimeDimensionsDoNotCollectConfig(t *testing.T) {
	for _, d := range collectionDimensions("runtime") {
		if d == "configuration" {
			t.Fatal("unexpected config")
		}
	}
}
