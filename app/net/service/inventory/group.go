package inventory

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/grouping"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
)

// DeviceGroup describes device group for network operations.
type DeviceGroup struct{ base.Service }

// Tree provides the tree operation within the caller's authorized network scope.
func (s *DeviceGroup) Tree(ctx context.Context, p *actions.DataPermission) (dto.DeviceGroupTree, error) {
	return s.tree(ctx, p, "")
}

// TreeForDeviceType retains only ancestors of visible devices of the given type.
// The inventory tree intentionally keeps empty groups for group administration.
func (s *DeviceGroup) TreeForDeviceType(ctx context.Context, p *actions.DataPermission, deviceType string) (dto.DeviceGroupTree, error) {
	if deviceType == "" {
		return dto.DeviceGroupTree{Items: []dto.DeviceGroupNode{}}, inventory.InvalidErr
	}
	return s.tree(ctx, p, deviceType)
}

// Create creates the requested record within the caller's authorized scope.
func (s *DeviceGroup) Create(ctx context.Context, r *dto.DeviceGroupReq, actor int) (dto.DeviceGroupNode, error) {
	result := dto.DeviceGroupNode{Children: []dto.DeviceGroupNode{}}
	if actor <= 0 {
		return result, inventory.InvalidErr
	}
	if err := r.Validate(false); err != nil {
		return result, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		if err := validateGroupParent(tx, 0, r.ParentID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&inventory.DeviceGroup{}).Where("code = ?", r.Code).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return inventory.GroupDuplicateErr
		}
		row := inventory.DeviceGroup{Code: r.Code, Name: r.Name, ParentID: r.ParentID, Description: r.Description, Enabled: r.Enabled, Position: r.Position, Version: 1}
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

// Update updates the requested record after checking ownership and version constraints.
func (s *DeviceGroup) Update(ctx context.Context, id int, r *dto.DeviceGroupReq, actor int) (dto.DeviceGroupNode, error) {
	result := dto.DeviceGroupNode{Children: []dto.DeviceGroupNode{}}
	if actor <= 0 || id <= 0 {
		return result, inventory.InvalidErr
	}
	if err := r.Validate(true); err != nil {
		return result, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		row, err := loadGroup(tx, id)
		if err != nil {
			return err
		}
		if row.Version != r.Version {
			return inventory.ConflictErr
		}
		if row.Code != r.Code {
			return inventory.InvalidErr
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

// Delete removes the selected records within the caller's authorized scope.
func (s *DeviceGroup) Delete(ctx context.Context, id int, version int64, actor int) error {
	if id <= 0 || version <= 0 || actor <= 0 {
		return inventory.InvalidErr
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		row, err := loadGroup(tx, id)
		if err != nil {
			return err
		}
		if row.Version != version {
			return inventory.ConflictErr
		}
		var count int64
		if err := tx.Model(&inventory.DeviceGroup{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return inventory.GroupNotEmptyErr
		}
		// Include retired/soft-deleted history: no membership may dangle.
		if err := tx.Unscoped().Model(&inventory.Device{}).Where("group_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return inventory.GroupNotEmptyErr
		}
		return tx.Delete(&row).Error
	})
}
func (s *DeviceGroup) tree(ctx context.Context, p *actions.DataPermission, deviceType string) (dto.DeviceGroupTree, error) {
	result := dto.DeviceGroupTree{Items: []dto.DeviceGroupNode{}}
	groups, err := grouping.Load(s.DB.WithContext(ctx))
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
	children := map[int][]inventory.DeviceGroup{}
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
			if path[group.ID] {
				continue
			}
			path[group.ID] = true
			node := dto.DeviceGroupNode{DeviceGroup: group, DirectDeviceCount: direct[group.ID], DeviceCount: direct[group.ID], Children: build(group.ID, path)}
			delete(path, group.ID)
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
func loadGroup(tx *gorm.DB, id int) (inventory.DeviceGroup, error) {
	var row inventory.DeviceGroup
	err := tx.First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = inventory.GroupNotFoundErr
	}
	return row, err
}
func validateGroupParent(tx *gorm.DB, id int, parent *int) error {
	seen := map[int]bool{}
	for parent != nil {
		if *parent == id || seen[*parent] {
			return inventory.InvalidErr
		}
		seen[*parent] = true
		row, err := loadGroup(tx, *parent)
		if err != nil {
			return inventory.InvalidErr
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
		if group.ID == *id {
			exists = true
		}
		if group.ParentID != nil {
			children[*group.ParentID] = append(children[*group.ParentID], group.ID)
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
func populateGroupNames(tx *gorm.DB, rows []inventory.Device) error {
	ids := []int{}
	for _, row := range rows {
		if row.GroupID != nil {
			ids = append(ids, *row.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var groups []inventory.DeviceGroup
	if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return err
	}
	names := map[int]string{}
	for _, group := range groups {
		names[group.ID] = group.Name
	}
	for i := range rows {
		if rows[i].GroupID != nil {
			rows[i].Group = names[*rows[i].GroupID]
		}
	}
	return nil
}

//nolint:nilnil // A nil group ID is the explicit unassigned membership value, not a missing result.
func resolveMembership(groups grouping.Directory, row inventory.Device, id *int, source string) (*int, error) {
	if source == "auto" {
		return groups.Resolve(row.Name, row.Serial, row.GroupID), nil
	}
	if id == nil {
		return nil, nil
	}
	for _, group := range groups {
		if group.ID == *id {
			if !group.Enabled && !grouping.SameID(row.GroupID, id) {
				return nil, inventory.InvalidErr
			}
			return id, nil
		}
	}
	return nil, inventory.InvalidErr
}

// AssignGroup provides the assign group operation within the caller's authorized network scope.
func (s *Device) AssignGroup(ctx context.Context, r *dto.DeviceGroupAssignReq, actor int, p *actions.DataPermission) (int, error) {
	if actor <= 0 {
		return 0, inventory.InvalidErr
	}
	if err := r.Validate(); err != nil {
		return 0, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := grouping.Lock(tx); err != nil {
			return err
		}
		scoped := Device{DB: tx}
		ids := make([]int, 0, len(r.Devices))
		versions := map[int]int64{}
		for _, row := range r.Devices {
			ids = append(ids, row.ID)
			versions[row.ID] = row.Version
		}
		sort.Ints(ids)
		var rows []inventory.Device
		if err := scoped.scoped(ctx, p).Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return inventory.NotFoundErr
		}
		for _, row := range rows {
			if row.Version != versions[row.ID] {
				return inventory.ConflictErr
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
			result := scoped.scoped(ctx, p).Where("id = ? AND version = ?", row.ID, row.Version).Updates(map[string]any{"group_id": id, "group_source": r.GroupSource, "version": gorm.Expr("version + 1"), "update_by": actor})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return inventory.ConflictErr
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(r.Devices), nil
}
