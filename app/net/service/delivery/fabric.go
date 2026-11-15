package delivery

import (
	"context"
	"time"

	"github.com/gatework/go-net-connector/connector"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
)

// ConnectionForDevice resolves defaults for an inventory row already loaded by
// a scoped service, or by an execution-time identity check. It performs no I/O
// against the device and never creates a connection row.
func (s *Workflow) ConnectionForDevice(ctx context.Context, d inventory.Device) (models.Connection, error) {
	return s.connection(ctx, d)
}

// ConnectionConfig shares credential precedence, host-key policy and transport
// bounds with delivery. Secrets remain in memory and are never persisted.
func (c Config) ConnectionConfig(d models.DeviceSnapshot, connection models.Connection) (connector.ConnectionConfig, error) {
	if !c.configured(connection.Profile) {
		return connector.ConnectionConfig{}, ConnectionErr
	}
	creds := c.credentials(connection.Profile)
	return connector.ConnectionConfig{Host: d.IP, Protocol: connector.Protocol(connection.Protocol), Port: connection.Port, Username: creds.Username, Password: creds.Password.Reveal(), EnablePassword: creds.EnablePassword.Reveal(), HostKeyPolicy: connector.HostKeyPolicy(c.HostKeyPolicy), KnownHosts: c.KnownHosts, LoginTimeout: 30 * time.Second, CommandTimeout: 60 * time.Second, WriteTimeout: 10 * time.Second, MaxOutputBytes: MaxOutputBytes, MaxScriptOutputBytes: MaxOutputBytes}, nil
}
