package service

import (
	"context"
	"errors"
	"sort"

	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/network"
	"go-admin/app/network/grouping"
	"go-admin/app/network/models"
	"go-admin/app/network/service/dto"
	"gorm.io/gorm"
)

type DeviceGroup struct{ base.Service }

func (s *DeviceGroup) Tree(ctx context.Context, p *actions.DataPermission) (dto.DeviceGroupTree, error) {
	return s.tree(ctx, p, "")
}

// TreeForDeviceType retains only ancestors of visible devices of the given type.
// The inventory tree intentionally keeps empty groups for group administration.
func (s *DeviceGroup) TreeForDeviceType(ctx context.Context, p *actions.DataPermission, deviceType string) (dto.DeviceGroupTree, error) {
	if deviceType == "" {
		return dto.DeviceGroupTree{Items: []dto.DeviceGroupNode{}}, network.ErrInvalidInput
	}
	return s.tree(ctx, p, deviceType)
}

func (s *DeviceGroup) tree(ctx context.Context, p *actions.DataPermission, deviceType string) (dto.DeviceGroupTree, error) {
	result := dto.DeviceGroupTree{Items: []dto.DeviceGroupNode{}}
	groups, err := grouping.Load(s.Orm.WithContext(ctx))
	if err != nil {
		return result, err
	}
	var counts []struct {
		GroupID *int
		Count   int64
	}
	device := Device{Service: s.Service}
	query := device.scoped(ctx, p)
	if deviceType != "" {
		query = query.Where("device_type = ?", deviceType)
	}
	if err = query.Select("group_id, count(*) AS count").Group("group_id").Scan(&counts).Error; err != nil {
		return result, err
	}
	direct := map[int]int64{}
	for _, count := range counts {
		result.TotalDeviceCount += count.Count
		if count.GroupID == nil {
			result.UngroupedDeviceCount += count.Count
		} else {
			direct[*count.GroupID] = count.Count
		}
	}
	children := map[int][]models.DeviceGroup{}
	for _, group := range groups {
		parent := 0
		if group.ParentID != nil {
			parent = *group.ParentID
		}
		children[parent] = append(children[parent], group)
	}
	var build func(int, map[int]bool) []dto.DeviceGroupNode
	build = func(parent int, path map[int]bool) []dto.DeviceGroupNode {
		nodes := []dto.DeviceGroupNode{}
		for _, group := range children[parent] {
			if path[group.Id] {
				continue
			}
			path[group.Id] = true
			node := dto.DeviceGroupNode{DeviceGroup: group, DirectDeviceCount: direct[group.Id], DeviceCount: direct[group.Id], Children: build(group.Id, path)}
			delete(path, group.Id)
			for _, child := range node.Children {
				node.DeviceCount += child.DeviceCount
			}
			if deviceType == "" || node.DeviceCount > 0 {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	result.Items = build(0, map[int]bool{})
	return result, nil
}

func (s *DeviceGroup) Create(ctx context.Context, r *dto.DeviceGroupReq, actor int) (dto.DeviceGroupNode, error) {
	result := dto.DeviceGroupNode{Children: []dto.DeviceGroupNode{}}
	if actor <= 0 {
		return result, network.ErrInvalidInput
	}
	if err := r.Validate(false); err != nil {
		return result, err
	}
	err := s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		if err := validateGroupParent(tx, 0, r.ParentID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.DeviceGroup{}).Where("code = ?", r.Code).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return network.ErrGroupDuplicate
		}
		row := models.DeviceGroup{Code: r.Code, Name: r.Name, ParentID: r.ParentID, Description: r.Description, Enabled: r.Enabled, Position: r.Position, Version: 1}
		row.SetCreateBy(actor)
		row.SetUpdateBy(actor)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		result.DeviceGroup = row
		return nil
	})
	return result, err
}
func (s *DeviceGroup) Update(ctx context.Context, id int, r *dto.DeviceGroupReq, actor int) (dto.DeviceGroupNode, error) {
	result := dto.DeviceGroupNode{Children: []dto.DeviceGroupNode{}}
	if actor <= 0 || id <= 0 {
		return result, network.ErrInvalidInput
	}
	if err := r.Validate(true); err != nil {
		return result, err
	}
	err := s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		row, err := loadGroup(tx, id)
		if err != nil {
			return err
		}
		if row.Version != r.Version {
			return network.ErrConflict
		}
		if row.Code != r.Code {
			return network.ErrInvalidInput
		}
		if err := validateGroupParent(tx, id, r.ParentID); err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"name": r.Name, "parent_id": r.ParentID, "description": r.Description, "enabled": r.Enabled, "position": r.Position, "version": gorm.Expr("version + 1"), "update_by": actor}).Error; err != nil {
			return err
		}
		row, err = loadGroup(tx, id)
		result.DeviceGroup = row
		return err
	})
	return result, err
}
func (s *DeviceGroup) Delete(ctx context.Context, id int, version int64, actor int) error {
	if id <= 0 || version <= 0 || actor <= 0 {
		return network.ErrInvalidInput
	}
	return s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		row, err := loadGroup(tx, id)
		if err != nil {
			return err
		}
		if row.Version != version {
			return network.ErrConflict
		}
		var count int64
		if err := tx.Model(&models.DeviceGroup{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return network.ErrGroupNotEmpty
		}
		// Include retired/soft-deleted history: no membership may dangle.
		if err := tx.Unscoped().Model(&models.Device{}).Where("group_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return network.ErrGroupNotEmpty
		}
		return tx.Delete(&row).Error
	})
}
func loadGroup(tx *gorm.DB, id int) (models.DeviceGroup, error) {
	var row models.DeviceGroup
	err := tx.First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = network.ErrGroupNotFound
	}
	return row, err
}
func validateGroupParent(tx *gorm.DB, id int, parent *int) error {
	seen := map[int]bool{}
	for parent != nil {
		if *parent == id || seen[*parent] {
			return network.ErrInvalidInput
		}
		seen[*parent] = true
		row, err := loadGroup(tx, *parent)
		if err != nil {
			return network.ErrInvalidInput
		}
		parent = row.ParentID
	}
	return nil
}

