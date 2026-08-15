// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioral tests for the hand-written Auvik transcendence features.
//
// FIXTURES ARE SPEC-DERIVED BY CONSTRUCTION.
// specRow/specRel take `specField` and `specRel` VALUES, never raw strings, so a
// fixture physically cannot name an attribute that is not in the registry — and
// auvik_specguard_test.go proves every registry entry exists in the shipped
// spec. That chain (spec.json -> guard -> registry -> fixture -> code) is what
// makes the original failure impossible: the first cut of these tests invented
// field names that matched the invented names in the code, so the suite passed
// while every command was wrong against real data.

package cli

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

// specRow builds a JSON:API resource object exactly as `sync` stores it, keyed
// by registry fields so no invented attribute name can enter a fixture.
func specRow(id string, attrs map[specField]any, rels map[specRel]any) auvikRow {
	a := map[string]any{}
	for f, v := range attrs {
		a[f.Field] = v
	}
	data := map[string]any{"id": id, "type": "test", "attributes": a}
	if len(rels) > 0 {
		r := map[string]any{}
		for rel, v := range rels {
			switch t := v.(type) {
			case string: // to-one
				r[rel.Name] = map[string]any{"data": map[string]any{"id": t}}
			case []string: // to-many, ids only
				items := make([]any, 0, len(t))
				for _, id := range t {
					items = append(items, map[string]any{"id": id})
				}
				r[rel.Name] = map[string]any{"data": items}
			case map[string]string: // to-many with an attribute per member
				items := make([]any, 0, len(t))
				for id, name := range t {
					items = append(items, map[string]any{
						"id":         id,
						"attributes": map[string]any{fDeviceName.Field: name},
					})
				}
				r[rel.Name] = map[string]any{"data": items}
			}
		}
		data["relationships"] = r
	}
	return auvikRow{ID: id, Data: data}
}

// tenantRel is the device->tenant edge, which every resource carries under the
// same name and which no relationships type in the registry owns exclusively.
func tenantRel(id string) map[specRel]any {
	return map[specRel]any{{Type: "deviceRelationships", Name: "tenant"}: id}
}

func tenantMap() map[string]string {
	return map[string]string{"t1": "acme", "t2": "globex"}
}

func device(id, name, dtype, model, tenant string) auvikRow {
	return specRow(id, map[specField]any{
		fDeviceName:      name,
		fDeviceType:      dtype,
		fDeviceMakeModel: model,
	}, tenantRel(tenant))
}

// ---------------------------------------------------------------- eol

