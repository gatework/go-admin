// Package matched is mismatch with the pair corrected: the control that
// shows mismatch fails for the pairing, not for how it is built.
package matched

import (
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/actions/testdata/fixture"
)

var _ = actions.Create[fixture.Widget, fixture.WidgetControl]
