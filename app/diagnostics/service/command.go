package service

import (
	"errors"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxDuration = 60 * time.Second
const serverDuration = 120 * time.Second

var (
	ErrInvalid      = errors.New("invalid diagnostic parameters")
	ErrUnavailable  = errors.New("diagnostic tool is not installed")
	ErrBusy         = errors.New("diagnostic capacity is busy")
	ErrNotFound     = errors.New("diagnostic job not found or expired")
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
)

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
	for _, label := range strings.Split(strings.TrimSuffix(target, "."), ".") {
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
		return command{}, ErrInvalid
	}
	if req.Mode != "client" && req.Mode != "server" {
		return command{}, ErrInvalid
	}
	if req.Mode == "server" && req.Tool != "iperf3" {
		return command{}, ErrInvalid
	}
	if req.Port < 0 || req.Port > 65535 || req.Duration < 0 || req.Duration > 30 {
		return command{}, ErrInvalid
	}
	if req.Mode != "server" {
		if !validTarget(req.Target) {
			return command{}, ErrInvalid
		}
		if addr, err := netip.ParseAddr(req.Target); err == nil {
			if req.Family == "4" && !addr.Is4() || req.Family == "6" && addr.Is4() {
				return command{}, ErrInvalid
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
		if req.Mode == "server" {
			if serverPort < 1024 || serverPort > 65535 {
				return command{}, ErrInvalid
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
	default:
		return command{}, ErrInvalid
	}
	return result, nil
}