func TestBuildEolReport_DatesComeFromV2NotV1(t *testing.T) {
	asOf := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	devices := []auvikRow{
		device("d1", "old-switch", "l3Switch", "Cisco 2960", "t1"),
		device("d2", "soon-fw", "firewall", "FortiGate 60F", "t1"),
		device("d3", "next-year-ap", "accessPoint", "Meraki MR33", "t2"),
		device("d4", "fine-router", "router", "ISR 4331", "t2"),
	}
	// v1 lifecycle: STATUS ONLY. If the code ever reads dates from here again,
	// nothing will parse and the buckets will be wrong.
	v1 := []auvikRow{
		specRow("d1", map[specField]any{fLifecycleLastSupportStatus: "expired"}, nil),
		specRow("d4", map[specField]any{fLifecycleLastSupportStatus: "covered"}, nil),
	}
	// v2 lifecycle: the real dates.
	v2 := []auvikRow{
		specRow("d1", map[specField]any{fLifecycleV2LastSupportDate: "2024-03-01"}, nil),
		specRow("d2", map[specField]any{fLifecycleV2LastSupportDate: "2026-09-28"}, nil),
		specRow("d3", map[specField]any{fLifecycleV2LastSupportDate: "2027-03-01"}, nil),
		specRow("d4", map[specField]any{fLifecycleV2LastSupportDate: "2030-01-01"}, nil),
	}
	warranties := []auvikRow{
		specRow("d2", map[specField]any{fWarrantyExpiration: "2026-09-13"}, nil),
	}

	got := buildEolReport(devices, v1, v2, warranties, tenantMap(), asOf)

	if got.DevicesTotal != 4 || got.DevicesDated != 4 {
		t.Fatalf("total=%d dated=%d, want 4/4", got.DevicesTotal, got.DevicesDated)
	}
	want := map[string]int{bucketExpired: 1, bucket90: 1, bucket365: 1, bucketSupported: 1}
	for k, v := range want {
		if got.Buckets[k] != v {
			t.Errorf("bucket %s = %d, want %d", k, got.Buckets[k], v)
		}
	}
	if got.Rows[0].DeviceName != "old-switch" {
		t.Errorf("expired device must sort first, got %q", got.Rows[0].DeviceName)
	}
	if got.Rows[0].Client != "acme" {
		t.Errorf("client not resolved: %q", got.Rows[0].Client)
	}
	// Warranty (2026-09-13) precedes last-support (2026-09-28) for d2.
	for _, r := range got.Rows {
		if r.DeviceID == "d2" && r.Source != "warranty_expiration" {
			t.Errorf("d2 source = %q, want warranty_expiration (earlier date wins)", r.Source)
		}
	}
	// Status enums must land in status fields, never in date fields.
	for _, r := range got.Rows {
		if r.DeviceID == "d1" {
			if r.LastSupportStatus != "expired" {
				t.Errorf("d1 status = %q, want expired", r.LastSupportStatus)
			}
			if r.LastSupportDate != "2024-03-01" {
				t.Errorf("d1 date = %q, want the v2 date", r.LastSupportDate)
			}
		}
	}
}

func TestBuildEolReport_StatusOnlyStillBucketsExpired(t *testing.T) {
	// A tenant that synced v1 lifecycle but not v2 has no dates at all. Auvik's
	// own "expired" verdict must still be honoured rather than reported as
	// supported.
	asOf := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	devices := []auvikRow{device("d1", "eol-sw", "switch", "C2960", "t1")}
	v1 := []auvikRow{specRow("d1", map[specField]any{fLifecycleLastSupportStatus: "expired"}, nil)}

	got := buildEolReport(devices, v1, nil, nil, tenantMap(), asOf)
	if got.Buckets[bucketExpired] != 1 {
		t.Fatalf("status-only expired device not bucketed expired: %+v", got.Buckets)
	}
	if got.DevicesDated != 0 {
		t.Errorf("no dates were synced, so DevicesDated must be 0, got %d", got.DevicesDated)
	}
	r := got.Rows[0]
	if r.DaysRemaining != nil {
		t.Errorf("must not invent a countdown without a date, got %d", *r.DaysRemaining)
	}
	if r.Source != "status only (no date synced)" {
		t.Errorf("source = %q, want the status-only marker", r.Source)
	}
}

func TestBuildEolReport_NoDataIsNotAnExpiryClaim(t *testing.T) {
	devices := []auvikRow{device("d1", "unknown", "switch", "?", "t1")}
	got := buildEolReport(devices, nil, nil, nil, tenantMap(), time.Now().UTC())
	r := got.Rows[0]
	if r.Bucket != bucketSupported || r.DaysRemaining != nil {
		t.Fatalf("a device with no lifecycle data must not claim an expiry: %+v", r)
	}
	if r.Source != "no lifecycle or warranty data" {
		t.Errorf("source = %q, want the explicit no-data marker", r.Source)
	}
}

func TestBuildEolReport_EmptyMarshalsAsArray(t *testing.T) {
	got := buildEolReport(nil, nil, nil, nil, nil, time.Now())
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`"rows":\[\]`).Match(b) {
		t.Errorf("rows must marshal as [], got %s", b)
	}
}

// ------------------------------------------------------------ changes

