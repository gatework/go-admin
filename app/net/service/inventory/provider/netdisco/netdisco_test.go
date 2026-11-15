package netdisco

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchAuthenticatesAndWhitelistsFacts(t *testing.T) {
	var logins, searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			logins.Add(1)
			username, password, ok := r.BasicAuth()
			if r.Method != http.MethodPost || !ok || username != "reader" || password != "fixture-password" {
				t.Errorf("wrong login request")
			}
			w.Write([]byte(`{"api_key":"opaque-token"}`))
			return
		}
		searches.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/device" || r.URL.Query().Get("q") != "%" || r.URL.Query().Get("seeallcolumns") != "true" || r.Header.Get("Authorization") != "opaque-token" {
			t.Errorf("wrong search request: %s", r.URL.Path)
		}
		w.Write([]byte(`[{"ip":"::ffff:192.0.2.1","name":" core ","dns":"core.example","serial":null,"vendor":"Vendor","model":"M1","os":"OS","os_ver":"1","location":"Room","contact":"Ops","description":"Device","mac":"00:11:22:33:44:55","tags":["edge"],"creation":"2026-10-01 12:30:00+08","last_discover":"not a date","snmp_comm":"private-community","snmp_authpass":"fixture-secret","log":"secret-log"}]`))
	}))
	defer server.Close()
	p := New(Config{URL: server.URL, Username: "reader", Password: "fixture-password"})
	rows, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].IP != "192.0.2.1" || rows[0].Name != "core" || rows[0].Hostname != "core.example" || rows[0].Serial != "" || rows[0].LastDiscoveredAt != "" || rows[0].SourceCreatedAt != "2026-10-01T04:30:00Z" {
		t.Fatalf("unexpected facts: %+v", rows)
	}
	b, _ := json.Marshal(rows)
	for _, secret := range []string{"snmp", "private-community", "fixture-secret", "secret-log"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("sensitive field leaked: %s", secret)
		}
	}
	if logins.Load() != 1 || searches.Load() != 1 {
		t.Fatal("unexpected automatic retries")
	}
}

func TestFetchRejectsMalformedInventoryAtomically(t *testing.T) {
	for _, payload := range []string{`null`, `{}`, `[{"ip":"192.0.2.1"},null]`, `[{"ip":"bad"}]`, `[{"ip":"192.0.2.1"},{"ip":"::ffff:192.0.2.1"}]`, `[{"ip":"192.0.2.1"}] {}`, `[{"ip":"192.0.2.1","tags":"wrong"}]`} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(payload)) }))
			defer server.Close()
			rows, err := New(Config{URL: server.URL, APIKey: "token"}).Fetch(context.Background())
			if err == nil || rows != nil {
				t.Fatalf("malformed inventory accepted: %+v %v", rows, err)
			}
		})
	}
}

func TestFetchDoesNotFollowRedirectOrLeakRawErrors(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer target.Close()
	for _, status := range []int{http.StatusFound, http.StatusUnauthorized, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			w.Write([]byte("password=fixture-secret"))
		}))
		_, err := New(Config{URL: server.URL, APIKey: "fixture-secret"}).Fetch(context.Background())
		server.Close()
		if err == nil || strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	if reached.Load() != 0 {
		t.Fatal("credentials followed a redirect")
	}
}

func TestFetchHonorsCancellationTimeoutAndResponseLimit(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	p := New(Config{URL: slow.URL, APIKey: "token"})
	p.timeout = 20 * time.Millisecond
	started := time.Now()
	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout was not bounded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Fetch(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"ip":"192.0.2.1","description":"` + strings.Repeat("x", 100) + `"}]`))
	}))
	defer large.Close()
	p = New(Config{URL: large.URL, APIKey: "token"})
	p.maxBytes = 64
	if rows, err := p.Fetch(context.Background()); err == nil || rows != nil {
		t.Fatal("oversized response accepted")
	}
}

func TestConfigurationRejectsCredentialsInURL(t *testing.T) {
	for _, endpoint := range []string{"", "file:///etc/passwd", "https://user:pass@example.com", "https://example.com?token=secret", "https://example.com/#secret", "https://example.com?", "https://example.com/#"} {
		p := New(Config{URL: endpoint, APIKey: "token"})
		if p.Configured() {
			t.Fatalf("unsafe configuration accepted")
		}
		if _, err := p.Fetch(context.Background()); err == nil || strings.Contains(err.Error(), endpoint) && endpoint != "" {
			t.Fatalf("configuration error must be safe: %v", err)
		}
	}
}

func TestSourceDatesPreserveUnknownTimezone(t *testing.T) {
	rows, err := decodeInventory([]byte(`[{"ip":"192.0.2.1","creation":"2026-03-16 15:25:29.651235","last_discover":"2026-10-26 07:41:18.801535"}]`))
	if err != nil || len(rows) != 1 || rows[0].SourceCreatedAt != "2026-03-16T15:25:29.651235" || rows[0].LastDiscoveredAt != "2026-10-26T07:41:18.801535" {
		t.Fatalf("naive source time was discarded or assigned a timezone: %+v %v", rows, err)
	}
}

func TestAmbiguousTimezoneIsNotGuessed(t *testing.T) {
	if got := sourceTime("2026-10-26 07:41:18 CST"); got != "" {
		t.Fatalf("ambiguous timezone assigned a UTC instant: %q", got)
	}
}
