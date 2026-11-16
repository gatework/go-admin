package wireless

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	wireless "github.com/gatework/go-wireless"
	models "github.com/gatework/motor/app/net/models/wireless"
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
		return InvalidErr
	}
	if err := validateDimensions(snapshot, started, finished); err != nil {
		return err
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
			if err := persistDimension(tx, id, sourceHash, name, dimensions[name], apNames); err != nil {
				return err
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
	evidence, err := lldpEvidence(o, selected)
	if err != nil {
		return err
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
			return InvalidErr
		}
		seen[name] = true
		if err := projectAPLLDP(tx, &aps[matches[0]], neighbors, controller, sourceHash, version, o, ev); err != nil {
			return err
		}
	}
	return nil
}

// Collection owns observed fields only. Never write back a previously read
// management draft, association, or optimistic version.
func saveFacts(tx *gorm.DB, row *models.Resource) error {
	row.Search = searchOf(row.Facts)
	if row.ID == 0 {
		return tx.Create(row).Error
	}
	columns := append([]string{"facts", "source_hash", "observation_version", "observed_at", "missing", "parent_status", "parent_resource_id"}, searchColumns...)
	return tx.Model(row).Select(columns).Updates(row).Error
}

func validateDimensions(snapshot wireless.Snapshot, started, finished time.Time) error {
	for name, o := range snapshot.Dimensions() {
		if o.Status() == wireless.NotRequested {
			continue
		}
		if name != o.Name() || o.CompletedAt() == nil || o.CompletedAt().Before(started.Add(-time.Second)) || o.CompletedAt().After(finished.Add(time.Second)) {
			return InvalidErr
		}
		if o.StartedAt() != nil && (o.StartedAt().Before(started.Add(-time.Second)) || o.StartedAt().After(*o.CompletedAt())) {
			return InvalidErr
		}
		if resourceKinds[string(name)] {
			seen := map[string]bool{}
			for _, r := range o.Records() {
				key := identity(string(name), r)
				if key == "" || seen[key] {
					return InvalidErr
				}
				seen[key] = true
			}
		}
	}
	return nil
}

func persistDimension(tx *gorm.DB, id int, sourceHash string, name wireless.Dimension, o wireless.Observation, apNames []string) error {

	if o.Status() == wireless.NotRequested {
		return nil
	}
	var state models.Dimension
	e := tx.Where("controller_id = ? AND name = ?", id, name).First(&state).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	if state.ID != 0 && !o.CompletedAt().After(state.ObservedAt) {
		return ConflictErr
	}
	isNew, stored := state.ID == 0, state.Version
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
	if e := writeState(tx, &state, isNew, stored); e != nil {
		return e
	}
	if name == wireless.LLDP {
		if e := projectLLDP(tx, id, sourceHash, state.Version, o, apNames); e != nil {
			return e
		}
		return nil
	}
	if !resourceKinds[string(name)] || !o.Complete() {
		return nil
	}
	if e := tx.Model(&models.Resource{}).Where("controller_id = ? AND kind = ?", id, name).Update("missing", true).Error; e != nil {
		return e
	}
	for _, facts := range o.Records() {
		if err := persistResource(tx, id, sourceHash, name, o, facts, &state); err != nil {
			return err
		}
	}
	// Projection issues collected above belong to the version written earlier
	// in this transaction, so the row is matched on that version unchanged.
	return writeState(tx, &state, false, state.Version)
}

// writeState inserts a state row or updates the one read earlier in the same
// transaction. It replaces GORM Save, which upserts: a row deleted by another
// transaction after it was read would be silently written back. A guarded
// update reports that, and a stale version, as ConflictErr instead.
func writeState(tx *gorm.DB, state any, isNew bool, storedVersion int64) error {
	if isNew {
		return tx.Create(state).Error
	}
	result := tx.Model(state).Where("version = ?", storedVersion).Select("*").Omit("id").Updates(state)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ConflictErr
	}
	return nil
}

func persistResource(tx *gorm.DB, id int, sourceHash string, name wireless.Dimension, o wireless.Observation, facts wireless.Record, state *models.Dimension) error {
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
	return nil
}

func lldpEvidence(o wireless.Observation, selected []string) ([]wireless.Record, error) {
	evidence := o.Evidence()
	requested, reported := map[string]bool{}, map[string]bool{}
	for _, name := range selected {
		requested[name] = true
	}
	for _, ev := range evidence {
		if text(ev["kind"]) == "ap_lldp_result" {
			name := text(ev["ap_name"])
			if len(requested) > 0 && !requested[name] {
				return nil, InvalidErr
			}
			reported[name] = true
		}
	}
	for _, name := range selected {
		if !reported[name] {
			evidence = append(evidence, wireless.Record{"kind": "ap_lldp_result", "ap_name": name, "status": "failed"})
		}
	}
	return evidence, nil
}
func projectAPLLDP(tx *gorm.DB, ap *models.Resource, neighbors []models.Resource, controller int, sourceHash string, version int64, o wireless.Observation, ev wireless.Record) error {
	name, status := text(ev["ap_name"]), text(ev["status"])
	ap.NeighborStatus = status
	if status != "complete" {
		if err := tx.Model(ap).Select("NeighborStatus").Updates(ap).Error; err != nil {
			return err
		}
		if err := persistNeighbor(tx, *ap, sourceHash, o, version, status, nil); err != nil {
			return err
		}
		return nil
	}
	facts := []wireless.Record{}
	for _, r := range o.Records() {
		if text(r["ap_name"]) == name {
			facts = append(facts, r)
		}
	}
	count, err := strconv.Atoi(text(ev["neighbor_count"]))
	if err != nil || count != len(facts) {
		return InvalidErr
	}
	for i := range neighbors {
		if text(neighbors[i].Facts["ap_name"]) == name {
			neighbors[i].Missing = true
			if err := tx.Model(&neighbors[i]).Select("Missing").Updates(&neighbors[i]).Error; err != nil {
				return err
			}
		}
	}
	if err := saveLLDPFacts(tx, ap, controller, sourceHash, version, o, facts); err != nil {
		return err
	}
	ap.NeighborEvidenceVersion = version
	ap.NeighborObservedAt = o.CompletedAt()
	if err := tx.Model(ap).Select("NeighborStatus", "NeighborEvidenceVersion", "NeighborObservedAt").Updates(ap).Error; err != nil {
		return err
	}
	if err := persistNeighbor(tx, *ap, sourceHash, o, version, status, facts); err != nil {
		return err
	}
	return nil
}
func saveLLDPFacts(tx *gorm.DB, ap *models.Resource, controller int, sourceHash string, version int64, o wireless.Observation, facts []wireless.Record) error {
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
	return nil
}
