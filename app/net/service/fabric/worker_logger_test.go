package fabric

import "testing"

// A worker's log line is the only trace of a failed tick. The host once built
// this worker without a logger and every failure disappeared.
func TestNewWorkerAlwaysHasALogger(t *testing.T) {
	s := &Fabric{}
	NewWorker(s, &fakeFabricTransport{})
	if s.Logger == nil {
		t.Fatal("worker constructed without a logger drops its tick failures")
	}
}
