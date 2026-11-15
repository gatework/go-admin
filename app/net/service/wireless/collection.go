package wireless

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

// CreateCollection persists a request; it does not open a device session.
func (s *Wireless) CreateCollection(ctx context.Context, controller int, req dto.CollectionReq, actor int, p *actions.DataPermission) (models.Collection, error) {
	var result models.Collection
	if actor <= 0 || len(req.RequestKey) < 8 || len(req.RequestKey) > 100 || strings.TrimSpace(req.RequestKey) != req.RequestKey {
		return result, InvalidErr
	}
	device, err := s.controller(ctx, controller, p)
	if err != nil {
		return result, err
	}
	if !deviceaccess.Read.Allows(device.Status) {
		return result, deviceaccess.UnavailableErr
	}
	conn, err := s.connection(ctx, device)
	if err != nil {
		return result, err
	}
	dimensions, err := s.dimensions(platform(conn), req.Dimensions)
	if err != nil {
		return result, err
	}
	lldp := false
	for _, d := range dimensions {
		if d == wireless.LLDP {
			lldp = true
		}
	}
	if lldp && len(req.APNames) == 0 || !lldp && len(req.APNames) > 0 {
		return result, InvalidErr
	}
	if len(req.APNames) > 0 {
		var aps []models.Resource
		if err = s.db(ctx).Where("controller_id = ? AND kind = ? AND missing = ?", controller, "access_points", false).Find(&aps).Error; err != nil {
			return result, err
		}
		available := map[string]int{}
		for _, ap := range aps {
			available[text(ap.Facts["name"])]++
		}
		seen := map[string]bool{}
		for _, name := range req.APNames {
			if strings.TrimSpace(name) == "" || available[name] != 1 || seen[name] {
				return result, InvalidErr
			}
			seen[name] = true
		}
	}
	payload := hash([]any{controller, dimensions, req.APNames})
	sourceHash := source(device, conn)
	result = models.Collection{ControllerID: controller, RequestKey: req.RequestKey, CreateBy: actor, PayloadHash: payload, SourceHash: sourceHash, Dimensions: dimensions, APNames: req.APNames, Status: "queued", Version: 1}
	err = s.db(ctx).Create(&result).Error
	if err == nil {
		return result, nil
	}
	var existing models.Collection
	if e := s.db(ctx).Where("create_by = ? AND request_key = ?", actor, req.RequestKey).First(&existing).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return models.Collection{}, err
		}
		return models.Collection{}, e
	}
	if existing.PayloadHash != payload {
		return models.Collection{}, ConflictErr
	}
	return existing, nil
}
