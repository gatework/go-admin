package request

import (
	"errors"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/errkind"
)

// Fail answers err with the status of its kind and its stable code. A record
// that does not exist is NotFound. Anything unclassified is an internal
// failure: it is logged with its detail, and the client sees only
// "internal_error", never the text of a driver, file system or device error.
func Fail(a *api.API, err error) {
	kind, code := errkind.Of(err)
	if kind == errkind.Unknown && errors.Is(err, gorm.ErrRecordNotFound) {
		kind, code = errkind.NotFound, "not_found"
	}
	if kind == errkind.Unknown {
		code = "internal_error"
		if a.Logger != nil {
			a.Logger.Errorf("request failed: %v", err)
		}
	}
	a.Error(kind.Status(), nil, code)
}
