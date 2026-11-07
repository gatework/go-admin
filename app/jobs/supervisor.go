package jobs

import (
	"context"
	"sync"
	"time"

	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
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
	key      string
	db       *gorm.DB
	lease    *lease
	cron     *cron.Cron
	control  *cronControl
	tickMu   sync.Mutex
	mu       sync.Mutex
	running  bool
	closed   bool
	deadline time.Time
	timer    *time.Timer
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
	cancel   context.CancelFunc
}

func newSupervisor(key string, db *gorm.DB, owner string) *supervisor {
	c := sdk.Runtime.GetAllCrontab()[key]
	return &supervisor{key: key, db: db, lease: &lease{db: db, owner: owner, ttl: leaseTTL},
		cron: c, control: &cronControl{cron: c}, stop: make(chan struct{}), done: make(chan struct{})}
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
		log.Errorf("[Job] scheduler lease for %s: %v", s.key, err)
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
		log.Errorf("[Job] reconciling %s: %v", s.key, err)
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

func (s *supervisor) shutdown(ctx context.Context) {
	s.stopOnce.Do(func() { close(s.stop) })
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
	if err := s.lease.releaseContext(ctx); err != nil {
		log.Errorf("[Job] releasing lease for %s: %v", s.key, err)
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
		return value.(*supervisor).reconcile(ctx)
	}
	if value, ok := supervisors.Load(key); ok {
		return value.(*supervisor).reconcile(ctx)
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
		if !s.holdsLease() {
			return nil
		}
		if err := applyJobs(ctx, s.cron, list, s.holdsLease); err != nil {
			return err
		}
		if ctx.Err() != nil || !s.holdsLease() {
			return ctx.Err()
		}
		s.control.start()
		return saveEntryIDs(ctx, s.db, list)
	})
}
