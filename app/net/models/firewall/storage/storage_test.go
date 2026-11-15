package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/config"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/projection"
	"github.com/gatework/firemon/vendors/fortinet"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

func setup(t *testing.T) (*storage.Store, *gorm.DB) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(models.Schema()...); e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&models.Instance{ID: 1, Name: "synthetic", Vendor: "test", Mode: "routed", Version: 1}).Error; e != nil {
		t.Fatal(e)
	}
	s, e := storage.New(db, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s, db
}
func graph(t *testing.T, incomplete bool) *fm.Firewall {
	t.Helper()
	r, _ := flow.PrefixRange(netip.MustParsePrefix("10.0.0.0/24"))
	d := fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, Addresses: []fm.AddressRecord{{ID: "a", Name: "local", Scope: "native-book", Ranges: []flow.AddressRange{r}}}, AddressGroups: []fm.AddressGroupRecord{{ID: "g", Name: "group", Members: []fm.AddressRef{{Kind: fm.AddressObject, ID: "a"}, {Kind: fm.AddressObject, ID: "missing"}}}}, Services: []fm.ServiceRecord{{ID: "s", Name: "https", Terms: []fm.ServiceTerm{{Protocol: flow.Interval{Low: 6, High: 6}, SourcePort: flow.Interval{Low: 0, High: 65535}, DestinationPort: flow.Interval{Low: 443, High: 443}}}}}}
	if !incomplete {
		d.AddressGroups[0].Members = d.AddressGroups[0].Members[:1]
	}
	fw, e := fm.Seal(d)
	if e != nil {
		t.Fatal(e)
	}
	return fw
}
func TestArchiveProjectionAndCompletePublication(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	fw := graph(t, true)
	if fw.Complete() {
		t.Fatal("incomplete fixture")
	}
	archived, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "incomplete"})
	if e != nil {
		t.Fatal(e)
	}
	if archived.Current || archived.Complete || archived.ProjectionState != "ready" {
		t.Fatalf("bad archive %+v", archived)
	}
	var i models.Instance
	db.First(&i, 1)
	if i.CurrentRevision != "" {
		t.Fatal("archive replaced current")
	}
	p, e := s.Objects(ctx, archived.ID, "address", storage.Filter{Address: "10.0.0.9"})
	if e != nil || p.Count != 1 || p.List[0].Scope != "native-book" {
		t.Fatalf("archive not projected %+v %v", p, e)
	}
	complete, e := s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "complete"})
	if e != nil {
		t.Fatal(e)
	}
	if !complete.Current || !complete.Complete {
		t.Fatal(complete)
	}
	loaded, e := s.Load(ctx, 1, archived.Revision)
	if e != nil || loaded.Complete() {
		t.Fatal("immutable archived load", e)
	}
	again, e := s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "complete"})
	if e != nil || again.ID != complete.ID {
		t.Fatal("retry not idempotent", again, e)
	}
	if _, e = s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "wrong-cas"}); e == nil {
		t.Fatal("missing CAS conflict")
	}
	if _, e = s.Load(ctx, 2, archived.Revision); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("cross instance snapshot leak", e)
	}
}
func TestReferencesFiltersAndStaleProjection(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	fw := graph(t, true)
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "archive"})
	if e != nil {
		t.Fatal(e)
	}
	groups, e := s.Objects(ctx, snap.ID, "address_group", storage.Filter{Keyword: "group"})
	if e != nil || groups.Count != 1 {
		t.Fatal(groups, e)
	}
	refs, e := s.References(ctx, snap.ID, "address_group", groups.List[0].ID)
	if e != nil || len(refs.Outgoing) != 2 || !refs.Outgoing[0].Resolved || refs.Outgoing[1].Resolved {
		t.Fatal(refs, e)
	}
	services, e := s.Objects(ctx, snap.ID, "service", storage.Filter{Protocol: "tcp", Port: "443"})
	if e != nil || services.Count != 1 {
		t.Fatal(services, e)
	}
	services, e = s.Objects(ctx, snap.ID, "service", storage.Filter{Protocol: "udp", Port: "443"})
	if e != nil || services.Count != 0 {
		t.Fatal(services, e)
	}
	complete, e := s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "complete"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(ctx, projection.Batch{Device: storage.DeviceKey(1), Revision: snap.Revision, Generation: 0, Complete: false, Firewall: fw}); e != nil {
		t.Fatal(e)
	}
	var i models.Instance
	db.First(&i, 1)
	if i.CurrentRevision != complete.Revision {
		t.Fatal("stale generation changed head")
	}
	var n int64
	db.Model(&models.Address{}).Where("snapshot_id = ?", snap.ID).Count(&n)
	if n != 1 {
		t.Fatal("retry duplicated projection", n)
	}
}

