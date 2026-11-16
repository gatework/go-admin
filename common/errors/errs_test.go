package errors_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/gatework/motor/common/errors"
)

//revive:disable-next-line:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var tokenExpiredErr = errors.New(errors.KindUnauthenticated, "auth.token_expired", "token is expired")

func TestError_reportsWhatItWasBuiltWith(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("dial tcp 10.0.0.7:6379: connection refused")
	err := errors.Wrap(cause, errors.KindUnavailable, "cache.unreachable", "please try again later")

	if err.Kind() != errors.KindUnavailable || err.Code() != "cache.unreachable" {
		t.Errorf("kind %v code %q", err.Kind(), err.Code())
	}
	if got := err.Message(); got != "please try again later" {
		t.Errorf("Message = %q; the cause must not reach the user", got)
	}
	if got, want := err.Error(), "cache.unreachable: please try again later: "+cause.Error(); got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Error("the cause is not reachable through errors.Is")
	}
}

func TestError_matchesItsSentinelAfterBeingCopied(t *testing.T) {
	t.Parallel()
	derived := tokenExpiredErr.WithCause(io.ErrUnexpectedEOF).WithMessage("please sign in again")
	wrapped := fmt.Errorf("refresh: %w", derived)

	if !errors.Is(wrapped, tokenExpiredErr) {
		t.Error("a copy no longer matches the sentinel it came from")
	}
	if !errors.Is(wrapped, io.ErrUnexpectedEOF) {
		t.Error("the cause added by WithCause is not reachable")
	}
	if tokenExpiredErr.Unwrap() != nil || tokenExpiredErr.Message() != "token is expired" {
		t.Error("WithCause or WithMessage modified the sentinel")
	}
	other := errors.New(errors.KindUnauthenticated, "auth.token_missing", "token is expired")
	if errors.Is(other, tokenExpiredErr) {
		t.Error("errors with different codes matched")
	}
	if errors.Is(errors.New(errors.KindInternal, "", "a"), errors.New(errors.KindInternal, "", "a")) {
		t.Error("errors without a code matched each other")
	}
}

func TestError_nilAndUnsetAreInternal(t *testing.T) {
	t.Parallel()
	var missing *errors.Error
	if missing.Kind() != errors.KindInternal || missing.Message() != "internal error" || missing.Code() != "" {
		t.Errorf("nil error: kind %v message %q code %q", missing.Kind(), missing.Message(), missing.Code())
	}
	if missing.Error() == "" || missing.Unwrap() != nil || missing.Is(tokenExpiredErr) {
		t.Error("nil error methods must be inert")
	}
	if got := missing.WithCause(io.EOF); !errors.Is(got, io.EOF) || got.Kind() != errors.KindInternal {
		t.Errorf("WithCause on nil = %v", got)
	}
	if got := errors.New(errors.KindUnset, "x", "").Kind(); got != errors.KindInternal {
		t.Errorf("an unset kind reports %v, want internal", got)
	}
}

func TestKindOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want errors.Kind
	}{
		{"nil", nil, errors.KindUnset},
		{"classified", tokenExpiredErr, errors.KindUnauthenticated},
		{"classified and wrapped", fmt.Errorf("login: %w", tokenExpiredErr), errors.KindUnauthenticated},
		{"canceled", fmt.Errorf("query: %w", context.Canceled), errors.KindCanceled},
		{"deadline", context.DeadlineExceeded, errors.KindTimeout},
		{"classification outranks the cause", errors.Wrap(context.Canceled, errors.KindUnavailable, "", ""), errors.KindUnavailable},
		{"unclassified", io.ErrUnexpectedEOF, errors.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := errors.KindOf(tt.err); got != tt.want {
				t.Errorf("KindOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_neverExposesAnUnclassifiedError(t *testing.T) {
	t.Parallel()
	secret := fmt.Errorf("pq: password authentication failed for user admin")
	if got := errors.Message(secret); got != "internal error" {
		t.Errorf("Message = %q", got)
	}
	if got := errors.Message(fmt.Errorf("save: %w", tokenExpiredErr)); got != "token is expired" {
		t.Errorf("Message = %q", got)
	}
	if got := errors.Message(errors.New(errors.KindNotFound, "user.missing", "")); got != "not found" {
		t.Errorf("an empty message falls back to %q", got)
	}
	if errors.Message(nil) != "" || errors.CodeOf(nil) != "" {
		t.Error("nil must yield empty text")
	}
	if got := errors.CodeOf(fmt.Errorf("save: %w", tokenExpiredErr)); got != "auth.token_expired" {
		t.Errorf("CodeOf = %q", got)
	}
}

func TestKind_statusAndTextRoundTrip(t *testing.T) {
	t.Parallel()
	statuses := map[errors.Kind]int{
		errors.KindInvalid:         http.StatusBadRequest,
		errors.KindUnauthenticated: http.StatusUnauthorized,
		errors.KindForbidden:       http.StatusForbidden,
		errors.KindNotFound:        http.StatusNotFound,
		errors.KindConflict:        http.StatusConflict,
		errors.KindTimeout:         http.StatusGatewayTimeout,
		errors.KindCanceled:        499,
		errors.KindUnavailable:     http.StatusServiceUnavailable,
		errors.KindInternal:        http.StatusInternalServerError,
	}
	for kind, status := range statuses {
		if !kind.Valid() {
			t.Errorf("%v is not valid", kind)
		}
		if got := kind.HTTPStatus(); got != status {
			t.Errorf("%v.HTTPStatus = %d, want %d", kind, got, status)
		}
		text, err := kind.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var decoded errors.Kind
		if err := decoded.UnmarshalText(text); err != nil || decoded != kind {
			t.Errorf("%q decoded to %v, %v", text, decoded, err)
		}
	}
	for _, invalid := range []errors.Kind{errors.KindUnset, errors.Kind(200)} {
		if invalid.Valid() {
			t.Errorf("%v reports valid", invalid)
		}
		if _, err := invalid.MarshalText(); err == nil {
			t.Errorf("%v was encoded", invalid)
		}
		if invalid.HTTPStatus() != http.StatusInternalServerError {
			t.Errorf("%v does not map to 500", invalid)
		}
	}
	kind := errors.KindForbidden
	if err := kind.UnmarshalText([]byte("unset")); err == nil || kind != errors.KindForbidden {
		t.Errorf("decoding an invalid name gave %v, %v", kind, err)
	}
}

func FuzzKind_UnmarshalText(f *testing.F) {
	for _, seed := range []string{"invalid", "not_found", "", "unset", "INTERNAL", "kind(3)"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		var kind errors.Kind
		if err := kind.UnmarshalText([]byte(text)); err != nil {
			if kind != errors.KindUnset {
				t.Fatalf("a failed decode left %v behind", kind)
			}
			return
		}
		if !kind.Valid() || kind.String() != text {
			t.Fatalf("%q decoded to %v", text, kind)
		}
	})
}

func ExampleError() {
	errNotFound := errors.New(errors.KindNotFound, "user.not_found", "user does not exist")

	lookup := func(id int) error {
		return fmt.Errorf("lookup %d: %w", id, errNotFound.WithCause(fmt.Errorf("sql: no rows in result set")))
	}

	err := lookup(7)
	fmt.Println(errors.Is(err, errNotFound))
	fmt.Println(errors.KindOf(err), errors.KindOf(err).HTTPStatus())
	fmt.Println(errors.Message(err))
	// Output:
	// true
	// not_found 404
	// user does not exist
}
