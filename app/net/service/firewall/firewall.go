package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gatework/motor/common/errors"

	"gorm.io/gorm"

	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is input the workbench refuses as malformed.
	InvalidErr = types.InvalidErr
	// NotFoundErr describes the err not found used by this module.
	NotFoundErr = errors.New(errors.KindNotFound, "firewall.not_found", "")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = types.ConflictErr
	// UnavailableErr reports missing firewall evidence.
	UnavailableErr = errors.New(errors.KindUnavailable, "firewall.evidence_unavailable", "")
)

//revive:enable:error-naming

// Firewall carries firewall state for this module.
type Firewall struct {
	base.Service
	Actor int
	Root  string
}

func safeError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return NotFoundErr
	}
	return e
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validKey reports whether s is a request or domain key: 1-128 ASCII
// letters, digits, '_' or '-'.
func validKey(s string) bool   { return len(s) > 0 && len(s) <= 128 && keyPattern.MatchString(s) }
func validTitle(s string) bool { return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 255 }

func (s *Firewall) db(ctx context.Context) *gorm.DB { return s.DB.WithContext(ctx) }

// conflictOnDuplicate reports a broken unique key (one instance per device,
// one domain per code) as ConflictErr, and any other error as itself.
func conflictOnDuplicate(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ConflictErr.WithCause(err)
	}
	return err
}
