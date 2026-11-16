package main

import "testing"

// Classified sentinels match by code, so two declarations of one code are one
// error: errors.Is cannot tell them apart.
func TestSentinelCodeDeclaredTwice(t *testing.T) {
	root := fixture(t, map[string]string{
		"a/errors.go": `package a
import errs "github.com/gatework/motor/common/errors"
var InvalidErr = errs.New(errs.KindInvalid, "sample.invalid", "")
var BudgetErr = errs.New(errs.KindInvalid, "sample.invalid", "")
var ConflictErr = errs.New(errs.KindConflict, "sample.conflict", "")
var AliasErr = ConflictErr
`,
		"b/errors.go": `package b
import errs "github.com/gatework/motor/common/errors"
var OtherErr = errs.New(errs.KindInvalid, "sample.invalid", "")
`,
		"a/errors_test.go": `package a
import errs "github.com/gatework/motor/common/errors"
var sampleErr = errs.New(errs.KindInvalid, "sample.conflict", "")
`,
	})
	findings := only(t, check(t, root, options{}), checkErrorCodeReused)
	if len(findings) != 2 {
		t.Fatalf("got %v, want the two later declarations of sample.invalid", findings)
	}
}
