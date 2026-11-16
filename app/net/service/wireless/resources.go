package wireless

import (
	"context"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

// Resources always resolves the owning controller through the caller's data scope.
func (s *Wireless) Resources(ctx context.Context, controller int, kind string, req *dto.PageReq, p *actions.DataPermission) ([]models.Resource, int64, error) {
	if !resourceKinds[kind] || page(req) != nil {
		return nil, 0, InvalidErr
	}
	if _, err := s.controller(ctx, controller, p); err != nil {
		return nil, 0, err
	}
	if req.Sort != "" && req.Sort != "id" {
		return nil, 0, InvalidErr
	}
	return resourcePage(s.db(ctx).Model(&models.Resource{}).Where("controller_id = ? AND kind = ?", controller, kind), req)
}
