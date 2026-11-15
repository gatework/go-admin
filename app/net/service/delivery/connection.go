package delivery

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/connection"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/delivery"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	network "github.com/gatework/motor/app/net/service/inventory"
)

// Connection provides the connection operation within the caller's authorized delivery scope.
func (s *Workflow) Connection(ctx context.Context, id int, p *actions.DataPermission) (models.Connection, error) {
	var d inventory.Device
	if e := s.inventory(ctx, p).First(&d, id).Error; errors.Is(e, gorm.ErrRecordNotFound) {
		return models.Connection{}, NotFoundErr
	} else if e != nil {
		return models.Connection{}, e
	}
	return s.connection(ctx, d)
}

// SetConnection persists the selected connection settings.
func (s *Workflow) SetConnection(ctx context.Context, id int, r dto.Connection, actor int, p *actions.DataPermission) (models.Connection, error) {
	if r.Profile != "" {
		if _, e := deviceaccess.ConnectorProfile(r.Profile); e != nil {
			return models.Connection{}, InvalidErr
		}
	}
	if (r.Protocol != "ssh" && r.Protocol != "telnet") || r.Port < 1 || r.Port > 65535 || r.Version < 0 {
		return models.Connection{}, InvalidErr
	}
	c, e := s.Connection(ctx, id, p)
	if e != nil {
		return c, e
	}
	if c.Version != r.Version {
		return c, ConflictErr
	}
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := connection.Save(ctx, tx, id, r, actor, p); e != nil {
			switch {
			case errors.Is(e, connection.ConflictErr):
				return ConflictErr
			case errors.Is(e, connection.NotFoundErr):
				return NotFoundErr
			case errors.Is(e, connection.InvalidErr):
				return InvalidErr
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

// Devices provides the devices operation within the caller's authorized delivery scope.
func (s *Workflow) Devices(ctx context.Context, r *ndto.DevicePageReq, p *actions.DataPermission) ([]Device, int64, error) {
	n := network.Device{Service: s.Service}
	rows, count, e := n.Page(ctx, r, p)
	if e != nil {
		return nil, 0, e
	}
	ids := make([]int, 0, len(rows))
	for _, d := range rows {
		ids = append(ids, d.ID)
	}
	connections, e := connection.LoadMany(ctx, s.db(ctx), ids)
	if e != nil {
		return nil, 0, e
	}
	out := make([]Device, 0, len(rows))
	for _, d := range rows {
		out = append(out, Device{Device: d, Connection: s.Config.Access.Effective(d, connections[d.ID], deviceaccess.Write)})
	}
	return out, count, nil
}
func (s *Workflow) connection(ctx context.Context, d inventory.Device) (models.Connection, error) {
	return s.Config.Access.Resolve(ctx, s.DB, d, deviceaccess.Write)
}

// ConnectionForDevice resolves the connection of an inventory row already
// loaded by a scoped service or an execution-time identity check.
func (s *Workflow) ConnectionForDevice(ctx context.Context, d inventory.Device) (models.Connection, error) {
	return s.connection(ctx, d)
}

// Device describes delivery device for delivery operations.
type Device struct {
	inventory.Device
	Connection models.Connection `json:"connection"`
}
