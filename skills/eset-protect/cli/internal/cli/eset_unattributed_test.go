// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// unattributedFixture is the Codex round-2 scenario plus its siblings. Acme's
// infected endpoint dev-gone was deleted and devices were fully synced, so the
// mirror no longer holds it, but its detection and its open incident are
// still synced. dev-orphan is synced but sits in a group created after the
// last device-groups sync. inc-none names no device, even after hydration.
func unattributedFixture(now time.Time) map[string][]map[string]any {
	devices := exposureFixtureDevices(now)
	devices = append(devices, map[string]any{"uuid": "dev-orphan", "displayName": "NEW-SITE-PC", "parentGroupUuid": "group-after-sync",
		"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * reportCustomerDay).Format(time.RFC3339)})
	det := func(uuid, device string) map[string]any {
		return map[string]any{"uuid": uuid, "displayName": "Win32/Agent.XYZ", "typeName": "Trojan", "occurTime": reportCustomerAgo(now, 2*reportCustomerDay),
			"severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": device}, "objectHashSha1": "3395856ce81f2b7382dee72602f798b642f14140"}
	}
	inc := func(uuid string, devices []string) map[string]any {
		return map[string]any{"uuid": uuid, "displayName": "Ransomware behavior " + uuid, "deviceUuids": devices, "status": "INCIDENT_STATUS_OPEN",
			"severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "createTime": reportCustomerAgo(now, reportCustomerDay), "updateTime": reportCustomerAgo(now, reportCustomerDay)}
	}
	return map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       devices,
		"detections":    {det("det-gone", "dev-gone"), det("det-orphan", "dev-orphan"), det("det-beta", "dev-beta")},
		"incidents":     {inc("inc-gone", []string{"dev-gone"}), inc("inc-none", []string{}), inc("inc-beta", []string{"dev-beta"})},
	}
}

// TestReportCustomerCountsRowsNoDeviceTiesToACustomer guards
// customer-views-count-unattributed-rows on report customer: rows whose owner
// is unknown are counted per section, the section is incomplete, the totals
// stay this customer's own, and the human report never reads as a clean
// quarter. Before the fix the same mirror gave detections total 0, incidents
// open_now 0, rejected_rows 0 and incomplete false.
func TestReportCustomerCountsRowsNoDeviceTiesToACustomer(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.DogfoodEnvVar, "")
	now := time.Now()
	db := exposureSeedDB(t, unattributedFixture(now))
	fake := &exposureFakeLive{
		lists: map[string]func(map[string]string) (esetPageResult, error){
			"/v1/device-vulnerabilities": exposureScopedPage(t,
				exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", ""),
				exposureVulnJSON("dev-new", "group-after-sync", "Google Chrome", "CVE-2024-3400", 70, "SEVERITY_LEVEL_HIGH", "")),
			"/v1/device-patches": exposurePage(t, false, map[string]any{"uuid": "dp-new", "deviceUuid": "dev-new", "deviceGroupUuid": "group-after-sync",
				"applicationPatch": map[string]any{"catalogId": "101", "category": patchesBacklogCategorySecurity, "resultVersionName": "127.0"}}),
			"/v1/patching-process-details": exposurePage(t, false),
			"/v1/scans/recent": exposurePage(t, false,
				map[string]any{"deviceUuid": "dev-orphan", "endTime": reportCustomerAgo(now, reportCustomerDay), "endedSuccessfully": true}),
		},
		gets: map[string]func() (json.RawMessage, error){
			"/v2/incidents/inc-none": reportCustomerHydrated("inc-none", []string{}, ""),
		},
	}
	exposureStub(t, &reportCustomerNewLive, fake)

	view, errOut := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	d := view.Detections
	if d == nil || d.Total != 0 || d.UnattributedDetections != 2 || !reflect.DeepEqual(d.UnattributedDetectionUUIDs, []string{"det-gone", "det-orphan"}) || !d.Incomplete {
		t.Fatalf("detections = %+v; want total 0 with det-gone and det-orphan unattributed and the section incomplete", d)
	}
	in := view.Incidents
	if in == nil || in.OpenNow != 0 || in.UnattributedIncidents != 2 || in.UnattributedOpen != 2 ||
		!reflect.DeepEqual(in.UnattributedIncidentUUIDs, []string{"inc-gone", "inc-none"}) || !in.Incomplete {
		t.Fatalf("incidents = %+v; want inc-gone and inc-none unattributed (both open) and the section incomplete", in)
	}
	if h := view.Health; h == nil || h.UnattributedDevices != 1 || !reflect.DeepEqual(h.UnattributedDeviceUUIDs, []string{"dev-orphan"}) || !h.Incomplete {
		t.Fatalf("health = %+v; want dev-orphan unattributed", h)
	}
	if v := view.Vulnerabilities; v == nil || v.Total != 1 || v.UnattributedRows != 1 || !v.Incomplete {
		t.Fatalf("vulnerabilities = %+v; want Acme's row and the unknown-group row unattributed", v)
	}
	if p := view.Patches; p == nil || p.PendingSecurityPatches != 0 || p.UnattributedRows != 1 || !p.Incomplete {
		t.Fatalf("patches = %+v; want the unknown-owner security patch unattributed", p)
	}
	if sc := view.ScanCoverage; sc == nil || sc.DevicesScanned != 0 || sc.UnattributedScans != 1 || !sc.Incomplete {
		t.Fatalf("scan coverage = %+v; want dev-orphan's scan unattributed", sc)
	}
	if view.UnattributedRows != 8 || view.OtherCustomerRows != 0 || view.RejectedRows != 0 {
		t.Fatalf("unattributed_rows = %d other = %d rejected = %d, want 8/0/0", view.UnattributedRows, view.OtherCustomerRows, view.RejectedRows)
	}
	if !strings.Contains(errOut, "8 row(s) could not be tied to any customer") {
		t.Fatalf("stderr must warn about the unattributed rows: %q", errOut)
	}

	// Beta's own rows are attributed and are not counted for Acme.
	beta, _ := reportCustomerRun(t, "Beta Logistics", "--db", db, "--json")
	if beta.Detections == nil || beta.Detections.Total != 1 || beta.Detections.UnattributedDetections != 2 || beta.Incidents.OpenNow != 1 {
		t.Fatalf("beta detections = %+v incidents = %+v", beta.Detections, beta.Incidents)
	}

	out, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Detections: 0 total (+2 unattributed, see note)",
		"0 open now (+2 unattributed, see note)",
		"note: incomplete: 2 incident(s) (2 open now) could not be tied to Acme Dental or another customer",
		"det-gone, det-orphan",
		"8 row(s) could not be tied to any customer",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("human report lacks %q:\n%s", want, out)
		}
	}
}

