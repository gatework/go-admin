package jobs

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"
)

// cronControl bounds blocked cron API work to one goroutine per scheduler.
// A custom Schedule.Next may block cron's command loop. Callers can time out
// without holding supervisor state locks or spawning another blocked worker.
// Pending stop is drained before a subsequent reconciliation can start cron.
type cronControl struct {
	cron        *cron.Cron
	mu          sync.Mutex
	busy        chan struct{}
	stopPending bool
	started     bool
	stopped     context.Context
}

func (c *cronControl) run(ctx context.Context, fn func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		if c.busy == nil {
			c.busy = make(chan struct{})
			result := make(chan error, 1)
			c.mu.Unlock()
			go c.execute(fn, result)
			select {
			case err := <-result:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		busy := c.busy
		c.mu.Unlock()
		select {
		case <-busy:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *cronControl) execute(fn func() error, result chan<- error) {
	var err error
	if fn != nil {
		err = fn()
	}
	for {
		c.mu.Lock()
		if c.stopPending {
			c.stopPending = false
			started := c.started
			c.started = false
			c.mu.Unlock()
			if started {
				stopped := c.cron.Stop()
				c.mu.Lock()
				c.stopped = stopped
				c.mu.Unlock()
			}
			continue
		}
		close(c.busy)
		c.busy = nil
		c.mu.Unlock()
		if result != nil {
			result <- err
		}
		return
	}
}

// start is called only inside run, after verifying current lease validity.
func (c *cronControl) start() {
	c.mu.Lock()
	c.started = true
	c.stopped = nil
	c.mu.Unlock()
	c.cron.Start()
}

func (c *cronControl) requestStop() {
	c.mu.Lock()
	c.stopPending = true
	if c.busy != nil {
		c.mu.Unlock()
		return
	}
	c.busy = make(chan struct{})
	c.mu.Unlock()
	go c.execute(nil, nil)
}

func (c *cronControl) waitStopped(ctx context.Context) {
	for {
		c.mu.Lock()
		busy, stopped := c.busy, c.stopped
		c.mu.Unlock()
		if busy == nil {
			if stopped != nil {
				select {
				case <-stopped.Done():
				case <-ctx.Done():
				}
			}
			return
		}
		select {
		case <-busy:
		case <-ctx.Done():
			return
		}
	}
}
