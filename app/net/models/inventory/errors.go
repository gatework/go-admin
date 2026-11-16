// Package inventory defines safe failures shared by the inventory boundaries.
package inventory

import "github.com/gatework/motor/common/errs"

// MaxExportRows bounds one CSV export; a filter matching more is refused
// rather than truncated, so a download is never silently incomplete.
const MaxExportRows = 10000

// Inventory failures. Each code is safe to persist in the sync state and to
// show: none wraps an upstream response, URL, credential or database error.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// NotConfiguredErr is a sync while the Netdisco connection is not configured.
	NotConfiguredErr = errs.New(errs.KindUnavailable, "inventory.netdisco_not_configured", "")
	// UpstreamErr is Netdisco failing to answer a request.
	UpstreamErr = errs.New(errs.KindUnavailable, "inventory.netdisco_unavailable", "")
	// AuthenticationErr is Netdisco refusing the configured credentials.
	AuthenticationErr = errs.New(errs.KindUnavailable, "inventory.netdisco_authentication_failed", "")
	// InvalidInventoryErr is Netdisco returning data the sync will not write.
	InvalidInventoryErr = errs.New(errs.KindUnavailable, "inventory.netdisco_invalid_data", "")
	// TimeoutErr is a sync that timed out or was cancelled; the stored
	// inventory is unchanged.
	TimeoutErr = errs.New(errs.KindTimeout, "inventory.sync_timeout", "")
	// RunningErr is a sync requested while another one holds the sync lease.
	RunningErr = errs.New(errs.KindConflict, "inventory.sync_running", "")
	// InterruptedErr is a sync whose lease expired before it finished.
	InterruptedErr = errs.New(errs.KindConflict, "inventory.sync_interrupted", "")
	// SyncScopeErr is a sync by a caller without the all-data scope it needs.
	SyncScopeErr = errs.New(errs.KindForbidden, "inventory.sync_scope_required", "")
	// NotFoundErr is a device that does not exist or is out of scope.
	NotFoundErr = errs.New(errs.KindNotFound, "inventory.device_not_found", "")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = errs.New(errs.KindConflict, "inventory.version_conflict", "")
	// PersistenceErr is a write that failed; the stored inventory is unchanged.
	// It is an internal failure: the HTTP answer is internal_error.
	PersistenceErr = errs.New(errs.KindInternal, "inventory.persistence_failed", "")
	// InvalidErr is a request the service refuses as malformed.
	InvalidErr = errs.New(errs.KindInvalid, "inventory.invalid_input", "")
	// ExportLimitErr is a device or circuit export over MaxExportRows rows.
	ExportLimitErr = errs.New(errs.KindInvalid, "inventory.export_limit", "")
	// GroupNotFoundErr is a device group that does not exist.
	GroupNotFoundErr = errs.New(errs.KindNotFound, "inventory.group_not_found", "")
	// GroupNotEmptyErr is a delete of a group that still has children or devices.
	GroupNotEmptyErr = errs.New(errs.KindConflict, "inventory.group_not_empty", "")
	// GroupDuplicateErr is a group code that is already taken.
	GroupDuplicateErr = errs.New(errs.KindConflict, "inventory.group_duplicate_code", "")
)

//revive:enable:error-naming
