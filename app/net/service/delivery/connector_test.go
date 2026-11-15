package delivery

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gatework/go-net-connector/connector"
)

func TestConnectorAdapterRetainsCommandReceiptWithoutApplicationLogging(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		defer server.Close()
		io.WriteString(server, "\r\n<fixture>")
		scanner := bufio.NewScanner(server)
		for scanner.Scan() {
			io.WriteString(server, scanner.Text()+"\r\nconfigured\r\n<fixture>")
		}
	}()
	profile, e := connectorProfile("h3c")
	if e != nil {
		t.Fatal(e)
	}
	session, e := newDeviceSession(profile, connector.ConnectionConfig{Host: "192.0.2.1", Username: "fixture-user", Password: "fixture-password"}, connector.WithTransportFactory(func(context.Context, connector.ConnectionConfig) (io.ReadWriteCloser, error) { return client, nil }))
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = session.Connect(ctx); e != nil {
		t.Fatal(e)
	}
	out := ""
	_, e = session.Execute(ctx, connector.Script{Commands: []connector.Command{{Text: "system-view"}}}, ExecutionHooks{Receipt: func(part string) error { out += part; return nil }})
	if e != nil || !strings.Contains(out, "configured") {
		t.Fatalf("receipt absent: %q %v", out, e)
	}
}

func TestConnectorPreservesH3CRuleScopeAndPersistsEachReceipt(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	commands := make(chan string, 8)
	go func() {
		defer server.Close()
		io.WriteString(server, "\r\n<fixture>")
		scan := bufio.NewScanner(server)
		for scan.Scan() {
			line := scan.Text()
			commands <- line
			body := "ok"
			if line == "dis this" {
				body = "rule 17 pass source-ip any\r\n#"
			}
			io.WriteString(server, line+"\r\n"+body+"\r\n<fixture>")
		}
	}()
	profile, _ := connectorProfile("h3c")
	session, e := newDeviceSession(profile, connector.ConnectionConfig{Host: "192.0.2.1", Username: "fixture", Password: "secret"}, connector.WithTransportFactory(func(context.Context, connector.ConnectionConfig) (io.ReadWriteCloser, error) { return client, nil }))
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = session.Connect(ctx); e != nil {
		t.Fatal(e)
	}
	script, _ := connector.ParseScript("rule pass source-ip any\nrule XXX append destination-ip any", "test")
	receipts, gates := 0, 0
	_, e = session.Execute(ctx, script, ExecutionHooks{Before: func(context.Context) error { gates++; return nil }, Receipt: func(string) error { receipts++; return nil }})
	if e != nil || receipts != 2 || gates < 4 {
		t.Fatalf("lost script scope %v receipts %d gates %d", e, receipts, gates)
	}
	for _, want := range []string{"rule pass source-ip any", "dis this", "rule 17 append destination-ip any"} {
		select {
		case got := <-commands:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		default:
			t.Fatal("missing expected send")
		}
	}
}
func TestConnectorChecksCancellationBeforeNestedHookCommand(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	commands := make(chan string, 8)
	go func() {
		defer server.Close()
		io.WriteString(server, "\r\n<fixture>")
		scan := bufio.NewScanner(server)
		for scan.Scan() {
			line := scan.Text()
			commands <- line
			io.WriteString(server, line+"\r\nok\r\n<fixture>")
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	profile, _ := connectorProfile("h3c")
	profile.AfterCommand = func(c context.Context, _ connector.Command, _ connector.StepResult, x connector.Executor) error {
		cancel()
		_, e := x.Execute(c, connector.Command{Text: "must-not-send"})
		return e
	}
	session, e := newDeviceSession(profile, connector.ConnectionConfig{Host: "192.0.2.1", Username: "fixture", Password: "secret"}, connector.WithTransportFactory(func(context.Context, connector.ConnectionConfig) (io.ReadWriteCloser, error) { return client, nil }))
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	if e = session.Connect(ctx); e != nil {
		t.Fatal(e)
	}
	receipts := 0
	_, e = session.Execute(ctx, connector.Script{Commands: []connector.Command{{Text: "system-view"}}}, ExecutionHooks{Before: func(c context.Context) error { return c.Err() }, Receipt: func(string) error { receipts++; return nil }})
	if e == nil || receipts != 1 {
		t.Fatalf("nested cancel lost receipt %v %d", e, receipts)
	}
	if got := <-commands; got != "system-view" {
		t.Fatal(got)
	}
	select {
	case got := <-commands:
		t.Fatal("sent after cancellation", got)
	default:
	}
}
