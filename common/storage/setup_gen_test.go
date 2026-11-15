package storage

import (
	"testing"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
)

// Issue #892: a configuration reload replaces the queue adapter and the
// consumers registered against the previous one are attached to a queue nobody
// publishes to any more.
//
// The fix has two halves. attachQueueConsumers gives a new queue its own
// consumers and the same queue none, which cmd/api covers against a queue the
// test controls. This is the other half: that a reload actually produces a new
// queue for it to notice. Setup is what config re-runs on every change, so
// calling it twice is what a reload does to this package.
func TestSetupBumpsTheQueueGenerationOnEveryReload(t *testing.T) {
	// Setup writes the process-wide sdk.Runtime - the cache and queue adapters -
	// and this package's own record of what it installed. Restoring all of it
	// keeps the test from deciding what a later test in this binary sees,
	// which is the same isolation cmd/api's freshRuntime provides.
	prevQ, prevC := testQueueConfig, testCacheConfig
	prevRuntime := sdk.Runtime
	prevInstalled, prevGen := installed, installedGen
	t.Cleanup(func() {
		testQueueConfig, testCacheConfig = prevQ, prevC
		sdk.Runtime = prevRuntime
		queueMu.Lock()
		installed, installedGen = prevInstalled, prevGen
		queueMu.Unlock()
	})
	sdk.Runtime = runtime.New()

	testCacheConfig = &config.Cache{Memory: &config.CacheMemory{}}
	testQueueConfig = &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}

	before := QueueGeneration()
	setupStorage(t)
	first := QueueGeneration()
	setupStorage(t)
	second := QueueGeneration()

	t.Logf("before=%d first=%d second=%d", before, first, second)
	if first == before {
		t.Fatal("the first Setup did not install a queue")
	}
	if second == first {
		t.Fatal("a second Setup - which is what a configuration reload does - did not install a new one")
	}
}
