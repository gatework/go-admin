package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/motor-core/logger"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/common/worker"
)

const executionTimeout = 10 * time.Minute

// NoWorkErr indicates that no delivery target is currently claimable.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var NoWorkErr = errors.New("no delivery work available")

//revive:enable:error-naming

// Session exposes no retries: every Send represents a potentially irreversible write.
type Session interface {
	Connect(ctx context.Context) error
	Execute(ctx context.Context, script connector.Script, hooks ExecutionHooks) (string, error)
	Close() error
}

// ExecutionHooks records authorization and execution receipts around device commands.
type ExecutionHooks struct {
	Before  func(context.Context) error
	Receipt func(string) error
}

// Factory creates an independently owned device session for a target.
type Factory func(models.Target) (Session, error)

// Worker advances queued operations while holding the required device leases.
type Worker struct {
	loop    worker.Loop
	s       *Workflow
	factory Factory
}

// NewWorker constructs an application-owned background worker.
func NewWorker(s *Workflow, f Factory) *Worker {
	if f == nil {
		f = ConnectorFactory(s.Config)
	}
	// A background worker has no caller to return an error to; its log line is the
	// only trace of a failed tick, so it must never depend on the host's wiring.
	if s.Logger == nil {
		s.Logger = logger.NewHelper(logger.Default())
	}
	return &Worker{s: s, factory: f, loop: worker.Loop{Name: "delivery worker", Logger: s.Logger}}
}

// Run owns the worker lifecycle until cancellation and waits for its pollers.
func (w *Worker) Run(ctx context.Context) error {
	concurrency := w.s.Config.Concurrency
	if concurrency < 1 {
		concurrency = 5
	}
	return w.loop.Run(ctx, concurrency, func(ctx context.Context) error {
		if !w.s.Config.Enabled {
			return nil
		}
		if err := w.Recover(ctx); err != nil {
			return fmt.Errorf("recovery: %w", err)
		}
		target, err := w.Claim(ctx)
		if errors.Is(err, NoWorkErr) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim: %w", err)
		}
		if target != nil {
			w.RunTarget(ctx, *target)
		}
		return nil
	})
}