func TestIncomingReferencesAndSettingsCAS(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	snap, e := s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "one"})
	if e != nil {
		t.Fatal(e)
	}
	addresses, e := s.Objects(ctx, snap.ID, "address", storage.Filter{})
	if e != nil {
		t.Fatal(e)
	}
	refs, e := s.References(ctx, snap.ID, "address", addresses.List[0].ID)
	if e != nil || len(refs.Incoming) != 1 {
		t.Fatalf("incoming references %+v %v", refs, e)
	}
	settings, e := s.SaveSettings(ctx, 1, types.Settings{Zones: []types.Binding{{Zone: "inside", Protected: true, Networks: []string{"10.0.0.0/24"}}}, Scopes: []types.ScopeBinding{{RoutingScope: "default", DomainID: 1}}})
	if e != nil || settings.Version != 1 {
		t.Fatal(settings, e)
	}
	if _, e = s.SaveSettings(ctx, 1, types.Settings{Version: 0}); !errors.Is(e, storage.VersionErr) {
		t.Fatal("settings CAS", e)
	}
	var i models.Instance
	db.First(&i, 1)
	if i.Settings.Version != 1 || len(i.Settings.Zones) != 1 {
		t.Fatal("typed settings lost", i.Settings)
	}
	var count int64
	db.Model(&models.ZoneBinding{}).Count(&count)
	if count != 1 {
		t.Fatal("zone bindings", count)
	}
	db.Model(&models.ScopeBinding{}).Count(&count)
	if count != 1 {
		t.Fatal("scope bindings", count)
	}
	if e = s.SaveCoverage(ctx, 1, snap.Revision, 0, nil); !errors.Is(e, storage.VersionErr) {
		t.Fatal("stale coverage", e)
	}
	if _, e = s.Object(ctx, snap.ID+1, "address", addresses.List[0].ID); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("snapshot details leak", e)
	}
}

