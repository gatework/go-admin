// Package netconfig loads the network capabilities' server-owned configuration
// once, at startup: device credentials and delivery settings, Netdisco and
// Zabbix. Everything else receives it through constructors and handler fields,
// so no request or worker tick reads the environment.
package netconfig

import (
	"github.com/gatework/motor/app/net/service/delivery"
	"github.com/gatework/motor/app/net/service/inventory/provider/netdisco"
	"github.com/gatework/motor/app/net/service/inventory/provider/zabbix"
)

// Config is the network capabilities' configuration.
type Config struct {
	// Delivery holds delivery's switches and the device access every
	// capability uses (Delivery.Access).
	Delivery delivery.Config
	Netdisco netdisco.Config
	Zabbix   zabbix.Config
}

// Load reads the configuration from the backend environment.
func Load() Config {
	return Config{Delivery: delivery.FromEnvironment(), Netdisco: netdisco.FromEnvironment(), Zabbix: zabbix.FromEnvironment()}
}