func TestBuildChangesReport_MergesFourFamilies(t *testing.T) {
	devices := []auvikRow{device("d1", "core-sw", "l3Switch", "C9300", "t1")}
	configs := []auvikRow{
		specRow("c1", map[specField]any{
			fConfigBackupTime: "2026-08-01T10:00:00Z", fConfigIsRunning: true,
		}, map[specRel]any{rConfigDevice: "d1"}),
	}
	audits := []auvikRow{
		specRow("a1", map[specField]any{
			fAuditDateStarted: "2026-08-03T09:00:00Z",
			fAuditCategory:    "terminal", fAuditAction: "create",
			fAuditStatus: "closed", fAuditUser: "priya",
		}, map[specRel]any{{Type: "auditRelationships", Name: "device"}: "d1"}),
	}
	notes := []auvikRow{
		specRow("n1", map[specField]any{
			fNoteLastModified:   "2026-08-02T08:00:00Z",
			fNoteTitle:          "scheduled swap",
			fNoteEntityID:       "d1",
			fNoteLastModifiedBy: "marcus",
		}, nil),
	}
	alerts := []auvikRow{
		specRow("al1", map[specField]any{
			fAlertDetectedOn: "2026-08-04T07:00:00Z",
			fAlertName:       "interface flap", fAlertSeverity: "warning",
		}, map[specRel]any{rAlertEntity: "d1"}),
		specRow("al2", map[specField]any{
			fAlertDetectedOn: "2026-08-05T07:00:00Z", fAlertName: "other device",
		}, map[specRel]any{rAlertEntity: "d999"}),
	}

	got := buildChangesReport("d1", devices, configs, audits, notes, alerts, tenantMap(), time.Time{})

	if len(got.Events) != 4 {
		t.Fatalf("got %d events, want 4 (one per family): %+v", len(got.Events), got.Events)
	}
	for i, kind := range []string{"alert", "audit", "note", "config"} {
		if got.Events[i].Kind != kind {
			t.Errorf("event %d = %q, want %q", i, got.Events[i].Kind, kind)
		}
	}
	if got.DeviceName != "core-sw" || got.Client != "acme" {
		t.Errorf("device/client not resolved: %q / %q", got.DeviceName, got.Client)
	}
	for _, e := range got.Events {
		if e.Summary == "other device" {
			t.Fatal("a foreign device's alert leaked into the timeline")
		}
	}
	if got.Events[0].Summary != "warning: interface flap" {
		t.Errorf("alert summary = %q, want severity prefix", got.Events[0].Summary)
	}
	// The audit summary is built from category+action+status, since Auvik
	// publishes no free-text audit description.
	if got.Events[1].Summary != "terminal create (closed)" {
		t.Errorf("audit summary = %q", got.Events[1].Summary)
	}
	if got.Events[1].Actor != "priya" {
		t.Errorf("audit actor = %q, want priya", got.Events[1].Actor)
	}
}

func TestBuildChangesReport_UnknownDeviceIsEmpty(t *testing.T) {
	got := buildChangesReport("nope", nil, nil, nil, nil, nil, nil, time.Time{})
	if len(got.Events) != 0 {
		t.Fatalf("want zero events, got %d", len(got.Events))
	}
	b, _ := json.Marshal(got)
	if !regexp.MustCompile(`"events":\[\]`).Match(b) {
		t.Errorf("events must marshal as [], got %s", b)
	}
}

// -------------------------------------------------- configuration audit

