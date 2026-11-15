package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"errors"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
)

// The HTTP listener: its limits and how it starts serving.

// Header and idle limits are not configurable: they protect the listener from
// clients that hold connections open, and no legitimate client needs more.
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
)

// newHTTPServer applies the configured body timeouts plus fixed header and
// idle limits. Without ReadHeaderTimeout a client that sends headers slowly
// holds a connection for as long as ReadTimeout allows, or forever when it is
// zero; without IdleTimeout keep-alive connections fall back to ReadTimeout.
func newHTTPServer(app config.Application, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", app.Host, app.Port),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       time.Duration(app.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(app.WriterTimeout) * time.Second,
		IdleTimeout:       idleTimeout,
	}
}

// startServing binds srv.Addr, hands the listener to srv on its own goroutine,
// and announces AfterListen.
//
// The bind is done here rather than left to ListenAndServe, which binds on the
// goroutine that serves. That put the failure every deployment actually hits -
// "address already in use" - on a goroutine nobody was reading, so the banner
// went on to claim the server was up, and there would be no way to keep
// AfterListen from announcing a socket that does not exist. A hook there is
// promised a reachable port; the only way to keep that promise is for the bind
// to have already happened on this goroutine.
//
// AfterListen is announced synchronously. Running it in a goroutine to save the
// few milliseconds would let it overlap the shutdown: on a fast SIGTERM the
// cleanup callbacks could finish before the startup ones had.
//
// Both ways of failing to start are therefore checked before the announcement:
// the bind, and - with ssl enabled - the certificate.
func startServing(srv *http.Server, useTLS bool, pem, key string) error {
	if useTLS {
		// Read the certificate before anything is announced. ServeTLS reads
		// these files itself, but on the serving goroutine - so a bad
		// certificate used to surface after AfterListen had already promised a
		// reachable port. Loading it here costs one extra read and moves the
		// failure onto this goroutine, where run() can return it.
		//
		// ServeTLS still does the real work below rather than this handing it a
		// tls.Listener: that is what sets up HTTP/2 negotiation, and taking it
		// over here would quietly drop h2 for every TLS deployment.
		if _, err := tls.LoadX509KeyPair(pem, key); err != nil {
			return fmt.Errorf("tls certificate: %w", err)
		}
	}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		// 服务连接
		var err error
		if useTLS {
			err = srv.ServeTLS(ln, pem, key)
		} else {
			err = srv.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Still fatal, as it was. Neither the bind nor the certificate is
			// among the errors that reach here any more - both are checked
			// above, on the caller's goroutine. What is left is a serve that
			// failed after the port was taken, and carrying on would park the
			// process on <-quit with nothing serving.
			log.NewHelper(log.Default()).Fatal("serve: ", err)
		}
	}()

	sdk.Runtime.RunPhase(runtime.AfterListen)
	return nil
}
