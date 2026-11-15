package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// BlockedTargetErr reports an HTTP job target the scheduler will not request.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var BlockedTargetErr = errors.New("jobs: http job target is not allowed")

//revive:enable:error-naming

const (
	httpJobTimeout   = 30 * time.Second
	httpJobBodyLimit = 1 << 20
	httpJobRedirects = 5
)

// CheckHTTPTarget validates an HTTP job's URL when it is saved: an absolute
// http or https URL with a host, and not a literal address the scheduler
// refuses to reach. Host names are checked again when they are resolved.
func CheckHTTPTarget(raw string) error {
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" {
		return fmt.Errorf("%w: need an absolute http or https URL", BlockedTargetErr)
	}
	if address, err := netip.ParseAddr(target.Hostname()); err == nil && blockedAddress(address) {
		return fmt.Errorf("%w: %s", BlockedTargetErr, address)
	}
	return nil
}

// blockedAddress covers what a job has no business requesting from the
// server's own position: the host itself (loopback, unspecified) and
// link-local addresses, which include cloud metadata endpoints. Private
// ranges stay reachable: internal services are what an operations platform's
// jobs call.
func blockedAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsLoopback() || address.IsUnspecified() || address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() || address.IsInterfaceLocalMulticast() || address.IsMulticast()
}

// refuseBlocked runs after name resolution, on the address actually dialled,
// so a host name that resolves to a blocked address is refused as well.
func refuseBlocked(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if blockedAddress(ip) {
		return fmt.Errorf("%w: %s", BlockedTargetErr, ip)
	}
	return nil
}

// httpJobClient connects directly (no environment proxy, which would make the
// address check meaningless) and re-checks every redirect.
var httpJobClient = &http.Client{
	Timeout: httpJobTimeout,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, Control: refuseBlocked}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: httpJobTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       time.Minute,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= httpJobRedirects {
			return errors.New("jobs: too many redirects")
		}
		return CheckHTTPTarget(req.URL.String())
	},
}

// fetchHTTPJob requests target once. An HTTP error status is a failure, so the
// job is retried and logged as failed rather than recorded as completed.
func fetchHTTPJob(ctx context.Context, target string) (string, error) {
	if err := CheckHTTPTarget(target); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	response, err := httpJobClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, httpJobBodyLimit))
	if err != nil {
		return "", err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("jobs: %s answered %s", request.URL.Redacted(), response.Status)
	}
	return string(body), nil
}