func TestBuildConfigAuditReport_ThreeFindings(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	stale := 30 * 24 * time.Hour
	devices := []auvikRow{
		device("d1", "protected-sw", "switch", "C9300", "t1"),
		device("d2", "stale-fw", "firewall", "FG60F", "t1"),
		device("d3", "never-backed-up", "router", "ISR", "t2"),
		device("d4", "no-running", "switch", "C2960", "t2"),
	}
	configs := []auvikRow{
		specRow("c1", map[specField]any{
			fConfigBackupTime: "2026-08-13T00:00:00Z", fConfigIsRunning: true,
		}, map[specRel]any{rConfigDevice: "d1"}),
		specRow("c2", map[specField]any{
			fConfigBackupTime: "2026-01-01T00:00:00Z", fConfigIsRunning: true,
		}, map[specRel]any{rConfigDevice: "d2"}),
		specRow("c3", map[specField]any{
			fConfigBackupTime: "2026-08-13T00:00:00Z", fConfigIsRunning: false,
		}, map[specRel]any{rConfigDevice: "d4"}),
	}

	got := buildConfigAuditReport(devices, configs, tenantMap(), now, stale)

	if got.Counts[findingNoBackup] != 1 {
		t.Errorf("no_backup = %d, want 1 (d3)", got.Counts[findingNoBackup])
	}
	if got.Counts[findingStale] != 1 {
		t.Errorf("stale_backup = %d, want 1 (d2)", got.Counts[findingStale])
	}
	if got.Counts[findingNotRunning] != 1 {
		t.Errorf("no_running_config = %d, want 1 (d4)", got.Counts[findingNotRunning])
	}
	if got.Protected != 1 {
		t.Errorf("protected = %d, want 1 (d1)", got.Protected)
	}
	// A healthy device must never appear as a finding.
	for _, f := range got.Findings {
		if f.DeviceID == "d1" {
			t.Fatalf("protected device reported as a finding: %+v", f)
		}
	}
	// no_backup sorts first — it is the worst state.
	if got.Findings[0].Finding != findingNoBackup {
		t.Errorf("first finding = %q, want no_backup", got.Findings[0].Finding)
	}
}

func TestBuildConfigAuditReport_NoConfigsFlagsEveryDevice(t *testing.T) {
	// Honest degradation: with nothing synced every device is genuinely
	// unprotected as far as the mirror knows, and the caller is told so.
	devices := []auvikRow{
		device("d1", "a", "switch", "m", "t1"),
		device("d2", "b", "switch", "m", "t1"),
	}
	got := buildConfigAuditReport(devices, nil, tenantMap(), time.Now().UTC(), 30*24*time.Hour)
	if got.Counts[findingNoBackup] != 2 || got.Protected != 0 {
		t.Fatalf("want both devices flagged no_backup, got %+v protected=%d", got.Counts, got.Protected)
	}
}

// ----------------------------------------------------- inventory diff

func snap(id, tenant, name, model, fp string) deviceSnapshotRow {
	return deviceSnapshotRow{DeviceID: id, TenantID: tenant, DeviceName: name, MakeModel: model, Fingerprint: fp}
}

func TestBuildInventoryDiff_DetectsAddedRemovedChanged(t *testing.T) {
	before := map[string]deviceSnapshotRow{
		"d1": snap("d1", "t1", "keeps", "A", "fp-same"),
		"d2": snap("d2", "t1", "vanishes", "B", "fp-b"),
		"d3": snap("d3", "t2", "renamed-old", "C", "fp-c-old"),
	}
	after := map[string]deviceSnapshotRow{
		"d1": snap("d1", "t1", "keeps", "A", "fp-same"),
		"d3": snap("d3", "t2", "renamed-new", "C", "fp-c-new"),
		"d4": snap("d4", "t2", "brand-new", "D", "fp-d"),
	}

	got := buildInventoryDiff(before, after, tenantMap())

	if got.Counts["added"] != 1 || got.Counts["removed"] != 1 || got.Counts["changed"] != 1 {
		t.Fatalf("counts = %+v, want 1/1/1", got.Counts)
	}
	byChange := map[string]inventoryChange{}
	for _, c := range got.Changes {
		byChange[c.Change] = c
	}
	if byChange["removed"].DeviceName != "vanishes" {
		t.Errorf("removed = %q", byChange["removed"].DeviceName)
	}
	if byChange["removed"].Detail != "no longer returned by the API" {
		t.Errorf("removed detail = %q", byChange["removed"].Detail)
	}
	for _, c := range got.Changes {
		if c.DeviceID == "d1" {
			t.Fatal("unchanged device reported as a change")
		}
	}
	// The delta must be describable, not the useless catch-all.
	if byChange["changed"].Detail == "identity fields changed" {
		t.Error("changed detail fell back to the catch-all; the persisted columns should explain it")
	}
}

