package diagnostics

import (
	"strings"
	"testing"
)

func TestCommandRejectsUnsafeAndInconsistentInputs(t *testing.T) {
	cases := []Request{
		{Tool: "sh", Target: "127.0.0.1"}, {Tool: "ping", Target: "--help"},
		{Tool: "ping", Target: "example.com;id"}, {Tool: "ping", Target: "$(id)"},
		{Tool: "ping", Target: "https://example.com"}, {Tool: "ping", Target: "a b"},
		{Tool: "ping", Target: "::1", Family: "4"}, {Tool: "ping", Target: "127.0.0.1", Family: "6"},
		{Tool: "ping", Target: ""}, {Tool: "ping", Target: "0.0.0.0"},
		{Tool: "ping", Target: "224.0.0.1"}, {Tool: "ping", Target: "fe80::1%eth0"},
		{Tool: "iperf3", Target: "localhost", Port: 65536}, {Tool: "iperf3", Target: "localhost", Duration: 31},
		{Tool: "ping", Target: "localhost", Mode: "server"}, {Tool: "iperf3", Target: "localhost", Mode: "unknown"},
	}
	for _, req := range cases {
		if _, err := buildCommand(req, 5201); err == nil {
			t.Errorf("accepted unsafe request: %+v", req)
		}
	}
}

func TestCommandsHaveFixedBoundsAndBothIperfModes(t *testing.T) {
	for _, tool := range []string{"ping", "mtr", "nexttrace", "traceroute", "iperf3"} {
		cmd, err := buildCommand(Request{Tool: tool, Target: "example.com", Family: "6"}, 5201)
		if err != nil {
			t.Fatal(err)
		}
		if cmd.name != tool || cmd.timeout <= 0 || cmd.timeout > maxDuration {
			t.Fatalf("unbounded command: %+v", cmd)
		}
		if !strings.Contains(strings.Join(cmd.args, " "), "example.com") {
			t.Fatalf("missing target: %+v", cmd)
		}
	}
	client, err := buildCommand(Request{Tool: "iperf3", Target: "127.0.0.1", Reverse: true}, 5201)
	if err != nil || !strings.Contains(strings.Join(client.args, " "), "-R") {
		t.Fatalf("reverse: %+v %v", client, err)
	}
	server, err := buildCommand(Request{Tool: "iperf3", Mode: "server"}, 5202)
	if err != nil || server.request.Port != 5202 || !strings.Contains(strings.Join(server.args, " "), "-s") {
		t.Fatalf("server: %+v %v", server, err)
	}
}
