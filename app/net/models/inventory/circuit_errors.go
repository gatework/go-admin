package inventory

import "github.com/gatework/motor/common/errkind"

// Circuit failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// CircuitNotFoundErr is a circuit that does not exist or is out of scope.
	CircuitNotFoundErr = errkind.New(errkind.NotFound, "inventory.circuit_not_found")
	// CircuitDuplicateErr is a circuit number already in use. It is a
	// validation failure the user corrects and retries, unlike ConflictErr,
	// which asks them to reload.
	CircuitDuplicateErr = errkind.New(errkind.Invalid, "inventory.circuit_duplicate_number")
	// CircuitLifecycleErr is a status change the lifecycle forbids: a
	// terminated circuit cannot be revived, and only planned circuits can be
	// deleted.
	CircuitLifecycleErr = errkind.New(errkind.Invalid, "inventory.circuit_lifecycle")
	// CircuitScopeErr is a caller without the circuit data scope.
	CircuitScopeErr = errkind.New(errkind.Forbidden, "inventory.circuit_scope_required")
	// CircuitDeviceErr is an endpoint device that does not exist or is out of scope.
	CircuitDeviceErr = errkind.New(errkind.Invalid, "inventory.circuit_device_invalid")
	// CircuitBindingErr is a monitoring binding to a host or item that is not
	// an enabled rate item.
	CircuitBindingErr = errkind.New(errkind.Invalid, "inventory.circuit_binding_invalid")
	// ZabbixNotConfiguredErr is a traffic query while Zabbix is not configured.
	ZabbixNotConfiguredErr = errkind.New(errkind.Unavailable, "inventory.zabbix_not_configured")
	// ZabbixUpstreamErr is Zabbix failing to answer a query.
	ZabbixUpstreamErr = errkind.New(errkind.Upstream, "inventory.zabbix_unavailable")
	// ZabbixTimeoutErr is a Zabbix query that timed out or was cancelled.
	ZabbixTimeoutErr = errkind.New(errkind.Timeout, "inventory.zabbix_timeout")
)

//revive:enable:error-naming
