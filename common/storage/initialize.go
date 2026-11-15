// Package storage owns the cache and queue generations installed by the host.
package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gatework/motor-core/captcha"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

// Setup builds a complete resource generation before publishing it. Queue
// subscriptions are attached before publication and Run, so reload never hands
// producers a live queue without its consumers. ctx is the host lifetime.
func Setup(ctx context.Context, settings config.Config, subscribe func(context.Context, corestorage.Queue) error) error {
	setupMu.Lock()
	defer setupMu.Unlock()
	lifecycleMu.Lock()
	closed := storageStopping
	lifecycleMu.Unlock()
	if closed {
		return nil
	}
	if err := finishPendingRetirements(ctx); err != nil {
		return err
	}
	cache, err := openCache(ctx, &settings.Cache)
	if err != nil {
		return fmt.Errorf("cache setup: %w", err)
	}
	q, err := openQueue(ctx, &settings.Queue)
	if err != nil {
		_ = cache.Close()
		return fmt.Errorf("queue setup: %w", err)
	}
	if subscribe != nil {
		if err := subscribe(ctx, q); err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), storageIOTimeout)
			defer cancel()
			return errors.Join(fmt.Errorf("queue subscriptions: %w", err), cache.Close(), q.Shutdown(cleanup))
		}
	}
	lifecycleMu.Lock()
	if storageStopping {
		lifecycleMu.Unlock()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), storageIOTimeout)
		defer cancel()
		return errors.Join(cache.Close(), q.Shutdown(cleanup))
	}
	cacheMu.Lock()
	previousCache := installedCache
	retiringCache = previousCache
	installedCache = cache
	cacheMu.Unlock()
	queueMu.Lock()
	previousQueue := installed
	installed = q
	installedGen++
	var retirement *queueRetirement
	if previousQueue != nil {
		retirement = &queueRetirement{queue: previousQueue, done: make(chan struct{})}
		retiringQueue = retirement
	}
	queueMu.Unlock()
	publishGeneration(ctx, cache, q)
	lifecycleMu.Unlock()
	registerQueueDrain()
	retirePreviousGeneration(ctx, previousCache, previousQueue, retirement)
	return nil
}

type shutdowner interface {
	Shutdown(ctx context.Context) error
}

var (
	setupMu         sync.Mutex
	lifecycleMu     sync.Mutex
	storageStopping bool
	queueMu         sync.Mutex
	installed       shutdowner
	installedGen    uint64
	retiringQueue   *queueRetirement
	drainRegistered bool
)

var setShutdown = func(f func(context.Context)) { sdk.Runtime.SetShutdown(f) }

func registerQueueDrain() {
	queueMu.Lock()
	first := !drainRegistered
	drainRegistered = true
	queueMu.Unlock()
	if first {
		setShutdown(func(ctx context.Context) {
			lifecycleMu.Lock()
			storageStopping = true
			lifecycleMu.Unlock()
			shutdownQueue(ctx)
			if err := shutdownCache(ctx); err != nil {
				logger.NewHelper(logger.Default()).Errorf("cache shutdown: %v", err)
			}
		})
	}
}

func finishedBeforeDeadline(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-ctx.Done():
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func shutdownQueue(ctx context.Context) {
	queueMu.Lock()
	current, retiring := installed, retiringQueue
	queueMu.Unlock()
	var err error
	if current != nil {
		err = current.Shutdown(ctx)
		if err == nil {
			queueMu.Lock()
			if installed == current {
				installed = nil
			}
			queueMu.Unlock()
		}
	}
	if retiring != nil {
		err = errors.Join(err, retiring.wait(ctx))
	}
	if err != nil {
		logger.NewHelper(logger.Default()).Warnf("queue shutdown: %v", err)
	}
}

// QueueGeneration increases whenever a new host-owned queue is published.
func QueueGeneration() uint64 { queueMu.Lock(); defer queueMu.Unlock(); return installedGen }

// queueRetirement coordinates bounded attempts to retire one generation. A
// timed-out attempt remains pending; shutdown can retry with its own budget.
// Concurrent waiters join the same attempt instead of duplicating Shutdown.
type queueRetirement struct {
	mu      sync.Mutex
	queue   shutdowner
	running bool
	changed chan struct{}
	done    chan struct{}
}

func (r *queueRetirement) wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		select {
		case <-r.done:
			r.mu.Unlock()
			return nil
		default:
		}
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return err
		}
		if r.running {
			changed := r.changed
			r.mu.Unlock()
			select {
			case <-changed:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		r.running = true
		r.changed = make(chan struct{})
		r.mu.Unlock()
		err := r.queue.Shutdown(ctx)
		r.mu.Lock()
		r.running = false
		if err == nil {
			close(r.done)
		}
		close(r.changed)
		r.mu.Unlock()
		return err
	}
}

func finishPendingRetirements(ctx context.Context) error {
	retirementContext, cancelRetirement := context.WithTimeout(ctx, storageIOTimeout)
	defer cancelRetirement()
	queueMu.Lock()
	pendingQueue := retiringQueue
	queueMu.Unlock()
	if pendingQueue != nil {
		if err := pendingQueue.wait(retirementContext); err != nil {
			return fmt.Errorf("previous queue retirement: %w", err)
		}
		queueMu.Lock()
		if retiringQueue == pendingQueue {
			retiringQueue = nil
		}
		queueMu.Unlock()
	}
	cacheMu.Lock()
	pendingCache := retiringCache
	cacheMu.Unlock()
	if pendingCache != nil {
		if err := pendingCache.CloseContext(retirementContext); err != nil {
			return fmt.Errorf("previous cache retirement: %w", err)
		}
		cacheMu.Lock()
		if retiringCache == pendingCache {
			retiringCache = nil
		}
		cacheMu.Unlock()
	}
	return nil
}

func retirePreviousGeneration(ctx context.Context, previousCache *ownedCache, previousQueue shutdowner, retirement *queueRetirement) {
	if previousCache != nil {
		retirementContext, cancelRetirement := context.WithTimeout(ctx, storageIOTimeout)
		defer cancelRetirement()
		if err := previousCache.CloseContext(retirementContext); err != nil {
			logger.NewHelper(logger.Default()).Errorf("cache retirement: %v", err)
		}
		cacheMu.Lock()
		select {
		case <-previousCache.closeDone:
			if retiringCache == previousCache {
				retiringCache = nil
			}
		default:
		}
		cacheMu.Unlock()
	}
	if previousQueue != nil {
		retirementContext, cancelRetirement := context.WithTimeout(ctx, storageIOTimeout)
		defer cancelRetirement()
		if err := retirement.wait(retirementContext); err != nil {
			logger.NewHelper(logger.Default()).Errorf("queue retirement: %v", err)
		}
		queueMu.Lock()
		select {
		case <-retirement.done:
			if retiringQueue == retirement {
				retiringQueue = nil
			}
		default:
		}
		queueMu.Unlock()
	}
}

// publishGeneration starts only the subscribed queue; shutdown retains its owner.
func publishGeneration(ctx context.Context, cache *ownedCache, q corestorage.Queue) {
	sdk.Runtime.SetCache(cache)
	sdk.Runtime.SetQueue(q)
	captcha.SetStore(captcha.NewCacheStore(ctx, cache, 10*time.Minute))
	go func() {
		if err := q.Run(ctx); err != nil && ctx.Err() == nil && !errors.Is(err, corestorage.ErrQueueClosed) {
			logger.NewHelper(logger.Default()).Errorf("queue runner: %v", err)
		}
	}()
}
