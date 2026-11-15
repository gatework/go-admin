package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
	corecache "github.com/gatework/motor-core/storage/cache"

	"github.com/gatework/motor/common/health"
)

func TestOwnedCacheOperationsAndPrefixes(t *testing.T) {
	isolate(t)
	testCacheConfig = &config.Cache{Memory: &config.CacheMemory{}}
	setupStorage(t)
	cache := corestorage.WithPrefix(sdk.Runtime.Cache(), "tenant:")
	if err := cache.Set(t.Context(), "count", "2", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Incr(t.Context(), "count", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Incr(t.Context(), "count", -1); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Get(t.Context(), "count"); err != nil || got != "2" {
		t.Fatalf("counter: %q %v", got, err)
	}
	if got, err := sdk.Runtime.Cache().Get(t.Context(), "tenant:count"); err != nil || got != "2" {
		t.Fatalf("hash: %q %v", got, err)
	}
	if err := cache.Expire(t.Context(), "count", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := cache.Del(t.Context(), "count"); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Get(t.Context(), "count"); !errors.Is(err, corestorage.ErrCacheMiss) || got != "" {
		t.Fatalf("miss: %q %v", got, err)
	}
}

type blockingMemory struct {
	*corecache.Memory
	entered, release chan struct{}
}

func (c *blockingMemory) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	close(c.entered)
	<-c.release
	return c.Memory.Set(ctx, key, value, ttl)
}
func TestCacheRetirementWaitsForActiveOperationAndBoundsCaller(t *testing.T) {
	memory := &blockingMemory{Memory: corecache.New(corecache.Options{SweepInterval: time.Minute}), entered: make(chan struct{}), release: make(chan struct{})}
	var closes atomic.Int64
	cache := &ownedCache{native: memory, closeResource: func() error { closes.Add(1); return memory.Close() }}
	work := make(chan error, 1)
	go func() { work <- cache.Set(t.Context(), "key", "value", time.Minute) }()
	<-memory.entered
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		err := cache.CloseContext(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("retirement wait: %v", err)
		}
	}
	if closes.Load() != 0 {
		t.Fatal("client closed during an active operation")
	}
	if _, err := cache.Get(t.Context(), "key"); !errors.Is(err, corestorage.ErrCacheClosed) {
		t.Fatalf("retired cache accepted a new operation: %v", err)
	}
	close(memory.release)
	if err := <-work; err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatalf("resource closed %d times", closes.Load())
	}
}

// replyProxy pauses Redis replies on this fixture's connections only. Real
// socket deadlines must release the cache call; the Redis server stays usable
// by other tests throughout.
func replyProxy(t *testing.T, upstream string) (string, *atomic.Bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var pause atomic.Bool
	release := make(chan struct{})
	var workers sync.WaitGroup
	var connections sync.Map
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", upstream)
			if err != nil {
				client.Close()
				continue
			}
			connections.Store(client, server)
			workers.Go(func() {
				defer connections.Delete(client)
				defer client.Close()
				defer server.Close()
				copied := make(chan struct{})
				go func() { defer close(copied); io.Copy(server, client); server.Close() }()
				buffer := make([]byte, 4096)
				for {
					n, err := server.Read(buffer)
					if n > 0 {
						if pause.Load() {
							<-release
						}
						if _, e := client.Write(buffer[:n]); e != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				client.Close()
				server.Close()
				<-copied
			})
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-done
		close(release)
		connections.Range(func(client, server any) bool { client.(net.Conn).Close(); server.(net.Conn).Close(); return true })
		workers.Wait()
	})
	return listener.Addr().String(), &pause
}

