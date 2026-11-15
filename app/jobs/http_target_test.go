package jobs

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHTTPTargetRefusesTheHostAndLinkLocal(t *testing.T) {
	for target, allowed := range map[string]bool{
		"https://monitor.example/api/ping":         true,
		"http://10.20.30.40/hook":                  true, // internal services are legitimate
		"http://127.0.0.1:8000/api/v1/sys-user":    false,
		"http://[::1]/":                            false,
		"http://169.254.169.254/latest/meta-data/": false,
		"http://0.0.0.0/":                          false,
		"file:///etc/passwd":                       false,
		"gopher://10.0.0.1/":                       false,
		"/relative/path":                           false,
	} {
		err := CheckHTTPTarget(target)
		if allowed != (err == nil) {
			t.Errorf("%s: allowed=%v err=%v", target, allowed, err)
		}
	}
}

// A host name that resolves to loopback is refused when it is dialled, which a
// check of the URL alone cannot see. httptest listens on loopback, so any
// request to it must fail.
func TestFetchRefusesLoopbackAtDialTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	target := "http://localhost:" + port
	if _, err := fetchHTTPJob(t.Context(), target); !errors.Is(err, BlockedTargetErr) {
		t.Fatalf("loopback reached by name: %v", err)
	}
}