func TestNativeFortinetRoundTripAndTypedNATReferences(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	const raw = `config system interface
edit inside
set ip 10.0.0.1 255.255.255.0
next
edit outside
set ip 198.51.100.1 255.255.255.0
next
end
config firewall address
edit local
set subnet 10.0.0.0 255.255.255.0
next
end
config firewall service custom
edit HTTPS
set tcp-portrange 443:1024-65535
next
end
config firewall ippool
edit outbound
set type overload
set startip 198.51.100.100
set endip 198.51.100.110
next
end
config firewall vip
edit public
set extintf outside
set extip 198.51.100.20
set mappedip 10.0.0.20
next
end
config firewall policy
edit 10
set srcintf inside
set dstintf outside
set srcaddr local
set dstaddr all
set action accept
set schedule always
set service HTTPS
set nat enable
set ippool enable
set poolname outbound
next
end
`
	fw, e := fortinet.Parse(ctx, config.Source{Text: raw, Complete: true}, fm.Metadata{Device: storage.DeviceKey(1)})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "native"})
	if e != nil {
		t.Fatal(e)
	}
	loaded, e := s.Load(ctx, 1, snap.Revision)
	if e != nil || loaded.Export().Raw != raw || loaded.SemanticDigest() != fw.SemanticDigest() {
		t.Fatal("native immutable round trip", e)
	}
	static, e := s.Objects(ctx, snap.ID, "static_nat", storage.Filter{})
	if e != nil || static.Count < 1 {
		t.Fatal("static NAT missing", static, e)
	}
	dynamic, e := s.Objects(ctx, snap.ID, "dynamic_nat", storage.Filter{})
	if e != nil || dynamic.Count < 1 {
		t.Fatal("dynamic NAT missing", dynamic, e)
	}
	refs, e := s.References(ctx, snap.ID, "dynamic_nat", dynamic.List[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	owner, pool := false, false
	for _, r := range refs.Outgoing {
		owner = owner || (r.Role == "rule" && r.Resolved && r.TargetKind == "rule")
		pool = pool || (r.Role == "source_pool" && r.Resolved && r.TargetKind == "nat_pool")
	}
	if !owner || !pool {
		t.Fatal("typed NAT ownership/reference loss", refs)
	}
	rules, e := s.Objects(ctx, snap.ID, "rule", storage.Filter{Address: "10.0.0.128/25", Protocol: "tcp", Port: "443", Zone: "inside"})
	if e != nil || rules.Count != 1 || rules.List[0].NativeID != "10" {
		t.Fatal("combined native rule filter", rules, e)
	}
	filteredNAT, e := s.Objects(ctx, snap.ID, "static_nat", storage.Filter{Address: "198.51.100.20"})
	foundVIP := false
	for _, item := range filteredNAT.List {
		foundVIP = foundVIP || item.NativeID == "vip:public"
	}
	if e != nil || !foundVIP {
		t.Fatal("native NAT mapping address filter", filteredNAT, e)
	}
	encoded, e := json.Marshal(dynamic)
	if e != nil || strings.Contains(string(encoded), "config system interface") {
		t.Fatal("raw configuration leaked", string(encoded), e)
	}
}

func TestProjectionFailureRetainsDurableOutboxAndRollsBack(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	fw := graph(t, false)
	if e := db.Callback().Create().Before("gorm:create").Register("fail_projection", func(tx *gorm.DB) {
		if tx.Statement.Table == (models.Address{}).TableName() {
			tx.AddError(errors.New("synthetic projection outage"))
		}
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "recover"}); e == nil {
		t.Fatal("projection outage lost")
	}
	manifest, e := s.Manifest(ctx)
	if e != nil || manifest.Acknowledged["sql"] != 0 || len(manifest.Events) != 1 {
		t.Fatal("outbox acknowledged failure", manifest, e)
	}
	var count int64
	db.Model(&models.Snapshot{}).Count(&count)
	if count != 0 {
		t.Fatal("partial snapshot committed", count)
	}
	db.Model(&models.Address{}).Count(&count)
	if count != 0 {
		t.Fatal("partial address committed", count)
	}
	if e = db.Callback().Create().Remove("fail_projection"); e != nil {
		t.Fatal(e)
	}
	if e = s.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	loaded, e := s.Load(ctx, 1, manifest.Events[0].Revision)
	if e != nil || loaded.SemanticDigest() != fw.SemanticDigest() {
		t.Fatal("recovery did not use immutable graph", e)
	}
	if e = s.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	db.Model(&models.Address{}).Count(&count)
	if count != 1 {
		t.Fatal("recovery duplicated projection", count)
	}
	manifest, e = s.Manifest(ctx)
	if e != nil || manifest.Acknowledged["sql"] != manifest.Events[0].Generation {
		t.Fatal("outbox recovery not acknowledged", manifest, e)
	}
}
func TestCompleteStaleGenerationCannotReplaceCurrent(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	old := graph(t, false)
	first, e := s.Import(ctx, 1, old, storage.ImportOptions{PublicationID: "first"})
	if e != nil {
		t.Fatal(e)
	}
	changed := old.Export()
	changed.Addresses[0].Name = "changed"
	newGraph, e := fm.Seal(changed)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Import(ctx, 1, newGraph, storage.ImportOptions{ExpectedRevision: first.Revision, PublicationID: "second"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(ctx, projection.Batch{Device: storage.DeviceKey(1), Revision: first.Revision, Generation: first.Generation, Complete: true, Firewall: old}); e != nil {
		t.Fatal(e)
	}
	var instance models.Instance
	db.First(&instance, 1)
	if instance.CurrentRevision != second.Revision || instance.Generation != second.Generation {
		t.Fatal("stale generation overwrote newer head", instance)
	}
	var snapshots []models.Snapshot
	db.Where("instance_id = ? AND current = ?", 1, true).Find(&snapshots)
	if len(snapshots) != 1 || snapshots[0].ID != second.ID {
		t.Fatal("multiple or wrong current snapshots", snapshots)
	}
}

func TestAnalysisTargetUniqueIdentity(t *testing.T) {
	_, db := setup(t)
	one := models.AnalysisTarget{AnalysisID: 1, InstanceID: 2}
	if e := db.Create(&one).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Create(&models.AnalysisTarget{AnalysisID: 1, InstanceID: 2}).Error; e == nil {
		t.Fatal("duplicate analysis evidence target accepted")
	}
	if e := db.Create(&models.AnalysisTarget{AnalysisID: 2, InstanceID: 2}).Error; e != nil {
		t.Fatal("instance cannot be pinned by independent analyses", e)
	}
}

func TestStaticNATAddressFilterPreservesDeclaredMappings(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	original, _ := flow.PrefixRange(netip.MustParsePrefix("198.51.100.0/24"))
	translated, _ := flow.PrefixRange(netip.MustParsePrefix("10.0.0.0/24"))
	fw, e := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, StaticNATs: []fm.StaticNATRecord{{ID: "mapping-only", Destination: []fm.Mapping{{Original: original, Translated: translated, Mode: fm.Offset}}}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "mapping"})
	if e != nil {
		t.Fatal(e)
	}
	for _, address := range []string{"198.51.100.1", "10.0.0.1"} {
		page, e := s.Objects(ctx, snap.ID, "static_nat", storage.Filter{Address: address})
		if e != nil || page.Count != 1 {
			t.Fatal("declared NAT mapping not indexed", address, page, e)
		}
	}
}

func TestWildcardNativeMatchesAppearInBrowseFilters(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	wildcard := fm.Match{AnySource: true, AnyDestination: true, AnyService: true, AnyFromZone: true, AnyToZone: true}
	fw, e := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, Rules: []fm.RuleRecord{{ID: "wildcard", Match: wildcard, Action: fm.Permit}, {ID: "empty", Action: fm.Permit}}, StaticNATs: []fm.StaticNATRecord{{ID: "wildcard", Match: wildcard}}, DynamicNATs: []fm.DynamicNATRecord{{ID: "wildcard", Match: wildcard, SourceMode: fm.NoTranslation, DestinationMode: fm.NoTranslation}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "wildcards"})
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"rule", "static_nat", "dynamic_nat"} {
		for _, f := range []storage.Filter{{Address: "10.0.0.1"}, {Protocol: "tcp", Port: "443"}, {Address: "2001:db8::1", Protocol: "udp", Port: "53"}} {
			page, e := s.Objects(ctx, snap.ID, kind, f)
			if e != nil || page.Count != 1 || page.List[0].NativeID != "wildcard" {
				t.Fatal("explicit wildcard omitted or unknown empty match promoted", kind, f, page, e)
			}
		}
	}
}
func TestObjectDiagnosticsDoNotLeakAcrossTypedIDCollisions(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	fw, e := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, Diagnostics: []fm.Diagnostic{{Object: "same", Code: "ambiguous", Message: "untyped captured diagnostic"}}, Addresses: []fm.AddressRecord{{ID: "same", FQDN: "unresolved.example.invalid"}}, Services: []fm.ServiceRecord{{ID: "same", Terms: []fm.ServiceTerm{{Protocol: flow.Interval{Low: 6, High: 6}, DestinationPort: flow.Interval{Low: 443, High: 443}}}}}, Rules: []fm.RuleRecord{{ID: "same", Action: fm.Permit, Match: fm.Match{AnySource: true, AnyDestination: true, AnyService: true}}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "collision"})
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"address", "service", "rule"} {
		page, e := s.Objects(ctx, snap.ID, kind, storage.Filter{})
		if e != nil || page.Count != 1 {
			t.Fatal(kind, page, e)
		}
		want := "known"
		if kind == "address" {
			want = "unknown"
		}
		if page.List[0].State != want {
			t.Fatal("cross-kind diagnostic poisoning", kind, page.List[0])
		}
		for _, d := range page.List[0].Diagnostics {
			if d.Code == "ambiguous" {
				t.Fatal("ambiguous graph diagnostic must remain snapshot-level", kind, d)
			}
		}
	}
	if len(snap.Diagnostics) < 2 {
		t.Fatal("snapshot diagnostic evidence lost", snap.Diagnostics)
	}
}

