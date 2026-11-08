package engine_test

import (
	"errors"
	"strings"
	"testing"

	"go-admin/app/firewall/engine"
	"go-admin/app/firewall/types"
)

func TestNormalizeClipboardRetainsPairedRowsAndOriginal(t *testing.T) {
	input := "源地址\t目的地址\t端口\t备注\n10.0.0.17 255.255.255.0\t192.0.2.8\t443\t申请一\n10.1.1.1-8\t198.51.100.1,198.51.100.2\tudp/53;tcp/1024-65535/443\t申请二"
	n, err := engine.Normalize(types.Input{Text: input})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Rows) != 2 {
		t.Fatalf("rows=%+v", n.Rows)
	}
	a, b := n.Rows[0], n.Rows[1]
	if a.Line != 2 || a.Source != "10.0.0.17 255.255.255.0" || a.Notes != "申请一" || a.Sources[0].Value != "10.0.0.0/24" || a.Services[0].Value != "tcp/443" {
		t.Fatalf("first row=%+v", a)
	}
	if b.Sources[0].Value != "10.1.1.1-10.1.1.8" || len(b.Destinations) != 2 || len(b.Services) != 2 || b.Services[1].Value != "tcp/1024-65535/443" {
		t.Fatalf("second row=%+v", b)
	}
	if len(a.Corrections) == 0 || len(b.Corrections) == 0 || a.Key == b.Key {
		t.Fatalf("provenance/corrections lost: %+v", n)
	}
}

func TestNormalizeInvalidFragmentsNeverBecomeWildcards(t *testing.T) {
	n, err := engine.Normalize(types.Input{Rows: []types.InputRow{{Key: "bad", Source: "10.0.0.1,300.1.1.1", Destination: "192.0.2.1", Service: ""}, {Source: "", Destination: "any", Service: "tcp/99999"}, {Source: "office-clients", Destination: "srv", Service: "WEB"}}})
	if err != nil {
		t.Fatal(err)
	}
	if n.Rows[0].Status != "partial" || len(n.Rows[0].Sources) != 1 || len(n.Rows[0].Services) != 0 || len(n.Rows[0].Issues) != 2 {
		t.Fatalf("mixed malformed input=%+v", n.Rows[0])
	}
	if n.Rows[1].Status != "invalid" || len(n.Rows[1].Sources) != 0 || len(n.Rows[1].Services) != 0 {
		t.Fatalf("blank promoted to wildcard=%+v", n.Rows[1])
	}
	if n.Rows[2].Sources[0].Kind != "name" || n.Rows[2].Services[0].Kind != "name" || n.Rows[2].Services[0].Value != "WEB" {
		t.Fatalf("native names lost=%+v", n.Rows[2])
	}
}

func TestNormalizeCSVColumnMappingAndBounds(t *testing.T) {
	n, err := engine.Normalize(types.Input{Text: "notes,dst,src,service\n\"test, quoted\",192.0.2.1,10.0.0.1,https", Columns: &types.Columns{Source: 2, Destination: 1, Service: 3, Notes: new(0)}})
	if err != nil || len(n.Rows) != 1 || n.Rows[0].Notes != "test, quoted" || n.Rows[0].Services[0].Value != "tcp/443" {
		t.Fatalf("csv=%+v err=%v", n, err)
	}
	if _, err = engine.Normalize(types.Input{Text: strings.Repeat("10.0.0.1\t192.0.2.1\t80\n", 1001)}); err == nil {
		t.Fatal("unbounded rows accepted")
	}
	if _, err = engine.Normalize(types.Input{Rows: []types.InputRow{{Source: "a", Destination: "b", Service: "80"}}, DefaultProtocol: "garbage"}); err == nil {
		t.Fatal("invalid default protocol accepted")
	}
}

func TestNormalizeWhitespaceThreeFieldRows(t *testing.T) {
	n, e := engine.Normalize(types.Input{Text: "10.0.0.1 192.0.2.1 443\n10.0.1.7 255.255.255.0 192.0.2.1 udp/53"})
	if e != nil || len(n.Rows) != 2 || n.Rows[0].Status != "ready" || n.Rows[1].Sources[0].Value != "10.0.1.0/24" {
		t.Fatalf("three field paste=%+v %v", n, e)
	}
}

