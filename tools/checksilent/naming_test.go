package main

import "testing"

func TestSentinelErrorNames(t *testing.T) {
	root := fixture(t, map[string]string{"errors.go": `package sample
import (e "errors"; "fmt"; "github.com/gatework/motor/common/errkind")
var ErrInvalid = e.New("invalid")
var conflict = errkind.New(errkind.Conflict, "sample.conflict")
var ConflictErr = errkind.New(errkind.Conflict, "sample.conflict")
var invalid = fmt.Errorf("invalid")
var InvalidErr = e.New("invalid")
var err error
func f() { err := e.New("local"); _ = err }
`})
	findings := only(t, check(t, root, options{}), checkErrorName)
	if len(findings) != 3 {
		t.Fatalf("got %v, want three package-level naming violations", findings)
	}
}
