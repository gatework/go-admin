package errors_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	errs "github.com/gatework/motor/common/errors"
)

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var tokenExpiredErr = errs.New(errs.KindUnauthenticated, "auth.token_expired", "token is expired")

func TestError_reportsWhatItWasBuiltWith(t *testing.T) {
	t.Parallel()
	cause := errors.New("dial tcp 10.0.0.7:6379: connection refused")
	err := errs.Wrap(cause, errs.KindUnavailable, "cache.unreachable", "please try again later")

	if err.Kind() != errs.KindUnavailable || err.Code() != "cache.unreachable" {
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
	other := errs.New(errs.KindUnauthenticated, "auth.token_missing", "token is expired")
	if errors.Is(other, tokenExpiredErr) {
		t.Error("errors with different codes matched")
	}
	if errors.Is(errs.New(errs.KindInternal, "", "a"), errs.New(errs.KindInternal, "", "a")) {
		t.Error("errors without a code matched each other")
	}
}

func TestError_nilAndUnsetAreInternal(t *testing.T) {
	t.Parallel()
	var missing *errs.Error
	if missing.Kind() != errs.KindInternal || missing.Message() != "internal error" || missing.Code() != "" {
		t.Errorf("nil error: kind %v message %q code %q", missing.Kind(), missing.Message(), missing.Code())
	}
	if missing.Error() == "" || missing.Unwrap() != nil || missing.Is(tokenExpiredErr) {
		t.Error("nil error methods must be inert")
	}
	if got := missing.WithCause(io.EOF); !errors.Is(got, io.EOF) || got.Kind() != errs.KindInternal {
		t.Errorf("WithCause on nil = %v", got)
	}
	if got := errs.New(errs.KindUnset, "x", "").Kind(); got != errs.KindInternal {
		t.Errorf("an unset kind reports %v, want internal", got)
	}
}

func TestKindOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want errs.Kind
	}{
		{"nil", nil, errs.KindUnset},
		{"classified", tokenExpiredErr, errs.KindUnauthenticated},
		{"classified and wrapped", fmt.Errorf("login: %w", tokenExpiredErr), errs.KindUnauthenticated},
		{"canceled", fmt.Errorf("query: %w", context.Canceled), errs.KindCanceled},
		{"deadline", context.DeadlineExceeded, errs.KindTimeout},
		{"classification outranks the cause", errs.Wrap(context.Canceled, errs.KindUnavailable, "", ""), errs.KindUnavailable},
		{"unclassified", io.ErrUnexpectedEOF, errs.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := errs.KindOf(tt.err); got != tt.want {
				t.Errorf("KindOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_neverExposesAnUnclassifiedError(t *testing.T) {
	t.Parallel()
	secret := errors.New("pq: password authentication failed for user admin")
	if got := errs.Message(secret); got != "internal error" {
		t.Errorf("Message = %q", got)
	}
	if got := errs.Message(fmt.Errorf("save: %w", tokenExpiredErr)); got != "token is expired" {
		t.Errorf("Message = %q", got)
	}
	if got := errs.Message(errs.New(errs.KindNotFound, "user.missing", "")); got != "not found" {
		t.Errorf("an empty message falls back to %q", got)
	}
	if errs.Message(nil) != "" || errs.CodeOf(nil) != "" {
		t.Error("nil must yield empty text")
	}
	if got := errs.CodeOf(fmt.Errorf("save: %w", tokenExpiredErr)); got != "auth.token_expired" {
		t.Errorf("CodeOf = %q", got)
	}
}

func TestKind_statusAndTextRoundTrip(t *testing.T) {
	t.Parallel()
	statuses := map[errs.Kind]int{
		errs.KindInvalid:         http.StatusBadRequest,
		errs.KindUnauthenticated: http.StatusUnauthorized,
		errs.KindForbidden:       http.StatusForbidden,
		errs.KindNotFound:        http.StatusNotFound,
		errs.KindConflict:        http.StatusConflict,
		errs.KindTimeout:         http.StatusGatewayTimeout,
		errs.KindCanceled:        499,
		errs.KindUnavailable:     http.StatusServiceUnavailable,
		errs.KindInternal:        http.StatusInternalServerError,
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
		var decoded errs.Kind
		if err := decoded.UnmarshalText(text); err != nil || decoded != kind {
			t.Errorf("%q decoded to %v, %v", text, decoded, err)
		}
	}
	for _, invalid := range []errs.Kind{errs.KindUnset, errs.Kind(200)} {
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
	kind := errs.KindForbidden
	if err := kind.UnmarshalText([]byte("unset")); err == nil || kind != errs.KindForbidden {
		t.Errorf("decoding an invalid name gave %v, %v", kind, err)
	}
}

func FuzzKind_UnmarshalText(f *testing.F) {
	for _, seed := range []string{"invalid", "not_found", "", "unset", "INTERNAL", "kind(3)"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		var kind errs.Kind
		if err := kind.UnmarshalText([]byte(text)); err != nil {
			if kind != errs.KindUnset {
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
	errNotFound := errs.New(errs.KindNotFound, "user.not_found", "user does not exist")

	lookup := func(id int) error {
		return fmt.Errorf("lookup %d: %w", id, errNotFound.WithCause(errors.New("sql: no rows in result set")))
	}

	err := lookup(7)
	fmt.Println(errors.Is(err, errNotFound))
	fmt.Println(errs.KindOf(err), errs.KindOf(err).HTTPStatus())
	fmt.Println(errs.Message(err))
	// Output:
	// true
	// not_found 404
	// user does not exist
}
