package errors

import "errors"

// The standard library's error functions, so a file that uses this package
// needs no second errors import (and no alias to tell the two apart).

// Is reports whether any error in err's tree matches target; see errors.Is.
// An *Error matches another with the same code.
func Is(err, target error) bool { return errors.Is(err, target) }

// As finds the first error in err's tree that matches target; see errors.As.
func As(err error, target any) bool { return errors.As(err, target) }

// AsType finds the first error in err's tree of type E; see errors.AsType.
func AsType[E error](err error) (E, bool) { return errors.AsType[E](err) }

// Join wraps the given errors into one; see errors.Join.
func Join(errs ...error) error { return errors.Join(errs...) }

// Unwrap returns the error err wraps, or nil; see errors.Unwrap.
func Unwrap(err error) error { return errors.Unwrap(err) }
