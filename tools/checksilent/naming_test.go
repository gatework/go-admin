package main

import "testing"

func TestSentinelErrorNames(t *testing.T) {
	root := fixture(t, map[string]string{
		"std.go": `package sample
import ("errors"; "fmt")
var ErrInvalid = errors.New("invalid")
var invalid = fmt.Errorf("invalid")
var InvalidErr = errors.New("invalid")
var err error
func f() { err := errors.New("local"); _ = err }
`,
		"classified.go": `package sample
import "github.com/gatework/motor/common/errors"
var conflict = errors.New(errors.KindConflict, "sample.conflict", "")
var ConflictErr = errors.New(errors.KindConflict, "sample.conflict", "")
`,
	})
	findings := only(t, check(t, root, options{}), checkErrorName)
	if len(findings) != 3 {
		t.Fatalf("got %v, want three package-level naming violations", findings)
	}
}
