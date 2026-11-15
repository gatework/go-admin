package errkind

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestOfFindsTheClassificationThroughWrapping(t *testing.T) {
	conflict := New(Conflict, "firewall.version_conflict")
	wrapped := fmt.Errorf("saving draft 7: %w", conflict)
	kind, code := Of(errors.Join(errors.New("other"), wrapped))
	if kind != Conflict || code != "firewall.version_conflict" {
		t.Fatalf("Of = %v %q", kind, code)
	}
	if !errors.Is(wrapped, conflict) {
		t.Fatal("wrapping lost the sentinel identity")
	}
	if errors.Is(wrapped, New(Conflict, "firewall.version_conflict")) {
		t.Fatal("distinct sentinels with the same code compared equal")
	}
	if kind, code := Of(errors.New("driver: connection refused")); kind != Unknown || code != "" {
		t.Fatalf("unclassified error classified as %v %q", kind, code)
	}
}

func TestEveryKindHasAStatus(t *testing.T) {
	want := map[Kind]int{
		Unknown: http.StatusInternalServerError, Invalid: http.StatusBadRequest,
		Unauthorized: http.StatusUnauthorized, Forbidden: http.StatusForbidden,
		NotFound: http.StatusNotFound, Conflict: http.StatusConflict,
		Busy: http.StatusTooManyRequests, Unavailable: http.StatusServiceUnavailable,
		Upstream: http.StatusBadGateway, Timeout: http.StatusGatewayTimeout,
	}
	for kind := range Timeout + 1 {
		if got := kind.Status(); got != want[kind] {
			t.Errorf("%d.Status() = %d, want %d", kind, got, want[kind])
		}
	}
}
