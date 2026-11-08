package service

import (
	"context"
	"github.com/gatework/go-net-connector/connector"
	"go-admin/app/delivery/models"
	"strings"
	"sync"
	"time"
)

// capture is an in-memory bounded receipt sink, never an application log.
type capture struct {
	mu   sync.Mutex
	body strings.Builder
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.body.Len()+len(p) > MaxOutputBytes {
		return 0, connector.ErrOutputLimit
	}
	return c.body.Write(p)
}
func (c *capture) take() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.body.String()
	c.body.Reset()
	return s
}

type deviceSession struct {
	device  *connector.Device
	capture *capture
	hooks   ExecutionHooks
}

func (d *deviceSession) before(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if d.hooks.Before != nil {
		return d.hooks.Before(ctx)
	}
	return nil
}

// Hook executors retain the connector's operation scope, but every nested send
// passes the same cancellation, identity and durable-write-intent boundary.
type guardedExecutor struct {
	connector.Executor
	before func(context.Context) error
}

func (x guardedExecutor) Execute(ctx context.Context, c connector.Command) (connector.StepResult, error) {
	if e := x.before(ctx); e != nil {
		return connector.StepResult{}, e
	}
	return x.Executor.Execute(ctx, c)
}
func newDeviceSession(p connector.Profile, cfg connector.ConnectionConfig, opts ...connector.Option) (*deviceSession, error) {
	d := &deviceSession{capture: &capture{}}
	cfg.Logger = d.capture
	prepare, after := p.Prepare, p.AfterCommand
	p.Prepare = func(ctx context.Context, c connector.Command, x connector.Executor) (connector.Command, bool, error) {
		if e := d.before(ctx); e != nil {
			return c, false, e
		}
		if prepare == nil {
			return c, false, nil
		}
		out, skip, e := prepare(ctx, c, guardedExecutor{Executor: x, before: d.before})
		if e == nil {
			e = d.before(ctx)
		}
		return out, skip, e
	}
	if after != nil {
		p.AfterCommand = func(ctx context.Context, c connector.Command, r connector.StepResult, x connector.Executor) error {
			return after(ctx, c, r, guardedExecutor{Executor: x, before: d.before})
		}
	}
	device, e := connector.New(p, cfg, opts...)
	if e != nil {
		return nil, e
	}
	d.device = device
	return d, nil
}
func (d *deviceSession) Connect(ctx context.Context) error {
	e := d.device.Connect(ctx)
	d.capture.take()
	return e
}
func (d *deviceSession) Execute(ctx context.Context, script connector.Script, h ExecutionHooks) (string, error) {
	d.hooks = h
	// Covers the connector's privilege step, which precedes Profile.Prepare.
	if e := d.before(ctx); e != nil {
		return "", e
	}
	_, e := d.device.ExecuteScript(ctx, script, connector.ExecuteOptions{MaxOutputBytes: MaxOutputBytes, OnReceipt: func(_ context.Context, _ connector.StepResult) error {
		part := d.capture.take()
		if h.Receipt != nil {
			return h.Receipt(part)
		}
		return nil
	}})
	return d.capture.take(), e
}
func (d *deviceSession) Close() error { return d.device.Close() }
func ConnectorFactory(c Config) Factory {
	return func(t models.Target) (Session, error) {
		p, e := connectorProfile(t.Connection.Profile)
		if e != nil {
			return nil, e
		}
		username, password, enable := c.credentials(t.Connection.Profile)
		return newDeviceSession(p, connector.ConnectionConfig{Host: t.Device.IP, Protocol: connector.Protocol(t.Connection.Protocol), Port: t.Connection.Port, Username: username, Password: password, EnablePassword: enable, HostKeyPolicy: connector.HostKeyPolicy(c.HostKeyPolicy), KnownHosts: c.KnownHosts, LoginTimeout: 30 * time.Second, CommandTimeout: 60 * time.Second, WriteTimeout: 10 * time.Second, MaxOutputBytes: MaxOutputBytes, MaxScriptOutputBytes: MaxOutputBytes})
	}
}
