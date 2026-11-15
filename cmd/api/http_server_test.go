package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk/config"
)

// A client that trickles its headers held a connection for as long as the body
// timeout allowed (forever when that is zero), and idle keep-alive connections
// fell back to the body timeout too. Both now have fixed limits of their own.
func TestHTTPServerBoundsHeadersAndIdleConnections(t *testing.T) {
	srv := newHTTPServer(config.Application{Host: "127.0.0.1", Port: 8000, ReadTimeout: 60, WriterTimeout: 60}, http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadHeaderTimeout > srv.ReadTimeout {
		t.Fatalf("ReadHeaderTimeout = %v", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout unset")
	}
	if srv.ReadTimeout != time.Minute || srv.WriteTimeout != time.Minute || srv.Addr != "127.0.0.1:8000" {
		t.Fatalf("configured values not applied: %+v", srv)
	}
	unbounded := newHTTPServer(config.Application{}, http.NotFoundHandler())
	if unbounded.ReadHeaderTimeout <= 0 {
		t.Fatal("a zero body timeout must not leave headers unbounded")
	}
}
