package fabric

import "github.com/gatework/motor/common/errors"

// InvalidErr is a Fabric request refused as malformed. It is declared here,
// where both the request types and the service can reach it; sentinels match
// by code, so it must be declared only once.
//
//revive:disable-next-line:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
var InvalidErr = errors.New(errors.KindInvalid, "fabric.invalid_input", "") //nolint:errname // Project sentinel errors use Err suffix.
