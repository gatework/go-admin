// Package health answers whether this process should be sent traffic.
//
// The two questions an orchestrator asks are not the same one, and motor
// answers them at two endpoints:
//
//   - /health is liveness: is the process there at all. It stays a bare 200,
//     because the honest answer to "should I restart you" is almost always no.
//     Restarting a process because its database is unreachable turns one
//     outage into a crash loop that also loses the connection pool, the cache
//     and every in-flight request.
//   - /ready is readiness: should this instance receive requests now. It fails
//     while the dependencies are unreachable, and - the part that only exists
//     because of the life-cycle phases - it fails as soon as shutdown begins,
//     before the server stops accepting.
//
// # What the draining answer is worth
//
// Order alone does not produce a window. Answering before the server stops
// accepting is the right order - the reverse reports the state after the
// connections are already cut - but with nothing between the two they are
// microseconds apart, and a poller on a multi-second interval never sees the
// 503.
//
// The delay between them is extend.shutdown.drain, which is zero unless it is
// configured. On the shipped defaults this is therefore still an answer that
// can be read rather than one anything acts on; a deployment that sets a drain
// window is the one that gets a window to act in.
//
// What acts on it depends on who does the removing. A load balancer that polls
// /ready takes this instance out when it reads the 503, and the window has to
// cover its check interval times its failure threshold, plus however long the
// removal takes to apply. On Kubernetes the endpoint is withdrawn when the Pod
// receives a deletionTimestamp, concurrently with SIGTERM and regardless of
// what the probe returns - there the window covers the delay in that removal
// reaching every node, and the 503 is what makes the state observable.
package health

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gatework/motor-core/sdk"
)

// draining is set when the process starts shutting down.
//
// It is kept here rather than read back from core: BeginShutdown sets a flag on
// the Application, but nothing exports it, and one host wanting to know is not
// yet a reason to widen that interface.
var draining atomic.Bool

// BeginDraining records that shutdown has started, so readiness fails from now
// on. It is called with BeginShutdown, before anything is taken apart.
func BeginDraining() { draining.Store(true) }

// Draining reports whether shutdown has begun.
func Draining() bool { return draining.Load() }

// Check is one dependency and what asking it produced.
type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Err  string `json:"error,omitempty"`
}

// extra are checks a host registers that this package cannot make itself.
//
// The direction is why this exists. Whether the schema matches what the binary
// expects is answered by the migration registry, which lives under cmd/ - and
// common/ has never imported cmd/. Rather than start, the host registers the
// check from where both are already in scope.
var (
	extraMu sync.RWMutex
	extra   []namedCheck
)

type namedCheck struct {
	name string
	fn   func(context.Context) error
}

// Register adds a check to what Ready asks.
//
// It panics on a duplicate name rather than replacing or appending: two checks
// under one name make the failing one impossible to identify from the response,
// and registering the same one twice is a wiring mistake worth hearing about at
// start-up rather than never.
func Register(name string, fn func(context.Context) error) {
	if name == "" {
		panic("health: a registered check needs a name")
	}
	if fn == nil {
		panic("health: check " + name + " is nil")
	}
	extraMu.Lock()
	defer extraMu.Unlock()
	for _, c := range extra {
		if c.name == name {
			panic("health: check " + name + " is already registered")
		}
	}
	extra = append(extra, namedCheck{name: name, fn: fn})
}

// registered returns the checks a host has added, copied so that Ready is not
// iterating the slice while another goroutine appends to it.
func registered() []namedCheck {
	extraMu.RLock()
	defer extraMu.RUnlock()
	out := make([]namedCheck, len(extra))
	copy(out, extra)
	return out
}

// Ready asks every dependency this process cannot serve a request without.
//
// The queue is deliberately absent. Nothing on Queue answers "are you
// reachable" without publishing something, the memory backend cannot fail, and
// a queue that is down degrades logging rather than stopping requests - which
// is a reason to alert, not a reason to leave the load balancer pool.
func Ready(ctx context.Context) []Check {
	checks := []Check{
		safely("database", func() error { return pingDB(ctx) }),
		safely("cache", func() error { return probeCache(ctx) }),
	}
	for _, c := range registered() {
		checks = append(checks, safely(c.name, func() error { return c.fn(ctx) }))
	}
	return checks
}

// safely turns a custom probe panic into a failed check.
func safely(name string, fn func() error) (c Check) {
	c = Check{Name: name}
	defer func() {
		if r := recover(); r != nil {
			c.OK, c.Err = false, fmt.Sprintf("the check panicked: %v", r)
		}
	}()
	if err := fn(); err != nil {
		c.Err = err.Error()
		return c
	}
	c.OK = true
	return c
}

// Healthy reports whether every check passed.
func Healthy(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func pingDB(ctx context.Context) error {
	databases := sdk.Runtime.Databases()
	if len(databases) == 0 {
		return errors.New("no database configured")
	}
	for tenant, db := range databases {
		if db == nil {
			return fmt.Errorf("database unavailable for tenant %q", tenant)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("database for tenant %q: %w", tenant, err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("database for tenant %q: %w", tenant, err)
		}
	}
	return nil
}

// cacheProbePrefix names the probe's keys. The key itself is per probe, not
// fixed: two /ready requests arriving together - or two instances sharing one
// redis, which is the normal deployment - would otherwise overwrite each
// other's value between the write and the read and each conclude the cache was
// broken. A readiness probe that reports false negatives under load takes
// healthy instances out of the pool, which is worse than not probing.
const cacheProbePrefix = "motor:health:"

// cacheProbeTTL is short because these keys are write-once and never read
// again by anyone else; it only has to outlive the read that follows.
const cacheProbeTTL = 30

// SetCacheProbe installs a custom readiness check. Passing nil uses the
// context-aware cache published by the host.
func SetCacheProbe(fn func(context.Context) error) func(context.Context) error {
	cacheProbeMu.Lock()
	defer cacheProbeMu.Unlock()
	previous := cacheProbeOverride
	cacheProbeOverride = fn
	return previous
}

var (
	cacheProbeMu       sync.RWMutex
	cacheProbeOverride func(context.Context) error
)

func probeCache(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cacheProbeMu.RLock()
	probe := cacheProbeOverride
	cacheProbeMu.RUnlock()
	if probe != nil {
		return probe(ctx)
	}
	cache := sdk.Runtime.Cache()
	if cache == nil {
		return errors.New("no cache configured")
	}
	return ProbeCache(ctx, cache)
}

// ContextCache is the narrow read/write contract needed by the readiness check.
type ContextCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// ProbeCache verifies a round trip using one key and the caller's deadline.
// Cleanup uses that same deadline; TTL reclaims the key after cancellation.
func ProbeCache(ctx context.Context, adapter ContextCache) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("could not build a probe key: %w", err)
	}
	key := cacheProbePrefix + hex.EncodeToString(suffix)
	want := time.Now().Format(time.RFC3339Nano)
	if err := adapter.Set(ctx, key, want, cacheProbeTTL*time.Second); err != nil {
		return err
	}
	defer func() { _ = adapter.Del(ctx, key) }()
	got, err := adapter.Get(ctx, key)
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("the cache returned a different value than was written")
	}
	return ctx.Err()
}
