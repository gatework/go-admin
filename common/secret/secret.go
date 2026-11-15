// Package secret holds credentials so that they cannot be printed by accident.
//
// A Secret formats as "[redacted]" under every fmt verb (including %+v and %#v
// of a struct that contains one), in slog output and in JSON. Code that needs
// the value asks for it by name with Reveal, which is easy to find in review.
package secret

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

const redacted = "[redacted]"

// Secret is a credential such as a password, token or API key.
type Secret string

// Reveal returns the credential. Pass it only to the code that authenticates.
func (s Secret) Reveal() string { return string(s) }

// IsSet reports whether a credential is present.
func (s Secret) IsSet() bool { return s != "" }

// String redacts the credential.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// GoString redacts the credential under %#v.
func (s Secret) GoString() string { return fmt.Sprintf("secret.Secret(%q)", s.String()) }

// Format redacts the credential under every verb, so %v, %+v, %s, %q, %x and
// friends cannot print the raw string through reflection.
func (s Secret) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		fmt.Fprint(f, s.GoString())
		return
	}
	fmt.Fprint(f, s.String())
}

// LogValue redacts the credential in slog output.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalJSON redacts the credential in JSON.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }
