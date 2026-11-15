// Package errkind classifies application errors so that one adapter can turn
// any of them into a response.
//
// Services and models declare their sentinel errors with New, giving each a
// Kind (what went wrong, from the caller's point of view) and a stable code
// (a dotted key such as "firewall.version_conflict" that clients match on and
// translate). They never carry an HTTP status or user-facing text: the HTTP
// adapter derives the status from the Kind, and the frontend owns the wording.
//
//	var ConflictErr = errkind.New(errkind.Conflict, "firewall.version_conflict")
//	return fmt.Errorf("saving draft %d: %w", id, ConflictErr)
//
// Wrapping keeps the classification: Of finds it anywhere in the chain.
package errkind

import (
	"errors"
	"net/http"
)

// Kind is the class of an error as the caller sees it.
type Kind uint8

// Kinds. The zero value is Unknown, which is what an unclassified error is:
// an internal failure whose detail must not reach the client.
const (
	Unknown Kind = iota
	Invalid
	Unauthorized
	Forbidden
	NotFound
	Conflict
	Busy
	Unavailable
)

// Status is the HTTP status that reports this kind.
func (k Kind) Status() int {
	switch k {
	case Invalid:
		return http.StatusBadRequest
	case Unauthorized:
		return http.StatusUnauthorized
	case Forbidden:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict:
		return http.StatusConflict
	case Busy:
		return http.StatusTooManyRequests
	case Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Error is a classified sentinel. Compare with errors.Is; its identity is the
// pointer, so two sentinels with the same code are still distinct.
type Error struct {
	kind Kind
	code string
}

// New declares a classified sentinel error. code is the stable key a client
// sees, so it must not change once published.
func New(kind Kind, code string) *Error {
	return &Error{kind: kind, code: code}
}

// Error returns the stable code.
func (e *Error) Error() string { return e.code }

// Kind returns the error's class.
func (e *Error) Kind() Kind { return e.kind }

// Code returns the stable key a client sees.
func (e *Error) Code() string { return e.code }

// Of returns the classification of the first classified error in err's chain.
// An unclassified error is Unknown with an empty code.
func Of(err error) (Kind, string) {
	if classified, ok := errors.AsType[*Error](err); ok {
		return classified.kind, classified.code
	}
	return Unknown, ""
}
