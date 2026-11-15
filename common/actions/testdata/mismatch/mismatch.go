// Package mismatch pairs a model with another model's request, which must
// not compile. TestGenericActionsRefuseAMismatchedPair builds it and expects
// the build to fail; testdata keeps it out of ./... .
package mismatch

import (
	"github.com/gatework/motor/app/demo/models"
	jobdto "github.com/gatework/motor/app/jobs/service/dto"
	"github.com/gatework/motor/common/actions"
)

var _ = actions.Create[models.DemoProduct, jobdto.SysJobControl]
