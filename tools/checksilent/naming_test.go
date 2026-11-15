package main

import "testing"

func TestSentinelErrorNames(t *testing.T) {
	root := fixture(t, map[string]string{"errors.go": `package sample
import (e "errors"; "fmt")
var ErrInvalid = e.New("invalid")
var invalid = fmt.Errorf("invalid")
var InvalidErr = e.New("invalid")
var err error
func f() { err := e.New("local"); _ = err }
`})
	findings := only(t, check(t, root, options{}), checkErrorName)
	if len(findings) != 2 {
		t.Fatalf("got %v, want two package-level naming violations", findings)
	}
}