func TestBrowseServiceFilterKeepsProtocolPortTermsPaired(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	fw, e := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, Services: []fm.ServiceRecord{{ID: "paired", Terms: []fm.ServiceTerm{{Protocol: flow.Interval{Low: 6, High: 6}, DestinationPort: flow.Interval{Low: 80, High: 80}}, {Protocol: flow.Interval{Low: 17, High: 17}, DestinationPort: flow.Interval{Low: 443, High: 443}}}}}, Rules: []fm.RuleRecord{{ID: "paired-rule", Action: fm.Permit, Match: fm.Match{AnySource: true, AnyDestination: true, Services: []fm.ServiceRef{{Kind: fm.ServiceObject, ID: "paired"}}}}, {ID: "any-rule", Action: fm.Permit, Match: fm.Match{AnySource: true, AnyDestination: true, AnyService: true}}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "paired"})
	if e != nil {
		t.Fatal(e)
	}
	page, e := s.Objects(ctx, snap.ID, "rule", storage.Filter{Protocol: "tcp", Port: "443"})
	if e != nil || page.Count != 1 || page.List[0].NativeID != "any-rule" {
		t.Fatal("separate native tuples combined", page, e)
	}
}
func TestExplicitUnsupportedEvidenceKeepsTypedOwnership(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	rangeOne, _ := flow.PrefixRange(netip.MustParsePrefix("10.0.0.0/24"))
	fw, e := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: storage.DeviceKey(1), Vendor: "test"}, Addresses: []fm.AddressRecord{{ID: "same", Ranges: []flow.AddressRange{rangeOne}}}, Services: []fm.ServiceRecord{{ID: "same", Unsupported: []string{"unsupported service term"}}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "unsupported"})
	if e != nil {
		t.Fatal(e)
	}
	address, e := s.Objects(ctx, snap.ID, "address", storage.Filter{})
	if e != nil || address.List[0].State != "known" || len(address.List[0].Diagnostics) != 0 {
		t.Fatal("service unsupported poisoned address", address, e)
	}
	service, e := s.Objects(ctx, snap.ID, "service", storage.Filter{})
	if e != nil || service.List[0].State != "unknown" || len(service.List[0].Diagnostics) != 1 {
		t.Fatal("typed unsupported evidence lost", service, e)
	}
}

