package types

import errs "github.com/gatework/motor/common/errors"

// The firewall conditions every layer reports - engine, storage and service.
// Each is declared once: sentinels match by code, so a second declaration of
// the same code would be the same error under another name.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is input the firewall workbench refuses as malformed.
	InvalidErr = errs.New(errs.KindInvalid, "firewall.invalid_input", "")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = errs.New(errs.KindConflict, "firewall.version_conflict", "")
)

//revive:enable:error-naming
