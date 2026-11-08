package service

import (
	"context"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	common "github.com/go-admin-team/go-admin-core/v2/sdk/contract/dto"
	network "go-admin/app/network/models"
	inventory "go-admin/app/network/service"
	ndto "go-admin/app/network/service/dto"
	"go-admin/app/wireless/service/dto"
)

func (s *Wireless) Controllers(ctx context.Context, req *dto.PageReq, p *actions.DataPermission) ([]network.Device, int64, error) {
	if page(req) != nil || req.Sort != "" && req.Sort != "updatedAt" || req.Direction != "" && req.Direction != "desc" {
		return nil, 0, ErrInvalid
	}
	service := inventory.Device{Service: s.Service}
	query := ndto.DevicePageReq{Pagination: common.Pagination{PageIndex: req.PageIndex, PageSize: req.PageSize}, DeviceType: "wireless_controller", Keyword: req.Keyword, Status: req.Status, GroupID: req.GroupID}
	return service.Page(ctx, &query, p)
}
