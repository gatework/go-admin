package errors

import (
	"context"
	"errors"
)

// Error is a classified failure. The zero value is not usable; construct one
// with New or Wrap. Methods are safe on a nil *Error and report an internal
// error, so a nil that escaped as a non-nil error interface does not panic.
type Error struct {
	kind  Kind
	code  string
	msg   string
	cause error
}

// New returns an Error of the given kind. code identifies the condition to
// programs and should be stable, lower-case and dotted ("auth.token_expired").
// msg is shown to users, so it must not contain internal detail.
func New(kind Kind, code, msg string) *Error {
	return &Error{kind: kind, code: code, msg: msg}
}

// Wrap returns an Error that classifies cause. The cause is reachable through
// errors.Is and errors.As and appears in Error for logs, but not in Message.
func Wrap(cause error, kind Kind, code, msg string) *Error {
	return &Error{kind: kind, code: code, msg: msg, cause: cause}
}

// Error returns the code, the message and the cause, for logs. Use Message
// for text shown to a user.
func (e *Error) Error() string {
	if e == nil {
		return KindInternal.publicMessage()
	}
	text := e.Message()
	if e.code != "" {
		text = e.code + ": " + text
	}
	if e.cause != nil {
		text += ": " + e.cause.Error()
	}
	return text
}

// Kind returns the classification, or KindInternal when none was set.
func (e *Error) Kind() Kind {
	if e == nil || !e.kind.Valid() {
		return KindInternal
	}
	return e.kind
}

// Code returns the stable identifier of the condition, or "".
func (e *Error) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

// Message returns text that is safe to show to a user. It never includes the
// cause.
func (e *Error) Message() string {
	if e == nil || e.msg == "" {
		return e.Kind().publicMessage()
	}
	return e.msg
}

// Unwrap returns the cause, or nil.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is reports whether target is an Error with the same non-empty code, which is
// what makes a package-level value work as a sentinel after WithCause or
// WithMessage produced a copy of it.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok || e == nil || other == nil {
		return false
	}
	return e.code != "" && e.code == other.code
}

// WithCause returns a copy of e that wraps cause. errors.Is still matches the
// original.
func (e *Error) WithCause(cause error) *Error {
	if e == nil {
		return Wrap(cause, KindInternal, "", "")
	}
	clone := *e
	clone.cause = cause
	return &clone
}

// WithMessage returns a copy of e with a different user-visible message.
func (e *Error) WithMessage(msg string) *Error {
	if e == nil {
		return New(KindInternal, "", msg)
	}
	clone := *e
	clone.msg = msg
	return &clone
}

// KindOf classifies any error. A nil error is KindUnset. An Error anywhere in
// the chain decides; otherwise context cancellation and deadline are
// recognised, and everything else is internal.
func KindOf(err error) Kind {
	if err == nil {
		return KindUnset
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind()
	}
	switch {
	case errors.Is(err, context.Canceled):
		return KindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	}
	return KindInternal
}

// CodeOf returns the code of the first Error in err's chain, or "".
func CodeOf(err error) string {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code()
	}
	return ""
}

// Message returns the text to show a user for any error: the message of the
// first Error in the chain, or a generic sentence for its kind. The text of an
// unclassified error is never returned, because it may carry internal detail.
func Message(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Message()
	}
	return KindOf(err).publicMessage()
}