func TestBuildInventoryDiff_NoBaselineFabricatesNothing(t *testing.T) {
	after := map[string]deviceSnapshotRow{"d1": snap("d1", "t1", "a", "A", "fp")}
	got := buildInventoryDiff(map[string]deviceSnapshotRow{}, after, nil)
	if len(got.Changes) != 0 || got.Counts["added"] != 0 {
		t.Fatalf("no baseline must report nothing, got %+v", got)
	}
}

func TestBuildInventoryDiff_IdenticalFleetIsSilent(t *testing.T) {
	m := map[string]deviceSnapshotRow{"d1": snap("d1", "t1", "a", "A", "fp1")}
	if got := buildInventoryDiff(m, m, nil); len(got.Changes) != 0 {
		t.Fatalf("identical fleets must be silent, got %+v", got.Changes)
	}
}

func TestDeviceFingerprint_IgnoresVolatileTelemetry(t *testing.T) {
	a := deviceSnapshotRow{DeviceID: "d1", DeviceName: "sw", MakeModel: "m", Serial: "S1", Firmware: "1.0"}
	b := a
	if deviceFingerprint(a) != deviceFingerprint(b) {
		t.Fatal("identical rows must fingerprint identically")
	}
	b.Firmware = "1.1"
	if deviceFingerprint(a) == deviceFingerprint(b) {
		t.Error("a firmware change must alter the fingerprint")
	}
}

// --------------------------------------------------- usage reconcile

func TestBuildReconcileReport_AttributesByDeviceID(t *testing.T) {
	devices := []auvikRow{
		device("dev-1", "sw-1", "switch", "m", "t1"),
		device("dev-2", "sw-2", "switch", "m", "t1"),
		device("dev-9", "fw-1", "firewall", "m", "t2"),
	}
	// Auvik bills via a device LIST on the usage record, and identifies the
	// client by domainPrefix — not by a tenant relationship.
	billing := []auvikRow{
		specRow("b1", map[specField]any{
			fUsageDomainPrefix: "acme", fUsageMonServers: float64(1), fUsageBillableDays: float64(30),
		}, map[specRel]any{rUsageDevices: map[string]string{"dev-1": "sw-1"}}),
		specRow("b2", map[specField]any{
			fUsageDomainPrefix: "globex",
		}, map[specRel]any{rUsageDevices: map[string]string{"dev-9": "fw-1"}}),
	}

	got := buildReconcileReport(devices, billing, tenantMap())

	byClient := map[string]reconcileClient{}
	for _, c := range got.Clients {
		byClient[c.Client] = c
	}
	acme := byClient["acme"]
	if acme.BilledCount == nil || *acme.BilledCount != 1 || acme.InventoryCount != 2 {
		t.Fatalf("acme = %+v, want billed 1 inventory 2", acme)
	}
	if acme.Delta == nil || *acme.Delta != 1 {
		t.Errorf("acme delta = %v, want +1", acme.Delta)
	}
	// Attribution must name the unbilled device, matched by ID not by name.
	if len(acme.SeenNotBilled) != 1 || acme.SeenNotBilled[0] != "sw-2" {
		t.Errorf("SeenNotBilled = %v, want [sw-2]", acme.SeenNotBilled)
	}
	if len(acme.BilledNotSeen) != 0 {
		t.Errorf("BilledNotSeen = %v, want empty (dev-1 is in inventory)", acme.BilledNotSeen)
	}
	if acme.MonitoredServers != 1 || acme.BillableDays != 30 {
		t.Errorf("counts not carried: %+v", acme)
	}
	globex := byClient["globex"]
	if globex.Delta == nil || *globex.Delta != 0 {
		t.Errorf("globex should reconcile exactly, got %v", globex.Delta)
	}
}