// TestCustomerScopedFleetViewsCountUnattributedDevices covers fleet health,
// gaps and stale: a synced device whose group is not in the synced tree is
// owner-unknown (not "(unassigned)"); --customer leaves it out but counts it,
// and the human output is never an all-clear while it is.
func TestCustomerScopedFleetViewsCountUnattributedDevices(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := exposureSeedDB(t, unattributedFixture(now))

	out, err := fleetStaleTestRun(t, "fleet", "health", "--db", db, "--customer", "Beta Logistics", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var health fleetHealthView
	fleetHealthTestDecode(t, []byte(out), &health)
	if health.UnattributedDevices != 1 || !reflect.DeepEqual(health.UnattributedDeviceUUIDs, []string{"dev-orphan"}) || health.Totals.Devices != 1 {
		t.Fatalf("fleet health --customer = %+v", health)
	}
	out, err = fleetStaleTestRun(t, "fleet", "health", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	health = fleetHealthView{}
	fleetHealthTestDecode(t, []byte(out), &health)
	found := false
	for _, r := range health.Customers {
		if r.Customer == esetCustomerNone {
			t.Fatalf("an owner-unknown device must not be filed as %s: %+v", esetCustomerNone, health.Customers)
		}
		if r.Customer == esetCustomerUnknownGroup && r.Devices == 1 {
			found = true
		}
	}
	if !found || health.UnattributedDevices != 1 {
		t.Fatalf("fleet health must roll dev-orphan up to %q: %+v", esetCustomerUnknownGroup, health)
	}

	out, err = fleetStaleTestRun(t, "fleet", "gaps", "--db", db, "--customer", "Beta Logistics", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var gaps fleetGapsView
	fleetHealthTestDecode(t, []byte(out), &gaps)
	if gaps.UnattributedDevices != 1 || gaps.DevicesChecked != 1 {
		t.Fatalf("fleet gaps --customer = %+v", gaps)
	}

	out, err = fleetStaleTestRun(t, "fleet", "stale", "--db", db, "--customer", "Beta Logistics", "--days", "30", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var stale fleetStaleView
	fleetHealthTestDecode(t, []byte(out), &stale)
	if stale.Total != 0 || stale.UnattributedDevices != 1 || !reflect.DeepEqual(stale.UnattributedDeviceUUIDs, []string{"dev-orphan"}) {
		t.Fatalf("fleet stale --customer = %+v", stale)
	}
	human, err := fleetStaleTestRun(t, "fleet", "stale", "--db", db, "--customer", "Beta Logistics", "--days", "30", "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(human, "No devices stale") || !strings.Contains(human, "1 stale device(s) could not be tied to Beta Logistics or another customer") {
		t.Fatalf("fleet stale must not print an all-clear over an unattributed stale device:\n%s", human)
	}
	human, err = fleetStaleTestRun(t, "fleet", "health", "--db", db, "--customer", "Beta Logistics", "--human-friendly")
	if err != nil || !strings.Contains(human, "1 synced device(s) could not be tied to Beta Logistics") {
		t.Fatalf("fleet health human output must name the unattributed device: %v\n%s", err, human)
	}
}

// TestDeviceKeyedViewsTreatAnUnknownGroupAsUnknownOwner covers triage,
// detections spread, devices profile and incidents context: a synced device
// whose group is not in the tree is owner-unknown, so triage --customer counts
// its detection as unattributed (it was filed under "(unassigned)" and dropped)
// and the labels say "(group not in synced tree)".
func TestDeviceKeyedViewsTreatAnUnknownGroupAsUnknownOwner(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := exposureSeedDB(t, unattributedFixture(now))

	out, err := fleetStaleTestRun(t, "triage", "--customer", "Beta Logistics", "--db", db, "--since", "7d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var tv triageView
	fleetHealthTestDecode(t, []byte(out), &tv)
	if tv.UnattributedDetections != 2 || !reflect.DeepEqual(tv.UnattributedDetectionUUIDs, []string{"det-gone", "det-orphan"}) {
		t.Fatalf("triage --customer unattributed detections = %d %v, want det-gone and det-orphan", tv.UnattributedDetections, tv.UnattributedDetectionUUIDs)
	}

	out, err = fleetStaleTestRun(t, "detections", "spread", "--db", db, "--since", "7d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var sv detectionsSpreadView
	fleetHealthTestDecode(t, []byte(out), &sv)
	if len(sv.Groups) != 1 || sv.Groups[0].UnattributedDetections != 2 || sv.UnattributedDetections != 2 ||
		!reflect.DeepEqual(sv.Groups[0].Customers, []string{"Beta Logistics"}) || sv.Groups[0].DevicesUnknownOwner != 2 || sv.Groups[0].CustomersPossible != 3 {
		t.Fatalf("detections spread = %+v", sv)
	}

	// A single-tenant tree (no COMPANY group) has one owner for every row.
	tenant := newExposureScope([]esetGroup{{UUID: "root", Name: "All"}}, nil)
	if c, known := tenant.owner("group-after-sync", "dev-gone"); !known {
		t.Fatalf("a single-tenant scope must treat every row as the tenant's: %+v", c)
	}
	if c, known := newEsetTree([]esetGroup{{UUID: "root", Name: "All"}}).ownerOf("group-after-sync"); !known || c.Name != esetCustomerTenant {
		t.Fatalf("single-tenant ownerOf = %+v %v", c, known)
	}

	groups := []esetGroup{{UUID: "acme", Name: "Acme Dental", EntityType: esetCompanyEntityType}}
	if v := devicesProfileDeviceView(esetDevice{UUID: "dev-orphan", ParentGroupUUID: "group-after-sync"}, groups); v.Customer != esetCustomerUnknownGroup {
		t.Fatalf("devices profile customer = %q, want %q", v.Customer, esetCustomerUnknownGroup)
	}
	inc, _ := incidentsContextParseIncident(json.RawMessage(`{"incident":{"uuid":"inc-1","deviceUuids":["dev-orphan"]}}`))
	results := []cliutil.FanoutResult[incidentsContextFetched]{{Source: "device:dev-orphan", Value: incidentsContextFetched{
		Job: incidentsContextJob{"device", "dev-orphan"}, Raw: json.RawMessage(`{"device":{"uuid":"dev-orphan","displayName":"NEW-SITE-PC","parentGroupUuid":"group-after-sync"}}`)}}}
	cv := incidentsContextBuild(inc, incidentsContextTruncated{}, results, nil, groups, nil)
	if len(cv.Devices) != 1 || cv.Devices[0].Customer != esetCustomerUnknownGroup || cv.UnattributedDevices != 1 {
		t.Fatalf("incidents context = %+v", cv)
	}
}

// TestPatchesBacklogCountsUnattributedLocalPatches covers the silent drop in
// patchesBacklogInCustomer: a mirrored security patch whose owner is unknown
// was left out of --customer with no count at all.
func TestPatchesBacklogCountsUnattributedLocalPatches(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.DogfoodEnvVar, "")
	now := time.Now()
	rows := unattributedFixture(now)
	rows["device-patches"] = []map[string]any{{"uuid": "dp-orphan", "deviceUuid": "dev-orphan", "deviceGroupUuid": "group-after-sync",
		"applicationPatch": map[string]any{"catalogId": "101", "category": patchesBacklogCategorySecurity, "resultVersionName": "127.0", "severity": "SEVERITY_LEVEL_HIGH"}}}
	db := exposureSeedDB(t, rows)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposureScopedPage(t),
		"/v1/patching-process-details": exposurePage(t, false),
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)
	out, _, err := exposureRunCLI(t, "patches", "backlog", "--customer", "Acme Dental", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view patchesBacklogView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if view.PatchSource != "local" || len(view.Customers) != 0 || view.UnattributedRows != 1 || !reflect.DeepEqual(view.UnattributedDeviceUUIDs, []string{"dev-orphan"}) {
		t.Fatalf("patches backlog --customer = %+v", view)
	}
	human, _, err := exposureRunCLI(t, "patches", "backlog", "--customer", "Acme Dental", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(human, "No available patches found.\n") || !strings.Contains(human, "1 patch row(s) could not be tied to Acme Dental") {
		t.Fatalf("patches backlog must not print an all-clear over an unattributed patch:\n%s", human)
	}
}
