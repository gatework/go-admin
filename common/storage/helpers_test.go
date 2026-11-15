package storage

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

var (
	testCacheConfig *config.Cache
	testQueueConfig *config.Queue
	testSubscribe   = func(ctx context.Context, q corestorage.Queue) error {
		return q.Subscribe(ctx, "t", func(context.Context, corestorage.Message) error { return nil })
	}
)

func setupStorage(t *testing.T) {
	t.Helper()
	settings := config.Config{}
	if testCacheConfig != nil {
		settings.Cache = *testCacheConfig
	}
	if testQueueConfig != nil {
		settings.Queue = *testQueueConfig
	}
	if err := Setup(t.Context(), settings, testSubscribe); err != nil {
		t.Fatal(err)
	}
}

func openRedisClient(options *config.RedisOptions) (*goredis.Client, error) {
	resolved, err := options.Options()
	if err != nil {
		return nil, err
	}
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
