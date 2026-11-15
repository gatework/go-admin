package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
)

const (
	leaseTTL       = 30 * time.Second
	leaseHeartbeat = 10 * time.Second
)

var supervisors sync.Map // tenant key -> *supervisor

// supervisor owns the scheduler, its lease deadline and reconciliation. The
// expiry callback never waits for database I/O. Stopping cron prevents future
// dispatch; already running jobs still need their own idempotency contract.
type supervisor struct {
	key             string
	db              *gorm.DB
	lease           *lease
	cron            *cron.Cron
	control         *cronControl
	tickMu          sync.Mutex
	mu              sync.Mutex
	running         bool
	closed          bool
	deadline        time.Time
	timer           *time.Timer
	stop            chan struct{}
	stopOnce        sync.Once
	done            chan struct{}
	cancel          context.CancelFunc
	execution       context.Context //nolint:containedctx // Scheduler-owned lifecycle context; request contexts are never retained.
	executionCancel context.CancelFunc
}

func newSupervisor(key string, db *gorm.DB, owner string) *supervisor {
	c := sdk.Runtime.Crontabs()[key]
	execution, cancel := context.WithCancel(context.Background())
	return &supervisor{key: key, db: db, lease: &lease{db: db, owner: owner, ttl: leaseTTL},
		execution: execution, executionCancel: cancel, cron: c, control: &cronControl{cron: c}, stop: make(chan struct{}), done: make(chan struct{})}
}

func (s *supervisor) start() {
	supervisors.Store(s.key, s)
	s.tick()
	go s.heartbeat()
	sdk.Runtime.SetShutdown(s.shutdown)
}

func (s *supervisor) heartbeat() {
	defer close(s.done)
	t := time.NewTicker(leaseHeartbeat)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.tick()
		}
	}
}

func (s *supervisor) tick() {
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	// The DB clock is sampled after this instant. Starting the local deadline
	// here (with a millisecond rounding margin) cannot extend it by reply latency.
	deadline := time.Now().Add(s.lease.ttl - 2*time.Millisecond)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	ioDeadline := deadline
	if s.running && s.deadline.Before(ioDeadline) {
		ioDeadline = s.deadline
	}
	ctx, cancel := context.WithDeadline(context.Background(), ioDeadline)
	s.cancel = cancel
	s.mu.Unlock()
	defer cancel()
	held, err := s.lease.acquireContext(ctx)
	s.mu.Lock()
	s.cancel = nil
	if s.closed {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.mu.Unlock()
		log.NewHelper(log.Default()).Errorf("[Job] scheduler lease for %s: %v", s.key, err)
		return
	}
	if !held || ctx.Err() != nil || !time.Now().Before(deadline) {
		s.stopLocked()
		s.mu.Unlock()
		return
	}
	s.deadline = deadline
	s.running = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(time.Until(deadline), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.deadline.Equal(deadline) {
			s.stopLocked()
		}
	})
	s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		log.NewHelper(log.Default()).Errorf("[Job] reconciling %s: %v", s.key, err)
	}
}

// stopLocked does not wait for active jobs or for the renewal goroutine.
func (s *supervisor) stopLocked() {
	wasRunning := s.running
	s.running = false
	if s.timer != nil {
		s.timer.Stop()
	}
	if wasRunning && s.cron != nil {
		s.control.requestStop()
	}
}

func (s *supervisor) stopScheduling() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

// leaseReleaseTimeout bounds the lease release at shutdown. It is one UPDATE
// and must finish well inside a container's stop grace period.
const leaseReleaseTimeout = 3 * time.Second

func (s *supervisor) shutdown(ctx context.Context) {
	s.stopOnce.Do(func() { close(s.stop); s.executionCancel() })
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.stopLocked()
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-ctx.Done():
	}
	s.control.waitStopped(ctx)
	// The release must not share the caller's budget: once the waits above have
	// spent it, a release on the same context fails at once and the successor
	// waits out the whole TTL. It gets its own short, independent deadline.
	release, cancel := context.WithTimeout(context.WithoutCancel(ctx), leaseReleaseTimeout)
	defer cancel()
	if err := s.lease.releaseContext(release); err != nil {
		log.NewHelper(log.Default()).Errorf("[Job] releasing lease for %s: %v", s.key, err)
	}
	supervisors.CompareAndDelete(s.key, s)
}

func (s *supervisor) holdsLease() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running && time.Now().Before(s.deadline)
}

// Reconcile applies committed desired state on the local leader. A standby
// deliberately does nothing: the leader also reconciles every heartbeat.
func Reconcile(ctx context.Context, key string) error {
	if value, ok := supervisors.Load("*"); ok {
		s, ok := value.(*supervisor)
		if !ok {
			return fmt.Errorf("jobs: invalid supervisor registry entry")
		}
		return s.reconcile(ctx)
	}
	if value, ok := supervisors.Load(key); ok {
		s, ok := value.(*supervisor)
		if !ok {
			return fmt.Errorf("jobs: invalid supervisor registry entry")
		}
		return s.reconcile(ctx)
	}
	return nil
}

func (s *supervisor) reconcile(ctx context.Context) error {
	s.mu.Lock()
	if !s.running || s.closed {
		s.mu.Unlock()
		return nil
	}
	deadline := s.deadline
	s.mu.Unlock()
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	ctx, cancelWait := context.WithTimeout(ctx, time.Second)
	defer cancelWait()
	return s.control.run(ctx, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		list, err := loadJobs(ctx, s.db)
		if err != nil {
			return err
		}
		stored := storedEntryIDs(list)
		if !s.holdsLease() {
			return nil
		}
		if err := applyJobs(ctx, s.cron, list, jobExecution{ctx: s.execution, allowed: s.holdsLease}); err != nil {
			return err
		}
		if ctx.Err() != nil || !s.holdsLease() {
			return ctx.Err()
		}
		s.control.start()
		return saveEntryIDs(ctx, s.db, list, stored)
	})
}
