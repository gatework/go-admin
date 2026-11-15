// Package testconfig publishes immutable core settings for host integration tests.
package testconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gatework/motor-core/config/source/memory"
	"github.com/gatework/motor-core/sdk/config"
)

// Apply publishes a modified snapshot and restores the previous one at cleanup.
func Apply(tb testing.TB, modify func(*config.Config)) {
	tb.Helper()
	tb.Cleanup(Swap(tb, modify))
}

// Swap installs a scoped snapshot and returns its restoration function.
func Swap(tb testing.TB, modify func(*config.Config)) func() {
	tb.Helper()
	old := config.Current()
	next := config.Current()
	modify(&next.Settings)
	publish(tb, next)
	return func() { publish(tb, old) }
}

func publish(tb testing.TB, s *config.Settings) {
	tb.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		tb.Fatal(err)
	}
	w, err := config.Setup(context.Background(), memory.NewSource(memory.WithJSON(data)), config.Options{})
	if err != nil {
		tb.Fatal(err)
	}
	if err = w.Close(); err != nil {
		tb.Fatal(err)
	}
}