func TestBuildReconcileReport_NoBillingKeepsNilNotZero(t *testing.T) {
	devices := []auvikRow{device("d1", "sw-1", "switch", "m", "t1")}
	got := buildReconcileReport(devices, nil, tenantMap())
	c := got.Clients[0]
	if c.BilledCount != nil || c.Delta != nil {
		t.Fatalf("missing billing must stay nil, got billed=%v delta=%v", c.BilledCount, c.Delta)
	}
	if got.Mismatched != 0 {
		t.Errorf("a missing billing record is not a mismatch, got %d", got.Mismatched)
	}
}

// ----------------------------------------------- device discovery-gaps

func TestBuildDiscoveryGapsReport_NotAuthorizedIsTheGap(t *testing.T) {
	devices := []auvikRow{
		device("d1", "ok-sw", "switch", "m", "t1"),
		device("d2", "no-snmp", "switch", "m", "t1"),
		device("d3", "still-working", "router", "m", "t2"),
	}
	statuses := []auvikRow{
		specRow("s1", map[specField]any{
			fDiscoverySNMP: "authorized", fDiscoveryLogin: "privileged",
			fDiscoveryWMI: "notSupported", fDiscoveryVMware: "disabled",
		}, map[specRel]any{{Type: "deviceRelationships", Name: "device"}: "d1"}),
		specRow("s2", map[specField]any{
			fDiscoverySNMP: "notAuthorized", fDiscoveryLogin: "authorized",
		}, map[specRel]any{{Type: "deviceRelationships", Name: "device"}: "d2"}),
		specRow("s3", map[specField]any{
			fDiscoverySNMP: "determining",
		}, map[specRel]any{{Type: "deviceRelationships", Name: "device"}: "d3"}),
	}

	got := buildDiscoveryGapsReport(devices, nil, statuses, tenantMap())

	if got.FullyDiscovered != 1 {
		t.Errorf("fully_discovered = %d, want 1 (d1: authorized/privileged/notSupported/disabled are all healthy)", got.FullyDiscovered)
	}
	if len(got.Gaps) != 2 {
		t.Fatalf("want 2 gaps (rejected + in-progress), got %d: %+v", len(got.Gaps), got.Gaps)
	}
	byDevice := map[string]discoveryGap{}
	for _, g := range got.Gaps {
		byDevice[g.DeviceID] = g
	}
	if got.Counts["credentialsRejected"] != 1 {
		t.Errorf("credentialsRejected = %d, want 1", got.Counts["credentialsRejected"])
	}
	if g := byDevice["d2"]; len(g.MissingWhat) != 1 || g.MissingWhat[0] != "snmp" {
		t.Errorf("d2 rejected probes = %v, want [snmp]", g.MissingWhat)
	}
	// "determining" is Auvik still working, not an operator action item.
	if g := byDevice["d3"]; len(g.MissingWhat) != 0 || len(g.Pending) != 1 {
		t.Errorf("d3 should be in-progress not rejected: %+v", g)
	}
}

func TestBuildDiscoveryGapsReport_NoEvidenceIsNeitherGapNorHealthy(t *testing.T) {
	devices := []auvikRow{device("d1", "unknown", "switch", "m", "t1")}
	got := buildDiscoveryGapsReport(devices, nil, nil, tenantMap())
	if len(got.Gaps) != 0 {
		t.Errorf("no discovery data must not be reported as a gap: %+v", got.Gaps)
	}
	if got.FullyDiscovered != 0 {
		t.Errorf("no discovery data must NOT inflate fully_discovered, got %d", got.FullyDiscovered)
	}
	if got.NoEvidence != 1 {
		t.Errorf("no_discovery_data = %d, want 1", got.NoEvidence)
	}
}

// ---------------------------------------------------------- asm shadow

