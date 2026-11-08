package service

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

type Owner struct {
	Tenant string
	UserID int
}
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
type Manager struct {
	mu     sync.Mutex
	jobs   map[string]*processJob
	lookup func(string) (string, error)
	closed bool
	wg     sync.WaitGroup
}

func NewManager() *Manager {
	return &Manager{jobs: make(map[string]*processJob), lookup: exec.LookPath}
}

func (m *Manager) Start(ctx context.Context, owner Owner, req Request) (Job, error) {
	if owner.UserID <= 0 || owner.Tenant == "" || ctx.Err() != nil {
		return Job{}, ErrInvalid
	}
	spec, err := buildCommand(req, 5201)
	if err != nil {
		return Job{}, err
	}
	path, err := m.lookup(spec.name)
	if err != nil {
		return Job{}, ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	if m.closed {
		return Job{}, ErrUnavailable
	}
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
		if job.owner == owner && (job.Mode == "server") == (spec.request.Mode == "server") {
			return Job{}, ErrBusy
		}
	}
	if active >= maxProcesses {
		return Job{}, ErrBusy
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
	job := &processJob{Job: Job{ID: hex.EncodeToString(bytes), Tool: spec.name, Target: spec.request.Target, Family: spec.request.Family, Mode: spec.request.Mode, Port: spec.request.Port, Command: spec.name + " " + strings.Join(spec.args, " "), Status: "running", StartedAt: now}, owner: owner, cancel: cancel, done: make(chan struct{})}
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
		return Job{}, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	m.jobs[job.ID] = job
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
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
	}()
	return snapshot(job), nil
}

func (m *Manager) Get(owner Owner, id string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	job, ok := m.jobs[id]
	if !ok || job.owner != owner {
		return Job{}, ErrNotFound
	}
	return snapshot(job), nil
}
func (m *Manager) Stop(owner Owner, id string) (Job, error) {
	m.mu.Lock()
	job, ok := m.jobs[id]
	if !ok || job.owner != owner {
		m.mu.Unlock()
		return Job{}, ErrNotFound
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
func (m *Manager) Close() {
	m.mu.Lock()
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

type Tool struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}
type Server struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Hostname       string   `json:"hostname"`
	ConnectHost    string   `json:"connectHost"`
	Addresses      []string `json:"addresses"`
	Tools          []Tool   `json:"tools"`
	SessionSeconds int      `json:"sessionSeconds"`
}

func (m *Manager) Server(connectHost string) Server {
	hostname, _ := os.Hostname()
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
