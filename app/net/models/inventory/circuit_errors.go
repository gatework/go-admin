package inventory

import "github.com/gatework/motor/common/errors"

// Circuit failures.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// CircuitNotFoundErr is a circuit that does not exist or is out of scope.
	CircuitNotFoundErr = errors.New(errors.KindNotFound, "inventory.circuit_not_found", "")
	// CircuitDuplicateErr is a circuit number already in use. It is a
	// validation failure the user corrects and retries, unlike ConflictErr,
	// which asks them to reload.
	CircuitDuplicateErr = errors.New(errors.KindInvalid, "inventory.circuit_duplicate_number", "")
	// CircuitLifecycleErr is a status change the lifecycle forbids: a
	// terminated circuit cannot be revived, and only planned circuits can be
	// deleted.
	CircuitLifecycleErr = errors.New(errors.KindInvalid, "inventory.circuit_lifecycle", "")
	// CircuitScopeErr is a caller without the circuit data scope.
	CircuitScopeErr = errors.New(errors.KindForbidden, "inventory.circuit_scope_required", "")
	// CircuitDeviceErr is an endpoint device that does not exist or is out of scope.
	CircuitDeviceErr = errors.New(errors.KindInvalid, "inventory.circuit_device_invalid", "")
	// CircuitBindingErr is a monitoring binding to a host or item that is not
	// an enabled rate item.
	CircuitBindingErr = errors.New(errors.KindInvalid, "inventory.circuit_binding_invalid", "")
	// ZabbixNotConfiguredErr is a traffic query while Zabbix is not configured.
	ZabbixNotConfiguredErr = errors.New(errors.KindUnavailable, "inventory.zabbix_not_configured", "")
	// ZabbixUpstreamErr is Zabbix failing to answer a query.
	ZabbixUpstreamErr = errors.New(errors.KindUnavailable, "inventory.zabbix_unavailable", "")
	// ZabbixTimeoutErr is a Zabbix query that timed out or was cancelled.
	ZabbixTimeoutErr = errors.New(errors.KindTimeout, "inventory.zabbix_timeout", "")
)

//revive:enable:error-naming
