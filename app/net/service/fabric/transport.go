package fabric

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/go-net-connector/vendors"
	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/go-vxlan/adapter"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	ds "github.com/gatework/motor/app/net/service/delivery"
)

// Transport lends one authenticated operation for preflight, writes and readback.
// It must never reconnect or retry inside the callback.
type Transport interface {
	Open(ctx context.Context, device delivery.DeviceSnapshot, connection delivery.Connection, run func(Session) error) error
}

// Session describes fabric session for fabric operations.
type Session interface {
	vxlan.Session
	Write(ctx context.Context, command string) error
}
type nativeTransport struct{ config ds.Config }

// Native transport retains explicit closure evidence even when the callback
// returns a policy/readback error; a closed failed operation need not strand a lease.
// Open provides the open operation within the caller's authorized fabric scope.
func (n nativeTransport) Open(ctx context.Context, d delivery.DeviceSnapshot, c delivery.Connection, fn func(Session) error) (err error) {
	if c.Profile != "cisco_nxos" {
		return transportReceiptError{UnsupportedPlatformErr, true}
	}
	cfg, e := n.config.ConnectionConfig(d, c)
	if e != nil {
		return transportReceiptError{e, true}
	}
	platform, e := vendors.NewBuiltinRegistry().Lookup(c.Profile)
	if e != nil {
		return transportReceiptError{e, true}
	}
	device, e := connector.New(platform.Profile(), cfg)
	if e != nil {
		return transportReceiptError{e, true}
	}
	defer func() {
		closeErr := device.Close()
		if combined := errors.Join(err, closeErr); combined != nil {
			err = transportReceiptError{combined, closeErr == nil}
		}
	}()
	if e := device.Connect(ctx); e != nil {
		return e
	}
	return device.WithOperation(ctx, func(_ context.Context, op *connector.Operation) error {
		return fn(adapter.Session{Operation: op, Device: device})
	})
}

type transportReceiptError struct {
	error
	closed bool
}

// TransportClosed provides the transport closed operation within the caller's authorized fabric scope.
func (e transportReceiptError) TransportClosed() bool { return e.closed }
func transportClosed(err error) bool {
	if err == nil {
		return true
	}
	var receipt interface{ TransportClosed() bool }
	return errors.As(err, &receipt) && receipt.TransportClosed()
}

func opsDB(ctx context.Context, s *Fabric) *gorm.DB {
	return s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}

func (s *Fabric) deviceConnection(ctx context.Context, d network.Device) (delivery.Connection, error) {
	workflow := ds.Workflow{Service: s.Service, Config: ds.FromEnvironment()}
	return workflow.ConnectionForDevice(ctx, d)
}
func opsJSON(v any) string {
	encoded, e := models.OperationsValue(v)
	if e == nil {
		if text, ok := encoded.(string); ok {
			return text
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func opsHash(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func sourceHash(d delivery.DeviceSnapshot, c delivery.Connection) string {
	return opsHash(struct {
		Device            delivery.DeviceSnapshot
		Version           int64
		Profile, Protocol string
		Port              int
	}{d, c.Version, c.Profile, c.Protocol, c.Port})
}
func targetRecord(d delivery.DeviceSnapshot) vxlan.Record {
	return vxlan.Record{"id": d.ID, "ip": d.IP, "name": d.Name, "hostname": d.Hostname}
}
func validObservation(o vxlan.Observation, dimension string, started, finished time.Time) bool {
	if o.Name != dimension || o.CompletedAt == nil || o.CompletedAt.Before(started.Add(-time.Second)) || o.CompletedAt.After(finished.Add(time.Second)) {
		return false
	}
	if o.StartedAt != nil && (o.StartedAt.After(*o.CompletedAt) || o.StartedAt.Before(started.Add(-time.Second))) {
		return false
	}
	switch o.Status {
	case "complete", "partial", "failed", "unsupported", "not_requested":
		return true
	}
	return false
}
