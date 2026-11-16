package request

import (
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/errors"
)

// Fail answers err with the HTTP status of its kind and its stable code. A
// record that does not exist is not_found and a broken unique key is
// duplicate. An internal failure - anything unclassified, or classified as
// internal - is logged with its detail, and the client sees only
// "internal_error", never the text of a driver, file system or device error.
func Fail(a *api.API, err error) {
	if errors.CodeOf(err) == "" {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = errors.Wrap(err, errors.KindNotFound, "not_found", "")
		case errors.Is(err, gorm.ErrDuplicatedKey):
			err = errors.Wrap(err, errors.KindConflict, "duplicate", "")
		}
	}
	kind, code := errors.KindOf(err), errors.CodeOf(err)
	switch {
	case kind == errors.KindInternal:
		code = "internal_error"
		if a.Logger != nil {
			a.Logger.Errorf("request failed: %v", err)
		}
	case code == "":
		// A context deadline or cancellation carries a kind but no code.
		code = kind.String()
	}
	a.Error(kind.HTTPStatus(), nil, code)
}
