package settings

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/config/source/memory"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
	hostconfig "github.com/gatework/motor/config"
)

type trackedLogger struct {
	logger.Logger
	closed atomic.Int32
}

func (l *trackedLogger) Close() error { l.closed.Add(1); return nil }

func preserveGlobals(t *testing.T) {
	t.Helper()
	previous, extra, snapshot := logger.Default(), hostconfig.Ext(), config.Current()
	owners.Lock()
	owned := owners.logger
	owners.logger = nil
	owners.Unlock()
	t.Cleanup(func() {
		if err := CloseLogger(); err != nil {
			t.Error(err)
		}
		owners.Lock()
		owners.logger = owned
		owners.Unlock()
		logger.SetDefault(previous)
		hostconfig.SetExt(extra)
		data, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		watcher, err := config.Setup(context.Background(), memory.NewSource(memory.WithJSON(data)), config.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := watcher.Close(); err != nil {
			t.Error(err)
		}
	})
}

func TestRejectedPreparationKeepsPublishedStateAndClosesCandidate(t *testing.T) {
	for _, reason := range []string{"cancelled", "callback failed"} {
		t.Run(reason, func(t *testing.T) {
			preserveGlobals(t)
			current := &trackedLogger{Logger: logger.Default()}
			candidate := &trackedLogger{Logger: logger.Default()}
			logger.SetDefault(current)
			owners.Lock()
			owners.logger = current
			owners.Unlock()
			hostconfig.SetExt(hostconfig.Extend{Metrics: hostconfig.Metrics{Token: "current"}})
			before := config.Current().Settings.Application.Name
			app := runtime.New()
			app.SetLogger(current)
			w := &Watcher{app: app, openLogger: func(config.Logger) (logger.Logger, error) { return candidate, nil }}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			expected := errors.New("preparation failed")
			err := w.start(ctx, memory.NewSource(memory.WithJSON([]byte(`{"settings":{"application":{"name":"candidate"},"extend":{"metrics":{"token":"candidate"}}}}`))), []func(*config.Settings) error{func(*config.Settings) error {
				if reason == "cancelled" {
					cancel()
					return nil
				}
				return expected
			}})
			if reason == "cancelled" {
				expected = context.Canceled
			}
			if !errors.Is(err, expected) {
				t.Fatalf("failure lost: %v", err)
			}
			if logger.Default() != current || app.Logger() != current || hostconfig.Ext().Metrics.Token != "current" {
				t.Fatal("rejected settings published host resources")
			}
			if config.Current().Settings.Application.Name != before {
				t.Fatal("rejected settings replaced core snapshot")
			}
			if candidate.closed.Load() != 1 || current.closed.Load() != 0 {
				t.Fatalf("wrong root ownership: candidate=%d current=%d", candidate.closed.Load(), current.closed.Load())
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if candidate.closed.Load() != 1 {
				t.Fatal("candidate closed twice")
			}
		})
	}
}

func TestWatcherCloseDiscardsCancelledReloadAndKeepsCommittedRoot(t *testing.T) {
	preserveGlobals(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	app := runtime.New()
	opened := make(chan *trackedLogger, 2)
	base := logger.Default()
	w := &Watcher{app: app, openLogger: func(config.Logger) (logger.Logger, error) {
		root := &trackedLogger{Logger: base}
		opened <- root
		return root, nil
	}}
	prepared := make(chan struct{})
	src := memory.NewSource(memory.WithJSON([]byte(`{"settings":{"application":{"name":"committed"},"extend":{"metrics":{"token":"committed"}}}}`)))
	if err := w.start(ctx, src, []func(*config.Settings) error{func(candidate *config.Settings) error {
		if candidate.Settings.Application.Name == "cancelled reload" {
			cancel()
			close(prepared)
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	committed := <-opened
	if logger.Default() != committed || app.Logger() != committed || hostconfig.Ext().Metrics.Token != "committed" {
		t.Fatal("accepted settings did not commit host resources")
	}
	if err := src.Write(&source.ChangeSet{Format: "json", Data: []byte(`{"settings":{"application":{"name":"cancelled reload"},"extend":{"metrics":{"token":"cancelled"}}}}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-prepared:
	case <-time.After(3 * time.Second):
		t.Fatal("reload did not reach preparation")
	}
	staged := <-opened
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if staged.closed.Load() != 1 || committed.closed.Load() != 0 {
		t.Fatalf("wrong root ownership after close: staged=%d committed=%d", staged.closed.Load(), committed.closed.Load())
	}
	if logger.Default() != committed || app.Logger() != committed || hostconfig.Ext().Metrics.Token != "committed" || config.Current().Settings.Application.Name != "committed" {
		t.Fatal("cancelled reload replaced committed state")
	}
	if err := CloseLogger(); err != nil {
		t.Fatal(err)
	}
	if committed.closed.Load() != 1 {
		t.Fatal("host did not close committed root")
	}
}
