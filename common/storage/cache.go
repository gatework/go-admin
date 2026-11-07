package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	corestorage "github.com/go-admin-team/go-admin-core/v2/storage"
	corecache "github.com/go-admin-team/go-admin-core/v2/storage/cache"
	redisstore "github.com/go-admin-team/go-admin-core/v2/storage/redis"
	goredis "github.com/redis/go-redis/v9"

	"go-admin/common/health"
)

const storageIOTimeout = 5 * time.Second

var (
	cacheMu        sync.Mutex
	cacheSetupMu   sync.Mutex
	installedCache *ownedCache
	retiringCache  *ownedCache
)

// ownedCache keeps the legacy cache semantics while retaining a context-aware
// readiness path and the resources that the legacy core wrapper hides.
// Retirement refuses new operations and waits for already-active ones before
// closing this generation's client. Clients are never shared across generations.
type ownedCache struct {
	legacy        corestorage.AdapterCache
	native        corestorage.Cache
	closeResource func() error
	mu            sync.Mutex
	active        sync.WaitGroup
	retired       bool
	closeOnce     sync.Once
	closeErr      error
	closeDone     chan struct{}
}

func openCache(c *config.Cache) (*ownedCache, error) {
	if c == nil || c.Redis == nil {
		memory := corecache.NewMemory()
		return &ownedCache{legacy: memory, closeResource: memory.Close}, nil
	}
	client, err := openRedisClient(c.Redis)
	if err != nil {
		return nil, err
	}
	native := redisstore.New(client)
	return &ownedCache{legacy: corestorage.LegacyAdapter(deadlineCache{native}), native: native,
		closeResource: func() error { return errors.Join(native.Close(), client.Close()) }}, nil
}

func openRedisClient(options *config.RedisOptions) (*goredis.Client, error) {
	resolved, err := options.GetRedisOptions()
	if err != nil {
		return nil, err
	}
	// The Redis client otherwise discards request deadlines for socket I/O.
	resolved.ContextTimeoutEnabled = true
	client := goredis.NewClient(resolved)
	ctx, cancel := context.WithTimeout(context.Background(), storageIOTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *ownedCache) acquire() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retired {
		return corestorage.ErrCacheClosed
	}
	c.active.Add(1)
	return nil
}
func (c *ownedCache) use(fn func() error) error {
	if err := c.acquire(); err != nil {
		return err
	}
	defer c.active.Done()
	return fn()
}
func (c *ownedCache) Close() error { return c.CloseContext(context.Background()) }
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
func (c *ownedCache) String() string { return c.legacy.String() }
func (c *ownedCache) Get(key string) (value string, err error) {
	err = c.use(func() error { var e error; value, e = c.legacy.Get(key); return e })
	return
}
func (c *ownedCache) Set(key string, value any, ttl int) error {
	return c.use(func() error { return c.legacy.Set(key, value, ttl) })
}
func (c *ownedCache) Del(key string) error { return c.use(func() error { return c.legacy.Del(key) }) }
func (c *ownedCache) HashGet(hash, key string) (value string, err error) {
	err = c.use(func() error { var e error; value, e = c.legacy.HashGet(hash, key); return e })
	return
}
func (c *ownedCache) HashDel(hash, key string) error {
	return c.use(func() error { return c.legacy.HashDel(hash, key) })
}
func (c *ownedCache) Increase(key string) error {
	return c.use(func() error { return c.legacy.Increase(key) })
}
func (c *ownedCache) Decrease(key string) error {
	return c.use(func() error { return c.legacy.Decrease(key) })
}
func (c *ownedCache) Expire(key string, ttl time.Duration) error {
	return c.use(func() error { return c.legacy.Expire(key, ttl) })
}
func (c *ownedCache) probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.use(func() error {
		if c.native != nil {
			return health.ProbeCache(ctx, c.native)
		}
		return health.ProbeCache(ctx, memoryProbe{c.legacy})
	})
}

// Memory operations remain on the original adapter, including miss, hash,
// counter and expiry behavior. They perform no remote I/O.
type memoryProbe struct{ corestorage.AdapterCache }

func (c memoryProbe) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return c.AdapterCache.Get(key)
}
func (c memoryProbe) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.AdapterCache.Set(key, value, int(ttl/time.Second))
}
func (c memoryProbe) Del(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.AdapterCache.Del(key); err != nil {
			return err
		}
	}
	return nil
}

// Legacy calls have no caller context. Bound their Redis I/O so retirement
// cannot wait forever on a caller that captured the previous runtime wrapper.
type deadlineCache struct{ corestorage.Cache }

func (c deadlineCache) String() string { return "redis" }

func (c deadlineCache) Get(ctx context.Context, key string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return c.Cache.Get(ctx, key)
}
func (c deadlineCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return c.Cache.Set(ctx, key, value, ttl)
}
func (c deadlineCache) Del(ctx context.Context, keys ...string) error {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return c.Cache.Del(ctx, keys...)
}
func (c deadlineCache) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return c.Cache.Incr(ctx, key, delta)
}
func (c deadlineCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return c.Cache.Expire(ctx, key, ttl)
}

func shutdownCache(ctx context.Context) error {
	cacheMu.Lock()
	current, retiring := installedCache, retiringCache
	installedCache = nil
	cacheMu.Unlock()
	var currentErr, retiringErr error
	if current != nil {
		currentErr = current.CloseContext(ctx)
	}
	if retiring != nil {
		retiringErr = retiring.CloseContext(ctx)
	}
	return errors.Join(currentErr, retiringErr)
}
