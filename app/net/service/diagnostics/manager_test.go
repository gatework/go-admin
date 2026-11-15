package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testManager(t *testing.T, script string) *Manager {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	m.lookup = func(string) (string, error) { return binary, nil }
	t.Cleanup(m.Close)
	return m
}
func awaitJob(t *testing.T, m *Manager, owner Owner, id string) Job {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		job, err := m.Get(owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != "running" {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Job{}
}
func TestJobOutputOwnershipAndExitStatus(t *testing.T) {
	m := testManager(t, "printf 'first\\n'; printf 'error\\n' >&2; exit 7\n")
	owner := Owner{Tenant: "a", UserID: 1}
	job, err := m.Start(context.Background(), owner, Request{Tool: "ping", Target: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []Owner{{Tenant: "a", UserID: 2}, {Tenant: "b", UserID: 1}} {
		if _, err := m.Get(other, job.ID); !errors.Is(err, NotFoundErr) {
			t.Fatal("read crossed owner boundary")
		}
		if _, err := m.Stop(other, job.ID); !errors.Is(err, NotFoundErr) {
			t.Fatal("stop crossed owner boundary")
		}
	}
	done := awaitJob(t, m, owner, job.ID)
	if done.Status != "failed" || !strings.Contains(done.Output, "first") || !strings.Contains(done.Output, "error") || done.FinishedAt == nil {
		t.Fatalf("unexpected result: %+v", done)
	}
}
func TestStopReapsProcessAndReleasesCapacity(t *testing.T) {
	m := testManager(t, "printf 'started\\n'; sleep 30\n")
	owner := Owner{Tenant: "a", UserID: 1}
	job, err := m.Start(context.Background(), owner, Request{Tool: "ping", Target: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), owner, Request{Tool: "mtr", Target: "localhost"}); !errors.Is(err, BusyErr) {
		t.Fatal("duplicate job accepted")
	}
	if _, err := m.Stop(owner, job.ID); err != nil {
		t.Fatal(err)
	}
	done := awaitJob(t, m, owner, job.ID)
	if done.Status != "cancelled" {
		t.Fatalf("status: %s", done.Status)
	}
	if _, err := m.Start(context.Background(), owner, Request{Tool: "ping", Target: "localhost"}); err != nil {
		t.Fatal(err)
	}
}
func TestOutputAndRetentionAreBounded(t *testing.T) {
	m := testManager(t, "head -c 400000 /dev/zero | tr '\\000' x\n")
	owner := Owner{Tenant: "a", UserID: 1}
	job, err := m.Start(context.Background(), owner, Request{Tool: "ping", Target: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	done := awaitJob(t, m, owner, job.ID)
	if !done.Truncated || len(done.Output) > maxOutput {
		t.Fatalf("output not bounded: %d", len(done.Output))
	}
	m.mu.Lock()
	m.jobs[job.ID].finishedAt = time.Now().Add(-retention - time.Second)
	m.mu.Unlock()
	if _, err := m.Get(owner, job.ID); !errors.Is(err, NotFoundErr) {
		t.Fatal("expired job retained")
	}
}
func TestUnavailableToolAndServerPortAllocation(t *testing.T) {
	m := testManager(t, "sleep 30\n")
	owner := Owner{Tenant: "a", UserID: 1}
	first, err := m.Start(context.Background(), owner, Request{Tool: "iperf3", Mode: "server"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Start(context.Background(), Owner{Tenant: "b", UserID: 1}, Request{Tool: "iperf3", Mode: "server"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Port == second.Port || first.ExpiresAt == nil {
		t.Fatal("server ports/expiry incorrect")
	}
	m.lookup = func(string) (string, error) { return "", os.ErrNotExist }
	if _, err := m.Start(context.Background(), owner, Request{Tool: "ping", Target: "localhost"}); !errors.Is(err, UnavailableErr) {
		t.Fatal(err)
	}
}
