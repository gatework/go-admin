// Package inventory defines safe failures shared by the inventory boundaries.
package inventory

import "github.com/gatework/motor/common/errkind"

// Inventory failures. Each code is safe to persist in the sync state and to
// show: none wraps an upstream response, URL, credential or database error.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// NotConfiguredErr is a sync while the Netdisco connection is not configured.
	NotConfiguredErr = errkind.New(errkind.Unavailable, "inventory.netdisco_not_configured")
	// UpstreamErr is Netdisco failing to answer a request.
	UpstreamErr = errkind.New(errkind.Upstream, "inventory.netdisco_unavailable")
	// AuthenticationErr is Netdisco refusing the configured credentials.
	AuthenticationErr = errkind.New(errkind.Upstream, "inventory.netdisco_authentication_failed")
	// InvalidInventoryErr is Netdisco returning data the sync will not write.
	InvalidInventoryErr = errkind.New(errkind.Upstream, "inventory.netdisco_invalid_data")
	// TimeoutErr is a sync that timed out or was cancelled; the stored
	// inventory is unchanged.
	TimeoutErr = errkind.New(errkind.Timeout, "inventory.sync_timeout")
	// RunningErr is a sync requested while another one holds the sync lease.
	RunningErr = errkind.New(errkind.Conflict, "inventory.sync_running")
	// InterruptedErr is a sync whose lease expired before it finished.
	InterruptedErr = errkind.New(errkind.Conflict, "inventory.sync_interrupted")
	// SyncScopeErr is a sync by a caller without the all-data scope it needs.
	SyncScopeErr = errkind.New(errkind.Forbidden, "inventory.sync_scope_required")
	// NotFoundErr is a device that does not exist or is out of scope.
	NotFoundErr = errkind.New(errkind.NotFound, "inventory.device_not_found")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = errkind.New(errkind.Conflict, "inventory.version_conflict")
	// PersistenceErr is a write that failed; the stored inventory is unchanged.
	// It is an internal failure: the HTTP answer is internal_error.
	PersistenceErr = errkind.New(errkind.Unknown, "inventory.persistence_failed")
	// InvalidInputErr is a request the service refuses as malformed.
	InvalidInputErr = errkind.New(errkind.Invalid, "inventory.invalid_input")
	// GroupNotFoundErr is a device group that does not exist.
	GroupNotFoundErr = errkind.New(errkind.NotFound, "inventory.group_not_found")
	// GroupNotEmptyErr is a delete of a group that still has children or devices.
	GroupNotEmptyErr = errkind.New(errkind.Conflict, "inventory.group_not_empty")
	// GroupDuplicateErr is a group code that is already taken.
	GroupDuplicateErr = errkind.New(errkind.Conflict, "inventory.group_duplicate_code")
)

//revive:enable:error-naming