func TestSnapshotLineageAndFrozenImportSoftware(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	data := graph(t, false).Export()
	data.Raw = "synthetic source for lineage"
	sourceDigest := sha256.Sum256([]byte(data.Raw))
	data.Metadata.SourceDigest = hex.EncodeToString(sourceDigest[:])
	data.Metadata.ParserVersion = "synthetic-parser-7"
	data.Metadata.Software = "synthetic-os-1"
	data.Metadata.CapturedAt = time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "lineage"})
	if e != nil {
		t.Fatal(e)
	}
	if snap.SourceDigest != data.Metadata.SourceDigest || snap.SemanticDigest != fw.SemanticDigest() || snap.ParserVersion != data.Metadata.ParserVersion || snap.Software != data.Metadata.Software || !snap.CapturedAt.Equal(data.Metadata.CapturedAt) {
		t.Fatal("lineage missing", snap)
	}
	again, e := s.Import(ctx, 1, fw, storage.ImportOptions{PublicationID: "lineage"})
	if e != nil || again.ID != snap.ID || again.SourceDigest != snap.SourceDigest || again.SemanticDigest != snap.SemanticDigest || again.Software != snap.Software || !again.CapturedAt.Equal(snap.CapturedAt) {
		t.Fatal("retry changed lineage", again, e)
	}
	job := models.Import{InstanceID: 1, RequestKey: "software", Software: "frozen-at-enqueue"}
	if e = db.Create(&job).Error; e != nil {
		t.Fatal(e)
	}
	var got models.Import
	if e = db.First(&got, job.ID).Error; e != nil || got.Software != job.Software {
		t.Fatal("software not durable", got, e)
	}
	encoded, e := json.Marshal(got)
	if e != nil || strings.Contains(string(encoded), "frozen-at-enqueue") || strings.Contains(string(encoded), "software") {
		t.Fatal("private import software exposed", string(encoded), e)
	}
}
func TestSettingsNetworkBindingsReplaceAtomically(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	settings, e := s.SaveSettings(ctx, 1, types.Settings{Zones: []types.Binding{{Zone: "inside", Networks: []string{"10.0.0.99/24", "2001:db8::10", " 192.0.2.1 "}}}})
	if e != nil {
		t.Fatal(e)
	}
	var rows []models.NetworkBinding
	if e = db.Where("instance_id = ?", 1).Order("position ASC").Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3 {
		t.Fatal("network rows missing", rows)
	}
	if rows[0].AddressFamily != 4 || rows[0].Start != "0a000000" || rows[0].End != "0a0000ff" || rows[0].OriginalText != "10.0.0.99/24" || rows[0].Version != settings.Version {
		t.Fatal("CIDR projection", rows[0])
	}
	if rows[1].AddressFamily != 6 || rows[1].Start != "20010db8000000000000000000000010" || rows[1].End != rows[1].Start {
		t.Fatal("IPv6 host projection", rows[1])
	}
	if rows[2].OriginalText != " 192.0.2.1 " {
		t.Fatal("original input lost", rows[2])
	}
	var zone models.ZoneBinding
	if e = db.First(&zone, rows[0].ZoneBindingID).Error; e != nil || zone.InstanceID != 1 || zone.Version != settings.Version {
		t.Fatal("network parent association", zone, e)
	}
	if _, e = s.SaveSettings(ctx, 1, types.Settings{Version: settings.Version, Zones: []types.Binding{{Zone: "replacement", Networks: []string{"203.0.113.1", "invalid"}}}}); !errors.Is(e, storage.InvalidErr) {
		t.Fatal("invalid network accepted", e)
	}
	var instance models.Instance
	db.First(&instance, 1)
	if instance.SettingsVersion != settings.Version {
		t.Fatal("failed network edit advanced version", instance)
	}
	db.Where("instance_id = ?", 1).Order("position ASC").Find(&rows)
	if len(rows) != 3 || rows[0].OriginalText != "10.0.0.99/24" {
		t.Fatal("failed edit replaced bindings", rows)
	}
	next, e := s.SaveSettings(ctx, 1, types.Settings{Version: settings.Version, Zones: []types.Binding{{Zone: "replacement", Networks: []string{"203.0.113.1"}}}})
	if e != nil {
		t.Fatal(e)
	}
	rows = nil
	db.Where("instance_id = ?", 1).Find(&rows)
	if len(rows) != 1 || rows[0].Version != next.Version || rows[0].OriginalText != "203.0.113.1" {
		t.Fatal("old bindings retained", rows)
	}
	if _, e = s.SaveSettings(ctx, 1, types.Settings{Version: next.Version}); e != nil {
		t.Fatal(e)
	}
	var count int64
	db.Model(&models.NetworkBinding{}).Where("instance_id = ?", 1).Count(&count)
	if count != 0 {
		t.Fatal("removed zone left orphan networks", count)
	}
}
