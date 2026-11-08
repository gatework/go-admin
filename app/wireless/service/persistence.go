package service

import (
	"context"
	"errors"
	wireless "github.com/gatework/go-wireless"
	"go-admin/app/wireless/models"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

var resourceKinds = map[string]bool{"access_points": true, "radios": true, "bsses": true, "clients": true, "lldp": true}

func identity(kind string, r wireless.Record) string {
	var parts []string
	switch kind {
	case "access_points":
		// The library's AP ID remains a string; the stable record uses serial/MAC
		// before a controller's mutable display name or numeric slot.
		for _, key := range []string{"serial_id", "serial_number", "serial", "mac_address", "ap_id", "name"} {
			if v := text(r[key]); v != "" {
				parts = []string{key, strings.ToLower(v)}
				break
			}
		}
	case "radios":
		parts = []string{text(r["ap_name"]), text(r["radio_id"])}
	case "bsses":
		parts = []string{text(r["ap_name"]), text(r["radio_id"]), text(r["bssid"]), text(r["ssid"])}
	case "clients":
		parts = []string{text(r["client_mac"]), text(r["ap_name"]), text(r["radio_id"]), text(r["ssid"])}
	case "lldp":
		parts = []string{text(r["ap_name"]), text(r["local_interface"]), text(r["chassis_id"]), text(r["port_id"]), text(r["system_name"]), text(r["remote_chassis_id"]), text(r["remote_interface"]), text(r["remote_name"])}
	}
	valid := false
	for _, p := range parts {
		valid = valid || p != ""
	}
	if !valid {
		return ""
	}
	return hash(parts)
}

// Persist validates controller identity, clocks, duplicates and monotonic versions
// before the transaction. Only complete observations replace authoritative facts.
func (s *Wireless) Persist(ctx context.Context, id int, sourceHash string, snapshot wireless.Snapshot, started, finished time.Time, apNames ...string) error {
	if id <= 0 || sourceHash == "" || hash(snapshot.Target()) != hash(wireless.Record{"controllerId": id}) || snapshot.ObservedAt().Before(started.Add(-time.Second)) || snapshot.ObservedAt().After(finished.Add(time.Second)) {
		return ErrInvalid
	}
	for name, o := range snapshot.Dimensions() {
		if o.Status() == wireless.NotRequested {
			continue
		}
		if name != o.Name() || o.CompletedAt() == nil || o.CompletedAt().Before(started.Add(-time.Second)) || o.CompletedAt().After(finished.Add(time.Second)) {
			return ErrInvalid
		}
		if o.StartedAt() != nil && (o.StartedAt().Before(started.Add(-time.Second)) || o.StartedAt().After(*o.CompletedAt())) {
			return ErrInvalid
		}
		if resourceKinds[string(name)] {
			seen := map[string]bool{}
			for _, r := range o.Records() {
				key := identity(string(name), r)
				if key == "" || seen[key] {
					return ErrInvalid
				}
				seen[key] = true
			}
		}
	}
	return s.db(ctx).Transaction(func(tx *gorm.DB) error {
		dimensions := snapshot.Dimensions()
		order := make([]wireless.Dimension, 0, len(dimensions))
		if _, ok := dimensions[wireless.AccessPoints]; ok {
			order = append(order, wireless.AccessPoints)
		}
		for name := range dimensions {
			if name != wireless.AccessPoints {
				order = append(order, name)
			}
		}
		for _, name := range order {
			o := dimensions[name]
			if o.Status() == wireless.NotRequested {
				continue
			}
			var state models.Dimension
			e := tx.Where("controller_id = ? AND name = ?", id, name).First(&state).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if state.ID != 0 && !o.CompletedAt().After(state.ObservedAt) {
				return ErrConflict
			}
			state.ControllerID = id
			state.Name = string(name)
			state.Version++
			state.Latest = o
			state.SourceHash = sourceHash
			state.ObservedAt = *o.CompletedAt()
			state.ProjectionStatus = string(o.Status())
			state.ProjectionIssues = []string{}
			if o.Complete() {
				state.LastComplete = &o
				state.CompleteAt = o.CompletedAt()
				state.CompleteSourceHash = sourceHash
			}
			if e := tx.Save(&state).Error; e != nil {
				return e
			}
			if name == wireless.LLDP {
				if e := projectLLDP(tx, id, sourceHash, state.Version, o, apNames); e != nil {
					return e
				}
				continue
			}
			if !resourceKinds[string(name)] || !o.Complete() {
				continue
			}
			if e := tx.Model(&models.Resource{}).Where("controller_id = ? AND kind = ?", id, name).Update("missing", true).Error; e != nil {
				return e
			}
			for _, facts := range o.Records() {
				key := identity(string(name), facts)
				var row models.Resource
				e := tx.Where("controller_id = ? AND kind = ? AND identity = ?", id, name, key).First(&row).Error
				if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				row.ControllerID = id
				row.Kind = string(name)
				row.Identity = key
				row.Facts = facts
				row.SourceHash = sourceHash
				row.ObservationVersion = state.Version
				row.ObservedAt = *o.CompletedAt()
				row.Missing = false
				if row.Version == 0 {
					row.Version = 1
				}
				row.ParentStatus = "confirmed"
				row.ParentResourceID = nil
				if name != wireless.AccessPoints {
					var n int64
					parent := text(facts["ap_name"])
					var parents []models.Resource
					if e := tx.Where("controller_id = ? AND kind = ? AND missing = ?", id, "access_points", false).Find(&parents).Error; e != nil {
						return e
					}
					for _, p := range parents {
						if text(p.Facts["name"]) == parent && parent != "" && p.SourceHash == sourceHash {
							n++
							parentID := p.ID
							row.ParentResourceID = &parentID
						}
					}
					if n != 1 {
						row.ParentResourceID = nil
						row.ParentStatus = "orphan"
						state.ProjectionStatus = "partial"
						state.ProjectionIssues = append(state.ProjectionIssues, "ap_parent_unresolved")
					}
				}
				if e := saveFacts(tx, &row); e != nil {
					return e
				}
			}
			if e := tx.Save(&state).Error; e != nil {
				return e
			}
		}
		return nil
	})
}

// Targeted LLDP can be partial overall while individual AP results are complete.
// Only a uniquely identified AP with a consistent completed record count can
// replace that AP's prior facts; unselected or failed APs retain their facts.
func projectLLDP(tx *gorm.DB, controller int, sourceHash string, version int64, o wireless.Observation, selected []string) error {
	var aps, neighbors []models.Resource
	if err := tx.Where("controller_id = ? AND kind = ? AND missing = ?", controller, "access_points", false).Find(&aps).Error; err != nil {
		return err
	}
	if err := tx.Where("controller_id = ? AND kind = ?", controller, "lldp").Find(&neighbors).Error; err != nil {
		return err
	}
	byName := map[string][]int{}
	for i, ap := range aps {
		name := text(ap.Facts["name"])
		if name != "" {
			byName[name] = append(byName[name], i)
		}
	}
	evidence := o.Evidence()
	requested, reported := map[string]bool{}, map[string]bool{}
	for _, name := range selected {
		requested[name] = true
	}
	for _, ev := range evidence {
		if text(ev["kind"]) == "ap_lldp_result" {
			name := text(ev["ap_name"])
			if len(requested) > 0 && !requested[name] {
				return ErrInvalid
			}
			reported[name] = true
		}
	}
	for _, name := range selected {
		if !reported[name] {
			evidence = append(evidence, wireless.Record{"kind": "ap_lldp_result", "ap_name": name, "status": "failed"})
		}
	}
	seen := map[string]bool{}
	for _, ev := range evidence {
		if text(ev["kind"]) != "ap_lldp_result" {
			continue
		}
		name := text(ev["ap_name"])
		matches := byName[name]
		status := text(ev["status"])
		// A retired AP remains in the ledger but can no longer authorize an
		// association. No active AP is not a successful empty LLDP result.
		if len(matches) == 0 && status != "complete" {
			continue
		}
		if name == "" || len(matches) != 1 || seen[name] {
			return ErrInvalid
		}
		seen[name] = true
		ap := &aps[matches[0]]
		ap.NeighborStatus = status
		if status != "complete" {
			if err := tx.Model(ap).Select("NeighborStatus").Updates(ap).Error; err != nil {
				return err
			}
			if err := persistNeighbor(tx, *ap, sourceHash, o, version, status, nil); err != nil {
				return err
			}
			continue
		}
		facts := []wireless.Record{}
		for _, r := range o.Records() {
			if text(r["ap_name"]) == name {
				facts = append(facts, r)
			}
		}
		count, err := strconv.Atoi(text(ev["neighbor_count"]))
		if err != nil || count != len(facts) {
			return ErrInvalid
		}
		for i := range neighbors {
			if text(neighbors[i].Facts["ap_name"]) == name {
				neighbors[i].Missing = true
				if err := tx.Model(&neighbors[i]).Select("Missing").Updates(&neighbors[i]).Error; err != nil {
					return err
				}
			}
		}
		for _, fact := range facts {
			key := identity("lldp", fact)
			var row models.Resource
			err := tx.Where("controller_id = ? AND kind = ? AND identity = ?", controller, "lldp", key).First(&row).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			row.ControllerID = controller
			row.Kind = "lldp"
			row.Identity = key
			row.Facts = fact
			row.SourceHash = sourceHash
			row.ObservationVersion = version
			row.ObservedAt = *o.CompletedAt()
			row.Missing = false
			row.ParentStatus = "confirmed"
			row.ParentResourceID = &ap.ID
			if row.Version == 0 {
				row.Version = 1
			}
			if err := saveFacts(tx, &row); err != nil {
				return err
			}
		}
		ap.NeighborEvidenceVersion = version
		ap.NeighborObservedAt = o.CompletedAt()
		if err := tx.Model(ap).Select("NeighborStatus", "NeighborEvidenceVersion", "NeighborObservedAt").Updates(ap).Error; err != nil {
			return err
		}
		if err := persistNeighbor(tx, *ap, sourceHash, o, version, status, facts); err != nil {
			return err
		}
	}
	return nil
}

// Collection owns observed fields only. Never write back a previously read
// management draft, association, or optimistic version.
func saveFacts(tx *gorm.DB, row *models.Resource) error {
	if row.ID == 0 {
		return tx.Create(row).Error
	}
	return tx.Model(row).Select("Facts", "SourceHash", "ObservationVersion", "ObservedAt", "Missing", "ParentStatus", "ParentResourceID").Updates(row).Error
}
