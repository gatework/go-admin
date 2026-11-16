// Package errs is the error model shared by every package of this module.
//
// An Error carries three things a caller acts on: a Kind, which says what
// class of failure it is and maps to a transport status; a stable code, which
// identifies the specific condition to programs; and a message that is safe to
// show to the user. The cause, if any, stays available to errors.Is and
// errors.As and to logs, but never becomes part of the public message.
//
// Packages declare their conditions once as values and compare with errors.Is:
//
//	var ErrTokenExpired = errs.New(errs.KindUnauthenticated, "auth.token_expired", "token is expired")
//
// An Error is immutable. WithCause and WithMessage return copies, so a
// package-level value can be shared between goroutines.
package errs

// This package is a verbatim copy of github.com/gatework/motor-core/errs at
// motor-core commit 993066d (branch refactor/runtime-hardening), which motor
// cannot import until it upgrades from motor-core v0.1.0. Do not change it
// here (apart from one test variable renamed to motor's Err-suffix
// convention): change motor-core, and on the upgrade replace this import path with
// motor-core's and delete the package.
