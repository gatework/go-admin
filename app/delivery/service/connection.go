package service

import (
	"context"
	"errors"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/delivery/models"
	"go-admin/app/delivery/service/dto"
	"go-admin/app/network/connection"
	inventory "go-admin/app/network/models"
	network "go-admin/app/network/service"
	ndto "go-admin/app/network/service/dto"
	"gorm.io/gorm"
	"os/exec"
)

func (s *Workflow) connection(ctx context.Context, d inventory.Device) (models.Connection, error) {
	c, e := connection.Load(ctx, s.Orm, d.Id)
	if e != nil {
		return c, e
	}
	return s.effectiveConnection(d, c), nil
}

func (s *Workflow) effectiveConnection(d inventory.Device, c models.Connection) models.Connection {
	c.InferredProfile = inferProfile(d)
	c.ProfileSource = "manual"
	c.Ready, c.Reason = false, ""
	if c.Profile == "" {
		c.Profile, c.ProfileSource = c.InferredProfile, "automatic"
	}
	if _, err := connectorProfile(c.Profile); err != nil {
		c.ProfileSource, c.Reason = "unknown", "profile_unknown"
		return c
	}
	c.Ready = s.Config.configured(c.Profile)
	if !c.Ready {
		c.Reason = "credentials_missing"
	}
	if c.Ready {
		if _, err := exec.LookPath(c.Protocol); err != nil {
			c.Ready = false
			c.Reason = "transport_unavailable"
		}
	}
	if d.Status != "active" {
		c.Ready, c.Reason = false, "device_unavailable"
	}
	return c
}
func (s *Workflow) Connection(ctx context.Context, id int, p *actions.DataPermission) (models.Connection, error) {
	var d inventory.Device
	if e := s.inventory(ctx, p).First(&d, id).Error; e != nil {
		return models.Connection{}, ErrNotFound
	}
	return s.connection(ctx, d)
}
func (s *Workflow) SetConnection(ctx context.Context, id int, r dto.Connection, actor int, p *actions.DataPermission) (models.Connection, error) {
	if r.Profile != "" {
		if _, e := connectorProfile(r.Profile); e != nil {
			return models.Connection{}, ErrInvalid
		}
	}
	if (r.Protocol != "ssh" && r.Protocol != "telnet") || r.Port < 1 || r.Port > 65535 || r.Version < 0 {
		return models.Connection{}, ErrInvalid
	}
	c, e := s.Connection(ctx, id, p)
	if e != nil {
		return c, e
	}
	if c.Version != r.Version {
		return c, ErrConflict
	}
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := connection.Save(ctx, tx, id, r, actor, p); e != nil {
			switch {
			case errors.Is(e, connection.ErrConflict):
				return ErrConflict
			case errors.Is(e, connection.ErrNotFound):
				return ErrNotFound
			case errors.Is(e, connection.ErrInvalid):
				return ErrInvalid
			default:
				return e
			}
		}
		return tx.Create(&models.Audit{DeviceID: id, ActorID: actor, Action: "connection"}).Error
	})
	if e != nil {
		return c, e
	}
	return s.Connection(ctx, id, p)
}

type DeliveryDevice struct {
	inventory.Device
	Connection models.Connection `json:"connection"`
}

func (s *Workflow) Devices(ctx context.Context, r *ndto.DevicePageReq, p *actions.DataPermission) ([]DeliveryDevice, int64, error) {
	n := network.Device{Service: s.Service}
	rows, count, e := n.Page(ctx, r, p)
	if e != nil {
		return nil, 0, e
	}
	out := make([]DeliveryDevice, 0, len(rows))
	connections := make(map[int]models.Connection, len(rows))
	if len(rows) > 0 {
		ids := make([]int, 0, len(rows))
		for _, d := range rows {
			ids = append(ids, d.Id)
		}
		var stored []models.Connection
		if e := s.db(ctx).Where("device_id IN ?", ids).Find(&stored).Error; e != nil {
			return nil, 0, e
		}
		for _, c := range stored {
			connections[c.DeviceID] = c
		}
	}
	for _, d := range rows {
		c, ok := connections[d.Id]
		if !ok {
			c = models.Connection{DeviceID: d.Id, Protocol: "ssh", Port: 22}
		}
		out = append(out, DeliveryDevice{Device: d, Connection: s.effectiveConnection(d, c)})
	}
	return out, count, nil
}