// groupScope expands the taxonomy once per list/export and never broadens an
// unknown positive ID to all inventory. Counts reuse the same device scope.
func groupScope(tx *gorm.DB, id *int) (func(*gorm.DB) *gorm.DB, error) {
	if id == nil {
		return func(db *gorm.DB) *gorm.DB { return db }, nil
	}
	if *id == 0 {
		return func(db *gorm.DB) *gorm.DB { return db.Where("group_id IS NULL") }, nil
	}
	groups, err := grouping.Load(tx)
	if err != nil {
		return nil, err
	}
	children := map[int][]int{}
	exists := false
	for _, group := range groups {
		if group.Id == *id {
			exists = true
		}
		if group.ParentID != nil {
			children[*group.ParentID] = append(children[*group.ParentID], group.Id)
		}
	}
	ids := []int{}
	seen := map[int]bool{}
	var visit func(int)
	visit = func(current int) {
		if seen[current] {
			return
		}
		seen[current] = true
		ids = append(ids, current)
		for _, child := range children[current] {
			visit(child)
		}
	}
	if exists {
		visit(*id)
	}
	return func(db *gorm.DB) *gorm.DB {
		if len(ids) == 0 {
			return db.Where("1 = 0")
		}
		return db.Where("group_id IN ?", ids)
	}, nil
}
func populateGroupNames(tx *gorm.DB, rows []models.Device) error {
	ids := []int{}
	for _, row := range rows {
		if row.GroupID != nil {
			ids = append(ids, *row.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var groups []models.DeviceGroup
	if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return err
	}
	names := map[int]string{}
	for _, group := range groups {
		names[group.Id] = group.Name
	}
	for i := range rows {
		if rows[i].GroupID != nil {
			rows[i].Group = names[*rows[i].GroupID]
		}
	}
	return nil
}
func resolveMembership(groups grouping.Directory, row models.Device, id *int, source string) (*int, error) {
	if source == "auto" {
		return groups.Resolve(row.Name, row.Serial, row.GroupID), nil
	}
	if id == nil {
		return nil, nil
	}
	for _, group := range groups {
		if group.Id == *id {
			if !group.Enabled && !grouping.SameID(row.GroupID, id) {
				return nil, network.ErrInvalidInput
			}
			return id, nil
		}
	}
	return nil, network.ErrInvalidInput
}
func (s *Device) AssignGroup(ctx context.Context, r *dto.DeviceGroupAssignReq, actor int, p *actions.DataPermission) (int, error) {
	if actor <= 0 {
		return 0, network.ErrInvalidInput
	}
	if err := r.Validate(); err != nil {
		return 0, err
	}
	err := s.Orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		scoped := Device{Service: base.Service{Orm: tx}}
		ids := make([]int, 0, len(r.Devices))
		versions := map[int]int64{}
		for _, row := range r.Devices {
			ids = append(ids, row.ID)
			versions[row.ID] = row.Version
		}
		sort.Ints(ids)
		var rows []models.Device
		if err := scoped.scoped(ctx, p).Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return network.ErrNotFound
		}
		for _, row := range rows {
			if row.Version != versions[row.Id] {
				return network.ErrConflict
			}
		}
		groups, err := grouping.Load(tx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			id, err := resolveMembership(groups, row, r.GroupID, r.GroupSource)
			if err != nil {
				return err
			}
			result := scoped.scoped(ctx, p).Where("id = ? AND version = ?", row.Id, row.Version).Updates(map[string]any{"group_id": id, "group_source": r.GroupSource, "version": gorm.Expr("version + 1"), "update_by": actor})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return network.ErrConflict
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(r.Devices), nil
}
