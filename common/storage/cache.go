package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
	"github.com/gatework/motor/common/health"
)

const storageIOTimeout = 5 * time.Second

var (
	cacheMu        sync.Mutex
	installedCache *ownedCache
	retiringCache  *ownedCache
)

// ownedCache rejects new operations at retirement, waits for admitted operations,
// and closes only this generation's resource. Close callers share one waiter.
type ownedCache struct {
	native        corestorage.Cache
	closeResource func() error
	mu            sync.Mutex
	active        sync.WaitGroup
	retired       bool
	closeOnce     sync.Once
	closeErr      error
	closeDone     chan struct{}
}

func openCache(ctx context.Context, options *config.Cache) (*ownedCache, error) {
	if options == nil {
		options = &config.Cache{}
	}
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	native, err := options.Open(ctx)
	if err != nil {
		return nil, err
	}
	return &ownedCache{native: native, closeResource: native.Close}, nil
}

// Close handles the close operation.
func (c *ownedCache) Close() error { return c.CloseContext(context.Background()) }

// CloseContext handles the closecontext operation.
func (c *ownedCache) CloseContext(ctx context.Context) error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.retired = true
		c.closeDone = make(chan struct{})
		c.mu.Unlock()
		go func() { c.active.Wait(); c.closeErr = c.closeResource(); close(c.closeDone) }()
	})
	if !finishedBeforeDeadline(ctx, c.closeDone) {
		return ctx.Err()
	}
	return c.closeErr
}

// Get returns the selected data.
func (c *ownedCache) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := c.use(ctx, func() error { var err error; value, err = c.native.Get(ctx, key); return err })
	return value, err
}

// Set updates the corresponding value.
func (c *ownedCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.use(ctx, func() error { return c.native.Set(ctx, key, value, ttl) })
}

// Del handles the del operation.
func (c *ownedCache) Del(ctx context.Context, keys ...string) error {
	return c.use(ctx, func() error { return c.native.Del(ctx, keys...) })
}

// Incr handles the incr operation.
func (c *ownedCache) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	var value int64
	err := c.use(ctx, func() error { var err error; value, err = c.native.Incr(ctx, key, delta); return err })
	return value, err
}

// Expire handles the expire operation.
func (c *ownedCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.use(ctx, func() error { return c.native.Expire(ctx, key, ttl) })
}

func shutdownCache(ctx context.Context) error {
	cacheMu.Lock()
	current, retiring := installedCache, retiringCache
	cacheMu.Unlock()
	var currentErr, retiringErr error
	if current != nil {
		currentErr = current.CloseContext(ctx)
		if currentErr == nil {
			cacheMu.Lock()
			if installedCache == current {
				installedCache = nil
			}
			cacheMu.Unlock()
		}
	}
	if retiring != nil {
		retiringErr = retiring.CloseContext(ctx)
	}
	return errors.Join(currentErr, retiringErr)
}

func (c *ownedCache) use(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.retired {
		c.mu.Unlock()
		return corestorage.ErrCacheClosed
	}
	c.active.Add(1)
	c.mu.Unlock()
	defer c.active.Done()
	return fn()
}

func (c *ownedCache) probe(ctx context.Context) error {
	return c.use(ctx, func() error { return health.ProbeCache(ctx, c.native) })
}
