package errs

import (
	"fmt"
	"net/http"
)

// Kind classifies a failure by what the caller can do about it.
type Kind uint8

// The kinds. KindUnset is the zero value and is not a valid classification.
const (
	KindUnset Kind = iota
	// KindInvalid: the request is malformed or fails validation.
	KindInvalid
	// KindUnauthenticated: the caller has not proved who it is.
	KindUnauthenticated
	// KindForbidden: the caller is known and not allowed.
	KindForbidden
	// KindNotFound: the thing addressed does not exist.
	KindNotFound
	// KindConflict: the request contradicts current state.
	KindConflict
	// KindTimeout: a deadline passed before the work finished.
	KindTimeout
	// KindCanceled: the caller gave up.
	KindCanceled
	// KindUnavailable: a dependency cannot serve right now; retrying may help.
	KindUnavailable
	// KindInternal: a defect or an unclassified failure.
	KindInternal
)

// statusClientClosedRequest is the de facto status for a request its client
// abandoned. net/http has no constant for it.
const statusClientClosedRequest = 499

// Valid reports whether k is one of the declared kinds other than KindUnset.
func (k Kind) Valid() bool { return k > KindUnset && k <= KindInternal }

// String returns the lower-case name used in logs and in text encoding.
func (k Kind) String() string {
	switch k {
	case KindUnset:
		return "unset"
	case KindInvalid:
		return "invalid"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindTimeout:
		return "timeout"
	case KindCanceled:
		return "canceled"
	case KindUnavailable:
		return "unavailable"
	case KindInternal:
		return "internal"
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// HTTPStatus returns the status code that reports k over HTTP. An unset or
// unknown kind is an internal error.
func (k Kind) HTTPStatus() int {
	switch k {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthenticated:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindTimeout:
		return http.StatusGatewayTimeout
	case KindCanceled:
		return statusClientClosedRequest
	case KindUnavailable:
		return http.StatusServiceUnavailable
	case KindUnset, KindInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

// MarshalText encodes a valid kind as its name.
func (k Kind) MarshalText() ([]byte, error) {
	if !k.Valid() {
		return nil, fmt.Errorf("errs: cannot encode %s", k)
	}
	return []byte(k.String()), nil
}

// UnmarshalText decodes a kind name and rejects anything else, leaving k
// unchanged on failure.
func (k *Kind) UnmarshalText(text []byte) error {
	for candidate := KindInvalid; candidate <= KindInternal; candidate++ {
		if candidate.String() == string(text) {
			*k = candidate
			return nil
		}
	}
	return fmt.Errorf("errs: unknown kind %q", text)
}

// publicMessage is what a caller is told when an error has no message of its
// own that is safe to show.
func (k Kind) publicMessage() string {
	switch k {
	case KindInvalid:
		return "invalid request"
	case KindUnauthenticated:
		return "authentication required"
	case KindForbidden:
		return "permission denied"
	case KindNotFound:
		return "not found"
	case KindConflict:
		return "conflict"
	case KindTimeout:
		return "request timed out"
	case KindCanceled:
		return "request canceled"
	case KindUnavailable:
		return "service unavailable"
	case KindUnset, KindInternal:
		return "internal error"
	}
	return "internal error"
}
