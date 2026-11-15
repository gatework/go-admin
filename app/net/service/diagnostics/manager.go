package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	maxOutput    = 256 << 10
	maxProcesses = 4
	maxJobs      = 128
	retention    = 15 * time.Minute
)

// Owner carries owner state for this module.
type Owner struct {
	Tenant string
	UserID int
}

// Job carries job state for this module.
type Job struct {
	ID         string     `json:"id"`
	Tool       string     `json:"tool"`
	Target     string     `json:"target"`
	Family     string     `json:"family"`
	Mode       string     `json:"mode"`
	Command    string     `json:"command"`
	Status     string     `json:"status"`
	Output     string     `json:"output"`
	Truncated  bool       `json:"truncated"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	Port       int        `json:"port"`
	Error      string     `json:"error,omitempty"`
}
type processJob struct {
	Job
	owner      Owner
	cancel     context.CancelFunc
	done       chan struct{}
	output     []byte
	finishedAt time.Time
}

// Manager carries manager state for this module.
type Manager struct {
	mu     sync.Mutex
	jobs   map[string]*processJob
	lookup func(string) (string, error)
	closed bool
	wg     sync.WaitGroup
}

// NewManager creates a manager owning diagnostic jobs and their processes.
func NewManager() *Manager {
	return &Manager{jobs: make(map[string]*processJob), lookup: exec.LookPath}
}

// Start launches a bounded diagnostic job owned by the caller.
func (m *Manager) Start(ctx context.Context, owner Owner, req Request) (Job, error) {
	if owner.UserID <= 0 || owner.Tenant == "" || ctx.Err() != nil {
		return Job{}, InvalidErr
	}
	spec, err := buildCommand(req, 5201)
	if err != nil {
		return Job{}, err
	}
	path, err := m.lookup(spec.name)
	if err != nil {
		return Job{}, UnavailableErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	if m.closed {
		return Job{}, UnavailableErr
	}
	used, err := m.availablePorts(owner, spec.request.Mode == "server")
	if err != nil {
		return Job{}, err
	}
	if spec.request.Mode == "server" {
		port := 5201
		for used[port] {
			port++
		}
		spec, err = buildCommand(req, port)
		if err != nil {
			return Job{}, err
		}
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	jobCtx, cancel := context.WithTimeout(context.Background(), spec.timeout)
	job := &processJob{ID: hex.EncodeToString(bytes), Tool: spec.name, Target: spec.request.Target, Family: spec.request.Family, Mode: spec.request.Mode, Port: spec.request.Port, Command: spec.name + " " + strings.Join(spec.args, " "), Status: "running", StartedAt: now, owner: owner, cancel: cancel, done: make(chan struct{})}
	if job.Mode == "server" {
		expires := now.Add(spec.timeout)
		job.ExpiresAt = &expires
	}
	cmd := exec.CommandContext(jobCtx, path, spec.args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TERM=dumb", "NO_COLOR=1")
	configureProcess(cmd)
	cmd.WaitDelay = time.Second
	output := &jobWriter{manager: m, job: job}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		cancel()
		return Job{}, fmt.Errorf("%w: %w", UnavailableErr, err)
	}
	m.jobs[job.ID] = job
	m.wg.Go(func() { m.wait(jobCtx, cancel, cmd, job) })
	return snapshot(job), nil
}

// Get describes the get used by this module.
func (m *Manager) Get(owner Owner, id string) (Job, error) {
	m.mu.Lock()
	// Get returns a caller-owned diagnostic job snapshot.
	defer m.mu.Unlock()
	m.prune()
	job, ok := m.jobs[id]
	if !ok || job.owner != owner {
		return Job{}, NotFoundErr
	}
	return snapshot(job), nil
}

// Stop describes the stop used by this module.
func (m *Manager) Stop(owner Owner, id string) (Job, error) {
	m.mu.Lock()
	// Stop cancels an authorized diagnostic job.
	job, ok := m.jobs[id]
	if !ok || job.owner != owner {
		m.mu.Unlock()
		return Job{}, NotFoundErr
	}
	if job.Status == "running" {
		job.cancel()
	}
	done := job.done
	m.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	return m.Get(owner, id)
}

// Close describes the close used by this module.
func (m *Manager) Close() {
	m.mu.Lock()
	// Close cancels and waits for all diagnostic processes.
	m.closed = true
	for _, job := range m.jobs {
		if job.Status == "running" {
			job.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func snapshot(job *processJob) Job {
	result := job.Job
	result.Output = strings.ToValidUTF8(string(job.output), "�")
	return result
}

// Server describes the server used by this module.
func (m *Manager) Server(connectHost string) Server {
	hostname, _ := os.Hostname()
	// Server describes the diagnostic tools available on this backend.
	name := os.Getenv("NETWORK_DIAGNOSTICS_NAME")
	if name == "" {
		name = hostname
	}
	publicHost := os.Getenv("NETWORK_DIAGNOSTICS_PUBLIC_HOST")
	if validTarget(publicHost) {
		connectHost = publicHost
	}
	if !validTarget(connectHost) {
		connectHost = hostname
	}
	result := Server{ID: "backend", Name: name, Hostname: hostname, ConnectHost: connectHost, Addresses: []string{}, Tools: []Tool{}, SessionSeconds: int(serverDuration.Seconds())}
	addresses, _ := net.InterfaceAddrs()
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			result.Addresses = append(result.Addresses, ip.String())
		}
	}
	for _, id := range []string{"ping", "mtr", "nexttrace", "iperf3", "traceroute"} {
		_, err := m.lookup(id)
		result.Tools = append(result.Tools, Tool{ID: id, Available: err == nil})
	}
	return result
}

type jobWriter struct {
	manager *Manager
	job     *processJob
}

func (w *jobWriter) Write(data []byte) (int, error) {
	w.manager.mu.Lock()
	defer w.manager.mu.Unlock()
	length := len(data)
	remaining := maxOutput - len(w.job.output)
	if length > remaining {
		data = data[:remaining]
		w.job.Truncated = true
	}
	w.job.output = append(w.job.output, data...)
	return length, nil
}

// Tool describes the tool used by this module.
type Tool struct {
	ID string `json:"id"`
	// Tool carries tool state for this module.
	Available bool `json:"available"`
}

// Server describes the server used by this module.
type Server struct {
	ID string `json:"id"`
	// Server carries server state for this module.
	Name           string   `json:"name"`
	Hostname       string   `json:"hostname"`
	ConnectHost    string   `json:"connectHost"`
	Addresses      []string `json:"addresses"`
	Tools          []Tool   `json:"tools"`
	SessionSeconds int      `json:"sessionSeconds"`
}

func (m *Manager) prune() {
	now := time.Now()
	for id, job := range m.jobs {
		if !job.finishedAt.IsZero() && now.Sub(job.finishedAt) > retention {
			delete(m.jobs, id)
		}
	}
	for len(m.jobs) >= maxJobs {
		var oldest *processJob
		for _, job := range m.jobs {
			if job.Status != "running" && (oldest == nil || job.finishedAt.Before(oldest.finishedAt)) {
				oldest = job
			}
		}
		if oldest == nil {
			break
		}
		delete(m.jobs, oldest.ID)
	}
}

func (m *Manager) wait(jobCtx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, job *processJob) {
	err := cmd.Wait()
	m.mu.Lock()
	job.finishedAt = time.Now().UTC()
	job.FinishedAt = &job.finishedAt
	switch {
	case jobCtx.Err() == context.Canceled:
		job.Status = "cancelled"
	case jobCtx.Err() == context.DeadlineExceeded:
		job.Status = "timed_out"
		if job.Mode == "server" {
			job.Status = "expired"
		}
	case err != nil:
		job.Status = "failed"
		job.Error = err.Error()
	default:
		job.Status = "succeeded"
	}
	cancel()
	close(job.done)
	m.mu.Unlock()
}

// availablePorts requires m.mu, keeping admission and process publication atomic.
func (m *Manager) availablePorts(owner Owner, server bool) (map[int]bool, error) {
	active := 0
	used := map[int]bool{}
	for _, job := range m.jobs {
		if job.Status != "running" {
			continue
		}
		active++
		if job.Mode == "server" {
			used[job.Port] = true
		}
		if job.owner == owner && (job.Mode == "server") == server {
			return nil, BusyErr
		}
	}
	if active >= maxProcesses {
		return nil, BusyErr
	}
	return used, nil
}
