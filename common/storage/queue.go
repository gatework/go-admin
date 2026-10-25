package storage

import (
	"context"
	"sync"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	corestorage "github.com/go-admin-team/go-admin-core/v2/storage"
	corequeue "github.com/go-admin-team/go-admin-core/v2/storage/queue"
	redisstore "github.com/go-admin-team/go-admin-core/v2/storage/redis"
)

// Register starts memory consumers immediately. The core Run method only waits
// for Shutdown and races its WaitGroup Add against Done; the host needs no
// extra runner. Shutdown still owns draining every registered consumer.
type ownedMemoryQueue struct {
	*corequeue.Memory
	once sync.Once
}

func (*ownedMemoryQueue) Run() {}

func (q *ownedMemoryQueue) Shutdown() {
	q.once.Do(q.Memory.Shutdown)
}

type ownedRedisQueue struct {
	corestorage.AdapterQueue
	closeClient func() error
	once        sync.Once
	mu          sync.Mutex
	active      sync.WaitGroup
	closing     bool
}

func (q *ownedRedisQueue) Shutdown() {
	q.once.Do(func() {
		q.mu.Lock()
		q.closing = true
		q.mu.Unlock()
		q.active.Wait()
		q.AdapterQueue.Shutdown()
		_ = q.closeClient()
	})
}

func (q *ownedRedisQueue) Append(message corestorage.Messager) error {
	q.mu.Lock()
	if q.closing {
		q.mu.Unlock()
		return corestorage.ErrQueueClosed
	}
	q.active.Add(1)
	q.mu.Unlock()
	defer q.active.Done()
	return q.AdapterQueue.Append(message)
}

type deadlineQueue struct{ corestorage.Queue }

func (q deadlineQueue) String() string { return "redis" }

func (q deadlineQueue) Publish(ctx context.Context, message corestorage.Message) error {
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return q.Queue.Publish(ctx, message)
}

func openQueue(c *config.Queue) (corestorage.AdapterQueue, error) {
	if c == nil || c.Empty() {
		return &ownedMemoryQueue{Memory: corequeue.NewMemory(10000)}, nil
	}
	if c.Redis == nil {
		return &ownedMemoryQueue{Memory: corequeue.NewMemory(c.Memory.PoolSize)}, nil
	}
	client, err := openRedisClient(&c.Redis.RedisOptions)
	if err != nil {
		return nil, err
	}
	native := redisstore.NewQueue(client, redisstore.QueueOptions{
		Group: c.Redis.Group, KeyPrefix: c.Redis.KeyPrefix, MaxAttempts: c.Redis.MaxAttempts,
		ClaimMinIdle: time.Duration(c.Redis.ClaimMinIdleSeconds) * time.Second,
	})
	return &ownedRedisQueue{AdapterQueue: corestorage.LegacyQueueAdapter(deadlineQueue{native}), closeClient: client.Close}, nil
}
