package fabric

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func (w *Worker) schedule(ctx context.Context, now time.Time) error {
	var identities []network.Device
	if e := w.s.db(ctx).Select("id", "name", "device_role").Find(&identities).Error; e != nil {
		return e
	}
	if e := w.s.reconcile(ctx, identities); e != nil {
		return e
	}
	var fabrics []models.Fabric
	if e := w.s.db(ctx).Where("collection_enabled = ?", true).Find(&fabrics).Error; e != nil {
		return e
	}
	for _, f := range fabrics {
		if e := w.scheduleFabric(ctx, f, now); e != nil {
			return e
		}
	}
	return nil
}

func (w *Worker) scheduleFabric(ctx context.Context, f models.Fabric, now time.Time) error {
	var pending int64
	if e := w.s.db(ctx).Model(&models.Collection{}).Where("fabric_id = ? AND status IN ?", f.ID, []string{"queued", "running"}).Count(&pending).Error; e != nil {
		return e
	}
	if pending > 0 {
		return nil
	}
	var devices []network.Device
	ids := w.s.db(ctx).Model(&models.Membership{}).Select("device_id").Where("fabric_id = ? AND status = ?", f.ID, "assigned")
	if e := w.s.db(ctx).Where("id IN (?) AND status IN ?", ids, []string{"pending", "active"}).Order("id").Find(&devices).Error; e != nil {
		return e
	}
	if len(devices) == 0 {
		return nil
	}
	for _, mode := range []string{"configuration", "runtime"} {
		if e := w.scheduleMode(ctx, f, devices, mode, now); e != nil {
			return e
		}
	}

	return nil
}

func (w *Worker) scheduleMode(ctx context.Context, f models.Fabric, devices []network.Device, mode string, now time.Time) error {
	minutes := f.RuntimeMinutes
	if mode == "configuration" {
		minutes = f.ConfigMinutes
	}
	if minutes <= 0 {
		if mode == "configuration" {
			minutes = 30
		} else {
			minutes = 5
		}
	}
	var recent models.Collection
	e := w.s.db(ctx).Where("fabric_id = ? AND mode IN ?", f.ID, []string{mode, "all"}).Order("created_at DESC").First(&recent).Error
	if e == nil && now.Sub(recent.CreatedAt) < time.Duration(minutes)*time.Minute {
		return nil
	}
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	slot := now.Unix() / int64(minutes*60)
	key := fmt.Sprintf("schedule:%d:%s:%d", f.ID, mode, slot)
	if _, e := w.s.createCollection(ctx, f.ID, &dto.CollectionCreateReq{RequestKey: key, Mode: mode}, 0, devices); e != nil {
		return e
	}
	return nil
}
