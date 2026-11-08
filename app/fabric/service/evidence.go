package service

import (
	"context"
	"go-admin/app/fabric/models"
)

// evidenceRows loads at most two observations per dimension and device, even
// after years of collection. Correlated anti-joins preserve acquisition order
// and work across the project's SQLite/MySQL/PostgreSQL databases.
func (s *Fabric) evidenceRows(ctx context.Context, fabricID int, deviceIDs []int) ([]models.Observation, error) {
	rows := []models.Observation{}
	if len(deviceIDs) == 0 {
		return rows, nil
	}
	table := models.Observation{}.TableName()
	newer := "newer.device_id = " + table + ".device_id AND newer.fabric_id = " + table + ".fabric_id AND newer.dimension = " + table + ".dimension AND (newer.observed_at > " + table + ".observed_at OR (newer.observed_at = " + table + ".observed_at AND newer.id > " + table + ".id))"
	latest := "NOT EXISTS (SELECT 1 FROM " + table + " newer WHERE " + newer + ")"
	complete := "(" + table + ".status = ? AND NOT EXISTS (SELECT 1 FROM " + table + " newer WHERE " + newer + " AND newer.status = ?))"
	err := s.Orm.WithContext(ctx).Where("fabric_id = ? AND device_id IN ?", fabricID, deviceIDs).Where("("+latest+" OR "+complete+")", "complete", "complete").Order("observed_at DESC, id DESC").Find(&rows).Error
	return rows, err
}
