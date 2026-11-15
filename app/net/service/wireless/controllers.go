package wireless

import (
	"context"

	"github.com/gatework/motor-core/sdk/contract/actions"
	network "github.com/gatework/motor/app/net/models/inventory"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/app/net/service/inventory"
)

// Controllers provides the controllers operation within the caller's authorized wireless scope.
func (s *Wireless) Controllers(ctx context.Context, req *dto.PageReq, p *actions.DataPermission) ([]network.Device, int64, error) {
	if page(req) != nil || req.Sort != "" && req.Sort != "updatedAt" || req.Direction != "" && req.Direction != "desc" {
		return nil, 0, InvalidErr
	}
	service := inventory.Device{Service: s.Service}
	query := ndto.DevicePageReq{PageIndex: req.PageIndex, PageSize: req.PageSize, DeviceType: "wireless_controller", Keyword: req.Keyword, Status: req.Status, GroupID: req.GroupID}
	return service.Page(ctx, &query, p)
}
