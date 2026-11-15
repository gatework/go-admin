// Package matched is mismatch with the pair corrected: the control that
// shows mismatch fails for the pairing, not for how it is built.
package matched

import (
	"github.com/gatework/motor/app/demo/models"
	"github.com/gatework/motor/app/demo/service/dto"
	"github.com/gatework/motor/common/actions"
)

var _ = actions.Create[models.DemoProduct, dto.DemoProductControl]