func TestNormalizeIPv6RejectsIPv4DottedMask(t *testing.T) {
	n, e := engine.Normalize(types.Input{Rows: []types.InputRow{{Source: "2001:db8::1/255.255.255.0", Destination: "2001:db8:1::1", Service: "icmpv6/128/0"}}})
	if e != nil || len(n.Rows[0].Sources) != 0 || n.Rows[0].Status != "invalid" {
		t.Fatalf("IPv4 mask applied to IPv6=%+v %v", n, e)
	}
}

func TestNormalizeListSeparatorsNeverBecomeDottedMaskPairs(t *testing.T) {
	n, e := engine.Normalize(types.Input{Rows: []types.InputRow{{Source: "10.0.0.1,255.255.255.255;128.0.0.0", Destination: "192.0.2.1", Service: "tcp,80"}}})
	if e != nil || len(n.Rows[0].Sources) != 3 || n.Rows[0].Sources[1].Value != "255.255.255.255/32" || len(n.Rows[0].Services) != 2 {
		t.Fatalf("explicit lists merged across delimiter=%+v %v", n, e)
	}
}
func TestNormalizeNumericNativeNamesRemainResolvable(t *testing.T) {
	n, e := engine.Normalize(types.Input{Rows: []types.InputRow{{Source: "10.20_clients", Destination: "192.0.2.1", Service: "80-WEB"}}})
	if e != nil || len(n.Rows[0].Sources) != 1 || n.Rows[0].Sources[0].Kind != "name" || len(n.Rows[0].Services) != 1 || n.Rows[0].Services[0].Kind != "name" {
		t.Fatalf("native names starting with digits rejected=%+v %v", n, e)
	}
}

func TestNormalizeRejectsUnboundedFragmentExpansion(t *testing.T) {
	_, e := engine.Normalize(types.Input{Rows: []types.InputRow{{Source: strings.Repeat("10.0.0.1,", 4100), Destination: "192.0.2.1", Service: "443"}}})
	if !errors.Is(e, engine.ErrBudget) {
		t.Fatalf("unbounded fragment parsing=%v", e)
	}
}

func TestNormalizeWhitespaceBarePortsKeepDefaultProtocolAlternatives(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{{"80 443", []string{"tcp/80", "tcp/443"}}, {"53 123", []string{"tcp/53", "tcp/123"}}, {"6 443", []string{"tcp/6", "tcp/443"}}, {"tcp 443 udp 53", []string{"tcp/443", "udp/53"}}, {"53/123", []string{"53/123"}}, {"80/tcp", []string{"tcp/80"}}} {
		t.Run(tc.input, func(t *testing.T) {
			n, e := engine.Normalize(types.Input{Rows: []types.InputRow{row("10.0.0.2", "8.8.8.8", tc.input)}})
			if e != nil {
				t.Fatal(e)
			}
			if len(n.Rows[0].Services) != len(tc.want) {
				t.Fatalf("service association changed=%+v", n.Rows[0])
			}
			for i, w := range tc.want {
				if n.Rows[0].Services[i].Value != w {
					t.Fatalf("service %d=%q want %q", i, n.Rows[0].Services[i].Value, w)
				}
			}
		})
	}
}

func TestNormalizeDoesNotEraseNativeNamesThatResembleHeader(t *testing.T) {
	n, e := engine.Normalize(types.Input{Text: "source,destination,443\n192.0.2.10,198.51.100.10,80\nsource,destination,service"})
	if e != nil || len(n.Rows) != 3 || n.Rows[0].Source != "source" || n.Rows[0].Destination != "destination" || n.Rows[0].Services[0].Value != "tcp/443" || n.Rows[2].Service != "service" {
		t.Fatalf("header heuristic erased native intent=%+v %v", n, e)
	}
	n, e = engine.Normalize(types.Input{Text: "source,destination,service\n192.0.2.10,198.51.100.10,80"})
	if e != nil || len(n.Rows) != 1 || n.Rows[0].Line != 2 {
		t.Fatalf("recognized header retained as request=%+v %v", n, e)
	}
}
