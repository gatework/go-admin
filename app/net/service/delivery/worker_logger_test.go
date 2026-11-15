package delivery

import "testing"

// A worker's log line is the only trace of a failed tick. The host once built
// this worker without a logger and every failure disappeared.
func TestNewWorkerAlwaysHasALogger(t *testing.T) {
	s := &Workflow{}
	NewWorker(s, nil)
	if s.Logger == nil {
		t.Fatal("worker constructed without a logger drops its tick failures")
	}
}
