package fabric

import (
	"context"

	"gorm.io/gorm"

	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	"github.com/gatework/motor/common/actions"
)

// Historical operation scopes are immutable target snapshots, so today's Fabric
// membership cannot filter them in SQL. Restricted callers scan stable ID batches
// and retain only their requested page; unscoped callers use DB count/pagination.
func historicalPage[T any](ctx context.Context, s *Fabric, fabric int, req *dto.PageReq, permission *actions.DataPermission, rowID func(T) int, scopeIDs func(T) []int) ([]T, int64, error) {
	out := []T{}
	queryForTasks := func() *gorm.DB {
		query := opsDB(ctx, s).Model(new(T)).Where("fabric_id = ?", fabric)
		if req.Status == "active" {
			return query.Where("status IN ?", []string{"queued", "running"})
		}
		if req.Status != "" {
			return query.Where("status = ?", req.Status)
		}
		return query
	}
	if actions.FullScope(permission) {
		var count int64
		if err := queryForTasks().Count(&count).Error; err != nil {
			return nil, 0, err
		}
		err := queryForTasks().Order("id DESC").Scopes(coredto.Paginate(req.PageSize, req.PageIndex)).Find(&out).Error
		return out, count, err
	}
	var deviceIDs []int
	if err := deviceScope(opsDB(ctx, s), permission).Pluck("id", &deviceIDs).Error; err != nil {
		return nil, 0, err
	}
	visible := make(map[int]bool, len(deviceIDs))
	for _, id := range deviceIDs {
		visible[id] = true
	}
	start := int64(req.PageIndex-1) * int64(req.PageSize)
	var count int64
	cursor := 0
	for {
		batch := []T{}
		query := queryForTasks().Order("id DESC").Limit(100)
		if cursor > 0 {
			query = query.Where("id < ?", cursor)
		}
		if err := query.Find(&batch).Error; err != nil {
			return nil, 0, err
		}
		if len(batch) == 0 {
			break
		}
		out, count = appendHistoricalPage(out, count, start, req.PageSize, batch, visible, scopeIDs)
		cursor = rowID(batch[len(batch)-1])
		if len(batch) < 100 {
			break
		}
	}
	return out, count, nil
}

func appendHistoricalPage[T any](out []T, count, start int64, pageSize int, batch []T, visible map[int]bool, scopeIDs func(T) []int) ([]T, int64) {
	for _, row := range batch {
		ids := scopeIDs(row)
		allowed := len(ids) > 0
		for _, id := range ids {
			if !visible[id] {
				allowed = false
				break
			}
		}
		if !allowed {
			continue
		}
		if count >= start && len(out) < pageSize {
			out = append(out, row)
		}
		count++
	}
	return out, count
}
