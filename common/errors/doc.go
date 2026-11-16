// Package errors is the error model shared by every package of this module.
//
// An Error carries three things a caller acts on: a Kind, which says what
// class of failure it is and maps to a transport status; a stable code, which
// identifies the specific condition to programs; and a message that is safe to
// show to the user. The cause, if any, stays available to errors.Is and
// errors.As and to logs, but never becomes part of the public message.
//
// Packages declare their conditions once as values and compare with errors.Is:
//
//	var TokenExpiredErr = errors.New(errors.KindUnauthenticated, "auth.token_expired", "token is expired")
//
// Import it without an alias. It also forwards the standard library's Is, As,
// AsType, Join and Unwrap, so a file never needs both errors packages; build
// an unclassified error with fmt.Errorf.
//
// An Error is immutable. WithCause and WithMessage return copies, so a
// package-level value can be shared between goroutines.
package errors

// kind.go and error.go are a verbatim copy of github.com/gatework/motor-core/errs
// (package errs there) at motor-core commit 993066d, branch
// refactor/runtime-hardening; motor cannot import that until it upgrades from
// motor-core v0.1.0. Change those two files in motor-core, not here. std.go is
// motor's own: on the upgrade, either motor-core gains the same forwarders and
// motor switches to its package, or this package keeps std.go and its copy.
