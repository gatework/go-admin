package fabric

import (
	"context"
	"errors"
	"regexp"
	"sort"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/fabric"
	networkmodels "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func ruleCandidates(name string, rules []models.Rule) []int {
	seen := map[int]bool{}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		compiled, err := regexp.Compile("(?i)" + r.Pattern)
		if err == nil && compiled.MatchString(name) {
			seen[r.FabricID] = true
		}
	}
	out := []int{}
	for id := range seen {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// ResolveMembership provides the resolve membership operation within the caller's authorized fabric scope.
func ResolveMembership(name string, m models.Membership, rules []models.Rule, version int64) models.Membership {
	m.NameSnapshot = name
	m.RuleVersion = version
	if m.Mode == "manual" {
		if m.FabricID == nil {
			m.Status = "unmatched"
		} else {
			m.Status = "assigned"
		}
		return m
	}
	m.Mode = "auto"
	m.FabricID = nil
	ids := ruleCandidates(name, rules)
	switch len(ids) {
	case 0:
		m.Status = "unmatched"
	case 1:
		m.Status = "assigned"
		id := ids[0]
		m.FabricID = &id
	default:
		m.Status = "conflict"
	}
	return m
}

// Rules provides the rules operation within the caller's authorized fabric scope.
func (s *Fabric) Rules(ctx context.Context) (models.RuleSet, error) {
	var row models.RuleSet
	err := s.DB.WithContext(ctx).First(&row, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, NotFoundErr
	}
	return row, err
}

// PreviewRules evaluates rules without applying device changes.
func (s *Fabric) PreviewRules(ctx context.Context, r *dto.RulesReq, p *actions.DataPermission) ([]dto.Member, error) {
	if err := s.validateRules(ctx, r.Rules); err != nil {
		return nil, err
	}
	devices, err := s.visibleDevices(ctx, p)
	if err != nil {
		return nil, err
	}
	out := []dto.Member{}
	for _, d := range devices {
		var m models.Membership
		err := s.DB.WithContext(ctx).First(&m, "device_id = ?", d.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		m = resolveDeviceMembership(d, m, r.Rules, r.Version)
		m.DeviceID = d.ID
		out = append(out, dto.Member{Membership: m, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Candidates: ruleCandidates(d.Name, r.Rules)})
	}
	return out, nil
}

// UpdateRules updates rules after checking ownership and version constraints.
func (s *Fabric) UpdateRules(ctx context.Context, r *dto.RulesReq, actor int, p *actions.DataPermission) (models.RuleSet, error) {
	var out models.RuleSet
	if actor <= 0 || r.Version <= 0 {
		return out, InvalidErr
	}
	if err := s.validateRules(ctx, r.Rules); err != nil {
		return out, err
	}
	var total, visible int64
	if err := s.DB.WithContext(ctx).Model(&networkmodels.Device{}).Count(&total).Error; err != nil {
		return out, err
	}
	if err := deviceScope(s.DB.WithContext(ctx), p).Count(&visible).Error; err != nil {
		return out, err
	}
	if total != visible {
		return out, ScopeErr
	}
	out = models.RuleSet{ID: 1, Version: r.Version + 1, Rules: r.Rules, UpdateBy: actor}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.RuleSet{}).Where("id = 1 AND version = ?", r.Version).Select("version", "rules", "update_by").Updates(out)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ConflictErr
		}
		return tx.Model(&models.Fabric{}).Where("1 = 1").Updates(map[string]any{"rules_version": out.Version, "version": gorm.Expr("version + 1")}).Error
	})
	return out, err
}
func (s *Fabric) validateRules(ctx context.Context, rules []models.Rule) error {
	if len(rules) > 100 {
		return InvalidErr
	}
	for _, r := range rules {
		if r.FabricID <= 0 || r.Pattern == "" || !validText(r.Pattern, 256) {
			return InvalidErr
		}
		if _, err := regexp.Compile("(?i)" + r.Pattern); err != nil {
			return InvalidErr
		}
		var n int64
		if err := s.DB.WithContext(ctx).Model(&models.Fabric{}).Where("id = ?", r.FabricID).Count(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return InvalidErr
		}
	}
	return nil
}

func fabricCandidateRole(role string) bool {
	switch role {
	case "leaf_switch", "spine_switch", "border_switch":
		return true
	}
	return false
}
func sameFabric(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func resolveDeviceMembership(d networkmodels.Device, m models.Membership, rules []models.Rule, version int64) models.Membership {
	m = ResolveMembership(d.Name, m, rules, version)
	if m.Mode != "manual" && !fabricCandidateRole(d.DeviceRole) {
		m.FabricID = nil
		m.Status = "unmatched"
	}
	return m
}