func TestBuildShadowReport_JoinsThroughUserEmails(t *testing.T) {
	clients := []auvikRow{specRow("c1", map[specField]any{fASMClientName: "acme"}, nil)}
	apps := []auvikRow{
		specRow("app1", map[specField]any{fASMAppName: "Figma"},
			map[specRel]any{rASMAppClient: "c1", rASMAppUsers: []string{"u1", "u2"}}),
		specRow("app2", map[specField]any{fASMAppName: "Confluence"},
			map[specRel]any{rASMAppClient: "c1", rASMAppUsers: []string{"u3"}}),
	}
	users := []auvikRow{
		specRow("u1", map[specField]any{fASMUserEmail: "a@x.com", fASMUserActive: true}, nil),
		specRow("u2", map[specField]any{fASMUserEmail: "b@x.com", fASMUserActive: true}, nil),
		// Disabled user must not count as usage.
		specRow("u3", map[specField]any{fASMUserEmail: "c@x.com", fASMUserDisabled: true}, nil),
	}
	// Licenses carry NO app relationship — they join by email only.
	licenses := []auvikRow{
		specRow("l1", map[specField]any{fASMLicenseEmail: "c@x.com", fASMLicenseType: "standard"}, nil),
	}

	got := buildShadowReport(apps, users, licenses, clients)

	byApp := map[string]shadowFinding{}
	for _, f := range got.Findings {
		byApp[f.Application] = f
	}
	fig := byApp["Figma"]
	if fig.Finding != findingUnlicensed || fig.ActiveUsers != 2 || fig.Licenses != 0 {
		t.Errorf("Figma = %+v, want unlicensed_usage with 2 active users", fig)
	}
	if fig.Client != "acme" {
		t.Errorf("client not resolved: %q", fig.Client)
	}
	conf := byApp["Confluence"]
	if conf.Finding != findingUnusedSeat || conf.ActiveUsers != 0 {
		t.Errorf("Confluence = %+v, want unused_licenses with 0 active users", conf)
	}
}

func TestBuildShadowReport_BalancedAppIsNotAFinding(t *testing.T) {
	clients := []auvikRow{specRow("c1", map[specField]any{fASMClientName: "acme"}, nil)}
	apps := []auvikRow{specRow("app1", map[specField]any{fASMAppName: "Slack"},
		map[specRel]any{rASMAppClient: "c1", rASMAppUsers: []string{"u1"}})}
	users := []auvikRow{specRow("u1", map[specField]any{fASMUserEmail: "a@x.com", fASMUserActive: true}, nil)}
	licenses := []auvikRow{specRow("l1", map[specField]any{fASMLicenseEmail: "a@x.com"}, nil)}

	got := buildShadowReport(apps, users, licenses, clients)
	for _, f := range got.Findings {
		if f.Application == "Slack" {
			t.Fatalf("1 active user with 1 license is balanced, got finding %+v", f)
		}
	}
}

// ---------------------------------------------------------- alert noise

