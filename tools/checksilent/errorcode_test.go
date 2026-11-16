package main

import "testing"

// Classified sentinels match by code, so two declarations of one code are one
// error: errors.Is cannot tell them apart.
func TestSentinelCodeDeclaredTwice(t *testing.T) {
	root := fixture(t, map[string]string{
		"a/errors.go": `package a
import "github.com/gatework/motor/common/errors"
var InvalidErr = errors.New(errors.KindInvalid, "sample.invalid", "")
var BudgetErr = errors.New(errors.KindInvalid, "sample.invalid", "")
var ConflictErr = errors.New(errors.KindConflict, "sample.conflict", "")
var AliasErr = ConflictErr
`,
		"b/errors.go": `package b
import "github.com/gatework/motor/common/errors"
var OtherErr = errors.New(errors.KindInvalid, "sample.invalid", "")
`,
		"a/errors_test.go": `package a
import "github.com/gatework/motor/common/errors"
var sampleErr = errors.New(errors.KindInvalid, "sample.conflict", "")
`,
	})
	findings := only(t, check(t, root, options{}), checkErrorCodeReused)
	if len(findings) != 2 {
		t.Fatalf("got %v, want the two later declarations of sample.invalid", findings)
	}
}
