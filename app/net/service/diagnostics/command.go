package diagnostics

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	errs "github.com/gatework/motor/common/errors"
)

const maxDuration = 60 * time.Second
const serverDuration = 120 * time.Second

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is a target or parameter the service will not run.
	InvalidErr = errs.New(errs.KindInvalid, "diagnostics.invalid_input", "")
	// UnavailableErr is a tool that is not installed on this server.
	UnavailableErr = errs.New(errs.KindUnavailable, "diagnostics.tool_unavailable", "")
	// BusyErr is a caller already running a diagnostic, or a server at capacity.
	BusyErr = errs.New(errs.KindUnavailable, "diagnostics.busy", "")
	// NotFoundErr is a job that does not exist, has expired, or is someone else's.
	NotFoundErr     = errs.New(errs.KindNotFound, "diagnostics.not_found", "")
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
)

//revive:enable:error-naming

// Request carries request state for this module.
type Request struct {
	Tool     string `json:"tool"`
	Target   string `json:"target"`
	Family   string `json:"family"`
	Mode     string `json:"mode"`
	Port     int    `json:"port"`
	Duration int    `json:"duration"`
	Reverse  bool   `json:"reverse"`
}

type command struct {
	name    string
	args    []string
	request Request
	timeout time.Duration
}

// validTarget accepts a single host, never an option, URL, port or shell fragment.
// Private and loopback addresses are intentional: this is an authenticated ops tool.
func validTarget(target string) bool {
	if addr, err := netip.ParseAddr(target); err == nil {
		return addr.Zone() == "" && !addr.IsUnspecified() && !addr.IsMulticast() && !addr.IsLinkLocalUnicast()
	}
	if len(target) == 0 || len(target) > 253 {
		return false
	}
	for label := range strings.SplitSeq(strings.TrimSuffix(target, "."), ".") {
		if !hostnamePattern.MatchString(label) {
			return false
		}
	}
	return true
}

func buildCommand(req Request, serverPort int) (command, error) {
	req.Target = strings.TrimSpace(req.Target)
	if req.Family == "" {
		req.Family = "auto"
	}
	if req.Mode == "" {
		req.Mode = "client"
	}
	if req.Family != "auto" && req.Family != "4" && req.Family != "6" {
		return command{}, InvalidErr
	}
	if req.Mode != "client" && req.Mode != "server" {
		return command{}, InvalidErr
	}
	if req.Mode == "server" && req.Tool != "iperf3" {
		return command{}, InvalidErr
	}
	if req.Port < 0 || req.Port > 65535 || req.Duration < 0 || req.Duration > 30 {
		return command{}, InvalidErr
	}
	if req.Mode != "server" {
		if !validTarget(req.Target) {
			return command{}, InvalidErr
		}
		if addr, err := netip.ParseAddr(req.Target); err == nil {
			if req.Family == "4" && !addr.Is4() || req.Family == "6" && addr.Is4() {
				return command{}, InvalidErr
			}
		}
	}
	result := command{name: req.Tool, request: req, timeout: maxDuration}
	family := []string{}
	if req.Family != "auto" {
		family = append(family, "-"+req.Family)
	}
	switch req.Tool {
	case "ping":
		result.args = append(family, "-n", "-c", "5", "-W", "2", req.Target)
	case "mtr":
		result.args = append(family, "--report", "--report-wide", "--no-dns", "--report-cycles", "10", "--interval", "0.5", "--max-ttl", "20", req.Target)
	case "traceroute":
		result.args = append(family, "-n", "-m", "20", "-q", "1", "-w", "1", req.Target)
	case "nexttrace":
		result.args = append(family, "--no-color", "--no-rdns", "--max-hops", "20", "--queries", "1", "--timeout", "1000", "--data-provider", "disable-geoip", req.Target)
	case "iperf3":
		return configureIperf(result, req, family, serverPort)
	default:
		return command{}, InvalidErr
	}
	return result, nil
}

func configureIperf(result command, req Request, family []string, serverPort int) (command, error) {
	if req.Mode == "server" {
		if serverPort < 1024 || serverPort > 65535 {
			return command{}, InvalidErr
		}
		result.request.Target = ""
		result.request.Port = serverPort
		result.timeout = serverDuration
		result.args = append(family, "-s", "-p", strconv.Itoa(serverPort), "--forceflush")
	} else {
		if req.Port == 0 {
			result.request.Port = 5201
		}
		if req.Duration == 0 {
			result.request.Duration = 10
		}
		result.args = append(family, "-c", req.Target, "-p", strconv.Itoa(result.request.Port), "-t", strconv.Itoa(result.request.Duration), "-i", "1", "--connect-timeout", "5000", "--forceflush")
		if req.Reverse {
			result.args = append(result.args, "-R")
		}
	}
	return result, nil
}