func TestBuildNoiseReport_RanksByVolumeWithResolvedNames(t *testing.T) {
	devices := []auvikRow{
		device("d1", "flappy", "switch", "m", "t1"),
		device("d2", "quiet", "router", "m", "t2"),
	}
	base := time.Now().UTC().Add(-2 * time.Hour)
	at := func(d time.Duration) string { return base.Add(d).Format(time.RFC3339) }
	alerts := []auvikRow{
		specRow("a1", map[specField]any{
			fAlertDetectedOn: at(0), fAlertSeverity: "warning",
			fAlertDismissed: true, fAlertStatus: "resolved",
		}, map[specRel]any{rAlertEntity: "d1"}),
		specRow("a2", map[specField]any{
			fAlertDetectedOn: at(time.Minute), fAlertSeverity: "critical",
			fAlertDismissed: true, fAlertStatus: "resolved",
		}, map[specRel]any{rAlertEntity: "d1"}),
		specRow("a3", map[specField]any{
			fAlertDetectedOn: at(2 * time.Minute), fAlertSeverity: "warning",
			fAlertStatus: "created",
		}, map[specRel]any{rAlertEntity: "d1"}),
		specRow("a4", map[specField]any{
			fAlertDetectedOn: at(3 * time.Minute), fAlertSeverity: "info",
		}, map[specRel]any{rAlertEntity: "d2"}),
	}

	got := buildNoiseReport(alerts, devices, tenantMap(), time.Now().UTC().Add(-24*time.Hour), "device")

	if got.AlertsInWindow != 4 {
		t.Fatalf("AlertsInWindow = %d, want 4 (detectedOn must parse)", got.AlertsInWindow)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got.Rows))
	}
	top := got.Rows[0]
	if top.DeviceName != "flappy" || top.Rank != 1 || top.Alerts != 3 {
		t.Errorf("top row = %+v, want flappy rank 1 with 3 alerts", top)
	}
	if top.Dismissed != 2 {
		t.Errorf("dismissed = %d, want 2", top.Dismissed)
	}
	if top.DeviceType != "switch" || top.Client != "acme" {
		t.Errorf("join did not resolve device metadata: %+v", top)
	}
	if top.Statuses["resolved"] != 2 || top.Statuses["created"] != 1 {
		t.Errorf("status mix = %+v", top.Statuses)
	}
}

func TestBuildNoiseReport_WindowExcludesOldAlerts(t *testing.T) {
	devices := []auvikRow{device("d1", "d", "switch", "m", "t1")}
	alerts := []auvikRow{
		specRow("old", map[specField]any{fAlertDetectedOn: "2020-01-01T00:00:00Z"},
			map[specRel]any{rAlertEntity: "d1"}),
		specRow("new", map[specField]any{fAlertDetectedOn: time.Now().UTC().Format(time.RFC3339)},
			map[specRel]any{rAlertEntity: "d1"}),
	}
	got := buildNoiseReport(alerts, devices, tenantMap(), time.Now().UTC().Add(-24*time.Hour), "device")
	if got.AlertsTotal != 2 || got.AlertsInWindow != 1 {
		t.Errorf("total=%d inWindow=%d, want 2/1", got.AlertsTotal, got.AlertsInWindow)
	}
}

func TestBuildNoiseReport_GroupByClient(t *testing.T) {
	devices := []auvikRow{
		device("d1", "a", "switch", "m", "t1"),
		device("d2", "b", "switch", "m", "t1"),
	}
	now := time.Now().UTC().Format(time.RFC3339)
	alerts := []auvikRow{
		specRow("a1", map[specField]any{fAlertDetectedOn: now}, map[specRel]any{rAlertEntity: "d1"}),
		specRow("a2", map[specField]any{fAlertDetectedOn: now}, map[specRel]any{rAlertEntity: "d2"}),
	}
	got := buildNoiseReport(alerts, devices, tenantMap(), time.Time{}, "client")
	if len(got.Rows) != 1 || got.Rows[0].Alerts != 2 || got.Rows[0].Client != "acme" {
		t.Fatalf("client rollup wrong: %+v", got.Rows)
	}
}

// ------------------------------------------------------------- helpers

func TestParseAuvikTime(t *testing.T) {
	for _, in := range []string{
		"2026-08-14T12:00:00.000Z", "2026-08-14T12:00:00Z", "2026-08-14", "2026-08-14 12:00:00",
	} {
		if _, ok := parseAuvikTime(in); !ok {
			t.Errorf("failed to parse %q", in)
		}
	}
	if _, ok := parseAuvikTime("not a date"); ok {
		t.Error("garbage must not parse")
	}
}

func TestAuvikRowRelMany(t *testing.T) {
	r := specRow("x", nil, map[specRel]any{rASMAppUsers: []string{"u1", "u2"}})
	if got := r.relMany(rASMAppUsers.Name); len(got) != 2 {
		t.Errorf("relMany = %v, want 2 ids", got)
	}
	if got := r.relMany("nonexistent"); len(got) != 0 {
		t.Errorf("missing relationship must yield empty, got %v", got)
	}
}
