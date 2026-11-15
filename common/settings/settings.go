// Package settings owns host resources created while loading configuration.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
	hostconfig "github.com/gatework/motor/config"
)

var owners struct {
	sync.Mutex
	logger logger.Logger
}

// Watcher owns the core settings watcher and resources prepared for a candidate
// that core has not committed. Committed logger roots belong to the host and
// remain available until CloseLogger, after application resource shutdown.
type Watcher struct {
	core       *config.Watcher
	app        *runtime.Application
	openLogger func(config.Logger) (logger.Logger, error)
	mu         sync.Mutex
	staged     logger.Logger
	extra      hostconfig.Extend
	closeOnce  sync.Once
	closeErr   error
}

// Load validates and prepares candidates, but publishes host settings and the
// logger only after core commits the same candidate. Close stops and waits for
// the watcher before the caller shuts down application resources.
func Load(ctx context.Context, app *runtime.Application, src source.Source, prepare ...func(*config.Settings) error) (*Watcher, error) {
	w := &Watcher{app: app, openLogger: func(c config.Logger) (logger.Logger, error) { return c.Open() }}
	if err := w.start(ctx, src, prepare); err != nil {
		return nil, err
	}
	return w, nil
}

// Close stops the source watcher and waits for in-progress preparation before
// releasing an uncommitted logger. Repeated calls wait for the same close.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		if w.core != nil {
			w.closeErr = w.core.Close()
		}
		w.closeErr = errors.Join(w.closeErr, w.discard())
	})
	return w.closeErr
}

// CloseLogger closes only the logger root committed by this package.
func CloseLogger() error {
	owners.Lock()
	owned := owners.logger
	owners.logger = nil
	owners.Unlock()
	return closeLogger(owned)
}

func closeLogger(log logger.Logger) error {
	if log == nil {
		return nil
	}
	if c, ok := log.(io.Closer); ok {
		return c.Close()
	}
	if s, ok := log.(interface{ Sync() error }); ok {
		return s.Sync()
	}
	return nil
}

func (w *Watcher) start(ctx context.Context, src source.Source, prepare []func(*config.Settings) error) error {
	if w.app == nil {
		return errors.New("settings: application is required")
	}
	core, err := config.Setup(ctx, src, config.Options{
		Callbacks: []func(*config.Settings) error{func(candidate *config.Settings) error {
			return w.prepare(candidate, prepare)
		}},
		OnApplied: func(*config.Settings) {
			w.commit()
			w.app.RunPhase(runtime.AfterResource)
		},
	})
	if err != nil {
		return errors.Join(err, w.discard())
	}
	w.core = core
	return nil
}

func (w *Watcher) prepare(candidate *config.Settings, callbacks []func(*config.Settings) error) error {
	if err := w.discard(); err != nil {
		return err
	}
	raw, err := json.Marshal(candidate.Settings.Extend)
	if err != nil {
		return err
	}
	var extra hostconfig.Extend
	if err := json.Unmarshal(raw, &extra); err != nil {
		return err
	}
	if _, err := extra.Shutdown.Budget(); err != nil {
		return err
	}
	next, err := w.openLogger(candidate.Settings.Logger)
	if err != nil {
		return err
	}
	for _, fn := range callbacks {
		if fn == nil {
			continue
		}
		if err := fn(candidate); err != nil {
			return errors.Join(err, closeLogger(next))
		}
	}
	w.mu.Lock()
	w.staged, w.extra = next, extra
	w.mu.Unlock()
	return nil
}

func (w *Watcher) commit() {
	w.mu.Lock()
	next, extra := w.staged, w.extra
	w.staged = nil
	w.mu.Unlock()
	hostconfig.SetExt(extra)
	owners.Lock()
	previous := owners.logger
	owners.logger = next
	owners.Unlock()
	logger.SetDefault(next)
	w.app.SetLogger(next)
	if err := closeLogger(previous); err != nil {
		logger.NewHelper(next).Errorf("retiring logger: %v", err)
	}
}

func (w *Watcher) discard() error {
	w.mu.Lock()
	staged := w.staged
	w.staged = nil
	w.mu.Unlock()
	return closeLogger(staged)
}