func TestRedisReadinessHonorsSocketDeadline(t *testing.T) {
	addr, pause := replyProxy(t, redisAddr(t))
	cache, err := openCache(t.Context(), &config.Cache{Redis: &config.RedisOptions{Addr: addr, MaxRetries: -1}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	previous := health.SetCacheProbe(cache.probe)
	t.Cleanup(func() { health.SetCacheProbe(previous) })
	pause.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	checks := health.Ready(ctx)
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Redis I/O ignored readiness deadline: %v", elapsed)
	}
	for _, check := range checks {
		if check.Name == "cache" {
			if check.OK {
				t.Fatal("blocked Redis reply passed readiness")
			}
			return
		}
	}
	t.Fatal("cache check missing")
}

func TestRedisCacheGenerationsHaveIndependentClients(t *testing.T) {
	options := &config.Cache{Redis: &config.RedisOptions{Addr: redisAddr(t)}}
	first, err := openCache(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := openCache(t.Context(), options)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.probe(t.Context()); err != nil {
		t.Fatalf("retiring previous generation closed current client: %v", err)
	}
}

func TestRetiredRedisResourcesCloseTheirClientConnections(t *testing.T) {
	addr := redisAddr(t)
	observer, err := openRedisClient(&config.RedisOptions{Addr: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	for _, kind := range []string{"cache", "queue"} {
		t.Run(kind, func(t *testing.T) {
			name := fmt.Sprintf("lifecycle-%s-%d", kind, time.Now().UnixNano())
			options := config.RedisOptions{URL: "redis://" + addr + "/0?client_name=" + name}
			var closeResource func()
			if kind == "cache" {
				resource, err := openCache(t.Context(), &config.Cache{Redis: &options})
				if err != nil {
					t.Fatal(err)
				}
				closeResource = func() { resource.Close() }
			} else {
				resource, err := openQueue(t.Context(), &config.Queue{Redis: &config.RedisQueue{RedisOptions: options}})
				if err != nil {
					t.Fatal(err)
				}
				closeResource = func() { _ = resource.Shutdown(t.Context()) }
			}
			defer closeResource()
			list, err := observer.ClientList(t.Context()).Result()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(list, "name="+name+" ") {
				t.Fatal("fixture did not open named Redis connection")
			}
			closeResource()
			deadline := time.Now().Add(time.Second)
			for {
				list, err = observer.ClientList(t.Context()).Result()
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(list, "name="+name+" ") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("retired Redis client connection remains open")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestShutdownAccountsForCacheAlreadyRetiringDuringReload(t *testing.T) {
	isolate(t)
	memory := &blockingMemory{Memory: corecache.New(corecache.Options{SweepInterval: time.Minute}), entered: make(chan struct{}), release: make(chan struct{})}
	previous := &ownedCache{native: memory, closeResource: memory.Close}
	cacheMu.Lock()
	installedCache = previous
	cacheMu.Unlock()
	operation := make(chan error, 1)
	go func() { operation <- previous.Set(t.Context(), "key", "value", time.Minute) }()
	<-memory.entered
	testCacheConfig = &config.Cache{}
	reloaded := make(chan struct{})
	go func() { defer close(reloaded); setupStorage(t) }()
	deadline := time.Now().Add(time.Second)
	for {
		previous.mu.Lock()
		retired := previous.retired
		previous.mu.Unlock()
		if retired {
			break
		}
		if time.Now().After(deadline) {
			close(memory.release)
			t.Fatal("reload did not retire old cache")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := shutdownCache(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown ignored still-active retired cache: %v", err)
	}
	close(memory.release)
	if err := <-operation; err != nil {
		t.Fatal(err)
	}
	<-reloaded
	if err := shutdownCache(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownRetainsCurrentCacheUntilActiveOperationCompletes(t *testing.T) {
	isolate(t)
	memory := &blockingMemory{Memory: corecache.New(corecache.Options{SweepInterval: time.Minute}), entered: make(chan struct{}), release: make(chan struct{})}
	current := &ownedCache{native: memory, closeResource: memory.Close}
	cacheMu.Lock()
	installedCache = current
	cacheMu.Unlock()
	work := make(chan error, 1)
	go func() { work <- current.Set(t.Context(), "key", "value", time.Minute) }()
	<-memory.entered
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	err := shutdownCache(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown=%v", err)
	}
	cacheMu.Lock()
	held := installedCache
	cacheMu.Unlock()
	if held != current {
		t.Fatal("shutdown dropped active cache")
	}
	close(memory.release)
	if err := <-work; err != nil {
		t.Fatal(err)
	}
	if err := shutdownCache(t.Context()); err != nil {
		t.Fatal(err)
	}
	cacheMu.Lock()
	held = installedCache
	cacheMu.Unlock()
	if held != nil {
		t.Fatal("completed shutdown retained cache")
	}
}
