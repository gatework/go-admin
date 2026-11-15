// Package mismatch pairs a model with another model's request, which must
// not compile. TestGenericActionsRefuseAMismatchedPair builds it and expects
// the build to fail; testdata keeps it out of ./... .
package mismatch

import (
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/actions/testdata/fixture"
)

var _ = actions.Create[fixture.Widget, fixture.GadgetControl]
