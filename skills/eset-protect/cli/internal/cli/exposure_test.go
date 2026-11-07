// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestNovelExposureHelpWires smoke-tests that the exposure command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelExposureHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"exposure", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("exposure --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "exposure"} {
		if !strings.Contains(help, want) {
			t.Fatalf("exposure --help missing %q in output:\n%s", want, help)
		}
	}
}

// ---- shared fixtures for exposure, patches backlog and report customer ----

func exposureFixtureGroups() []map[string]any {
	return []map[string]any{
		{"uuid": "msp", "displayName": "Servosity MSP", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{"uuid": "acme", "displayName": "Acme Dental", "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType},
		{"uuid": "acme-hq", "displayName": "HQ", "parentGroupUuid": "acme", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{"uuid": "beta", "displayName": "Beta Logistics", "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType},
		// A customer nested inside Acme's tree: ESET's recursive group scope
		// for Acme also returns its rows, which must stay Acme Labs'.
		{"uuid": "acme-lab", "displayName": "Acme Labs", "parentGroupUuid": "acme-hq", "linkedEntityType": esetCompanyEntityType},
	}
}

func exposureFixtureDevices(now time.Time) []map[string]any {
	return []map[string]any{
		{"uuid": "dev-front", "displayName": "FRONTDESK-01", "parentGroupUuid": "acme-hq", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK",
			"lastSyncTime": now.Add(-time.Hour).Format(time.RFC3339), "operatingSystem": map[string]any{"displayName": "Microsoft Windows 11 Pro"}},
		{"uuid": "dev-recep", "displayName": "RECEPTION-02", "parentGroupUuid": "acme", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED",
			"isMuted": true, "lastSyncTime": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)},
		{"uuid": "dev-xray", "displayName": "XRAY-PC", "parentGroupUuid": "acme-hq", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_RECOMMENDED"},
		{"uuid": "dev-beta", "displayName": "BETA-WH-01", "parentGroupUuid": "beta", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK",
			"lastSyncTime": now.Add(-2 * time.Hour).Format(time.RFC3339)},
	}
}

func exposureFixtureScope(t *testing.T) exposureScope {
	t.Helper()
	var groups []esetGroup
	var devices []esetDevice
	exposureRoundTrip(t, exposureFixtureGroups(), &groups)
	exposureRoundTrip(t, exposureFixtureDevices(time.Now()), &devices)
	return newExposureScope(groups, devices)
}

func exposureRoundTrip(t *testing.T, in, out any) {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
}

// exposureVulnJSON builds one live v1DeviceVulnerability row.
func exposureVulnJSON(device, group, kind, cve string, risk int, severity, first string, patches ...string) map[string]any {
	core := map[string]any{"cveNumber": cve, "riskScore": risk, "severity": severity, "firstDetectTime": first, "patchAvailable": len(patches) > 0}
	row := map[string]any{"deviceUuid": device, "deviceGroupUuid": group, "availablePatchUuids": patches}
	switch kind {
	case "os":
		core["osFamilyId"] = 1
		row["osVulnerability"] = core
	case "package":
		core["package"] = map[string]any{"name": "openssl", "displayName": "OpenSSL", "versionName": "3.0.2-0ubuntu1"}
		row["packageVulnerability"] = core
	default:
		core["application"] = map[string]any{"displayName": kind, "version": map[string]any{"name": "126.0.6478.127"}}
		row["applicationVulnerability"] = core
	}
	return row
}

func exposureDecodeFixture(t *testing.T, rows ...map[string]any) []exposureVuln {
	t.Helper()
	var rej esetRejects
	out := exposureDecodeVulns(exposureItems(t, rows...), &rej)
	if rej.N != 0 {
		t.Fatalf("fixture rows were rejected: %+v", rej)
	}
	return out
}

func exposureItems(t *testing.T, rows ...map[string]any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// exposureSeedDB writes resource rows (keyed by their uuid) into a fresh
// mirror and records a completed sync for each seeded resource, as a real
// sync would; an unfinished sync is a separate fixture
// (TestMirrorReadersFlagAnInterruptedSync).
func exposureSeedDB(t *testing.T, rows map[string][]map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for rt, list := range rows {
		if err := db.SaveSyncState(rt, "", len(list)); err != nil {
			t.Fatalf("seed sync state %s: %v", rt, err)
		}
		for i, r := range list {
			id, _ := r["uuid"].(string)
			if id == "" {
				id = fmt.Sprintf("%s-%d", rt, i)
			}
			b, _ := json.Marshal(r)
			if err := db.Upsert(rt, id, b); err != nil {
				t.Fatalf("seed %s: %v", rt, err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// exposureFakeLive serves canned ESET Connect responses keyed by URL path.
type exposureFakeLive struct {
	lists  map[string]func(params map[string]string) (esetPageResult, error)
	gets   map[string]func() (json.RawMessage, error)
	params map[string]map[string]string
}

func exposurePathOf(url string) string {
	if i := strings.Index(url, ".eset.systems"); i >= 0 {
		return url[i+len(".eset.systems"):]
	}
	return url
}

func (f *exposureFakeLive) api() exposureLiveAPI {
	if f.params == nil {
		f.params = map[string]map[string]string{}
	}
	return exposureLiveAPI{
		list: func(_ context.Context, url string, params map[string]string, _ string, _ int) (esetPageResult, error) {
			p := exposurePathOf(url)
			f.params[p] = params
			if fn, ok := f.lists[p]; ok {
				return fn(params)
			}
			return esetPageResult{}, fmt.Errorf("HTTP 404: unexpected list %s", p)
		},
		get: func(_ context.Context, url string) (json.RawMessage, error) {
			p := exposurePathOf(url)
			if fn, ok := f.gets[p]; ok {
				return fn()
			}
			return nil, fmt.Errorf("HTTP 404: unexpected get %s", p)
		},
	}
}

func exposureStub(t *testing.T, target *func(*rootFlags) (exposureLiveAPI, error), f *exposureFakeLive) {
	t.Helper()
	orig := *target
	*target = func(*rootFlags) (exposureLiveAPI, error) { return f.api(), nil }
	t.Cleanup(func() { *target = orig })
}

func exposurePage(t *testing.T, truncated bool, rows ...map[string]any) func(map[string]string) (esetPageResult, error) {
	items := exposureItems(t, rows...)
	return func(map[string]string) (esetPageResult, error) {
		return esetPageResult{Items: items, Pages: 1, Truncated: truncated}, nil
	}
}

// exposureScopedPage emulates ESET's recursive deviceGroupUuid scoping over
// the fixture tree: only rows whose group is the requested group or sits
// anywhere under it (so a nested customer's rows come back too). Rows with
// a group the tree does not know are returned for any scope, as a live tree
// newer than the mirror would.
func exposureScopedPage(t *testing.T, rows ...map[string]any) func(map[string]string) (esetPageResult, error) {
	var groups []esetGroup
	exposureRoundTrip(t, exposureFixtureGroups(), &groups)
	parent := map[string]string{}
	for _, g := range groups {
		parent[g.UUID] = g.ParentUUID
	}
	under := func(g, root string) bool {
		if _, known := parent[g]; !known {
			return true
		}
		for cur := g; cur != ""; cur = parent[cur] {
			if cur == root {
				return true
			}
		}
		return false
	}
	return func(params map[string]string) (esetPageResult, error) {
		want := params["deviceGroupUuid"]
		kept := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			g, _ := r["deviceGroupUuid"].(string)
			if want == "" || under(g, want) {
				kept = append(kept, r)
			}
		}
		return esetPageResult{Items: exposureItems(t, kept...), Pages: 1}, nil
	}
}

func exposureRunCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// ---- pure logic ----

func TestExposureParseCVEs(t *testing.T) {
	got := exposureParseCVEs(" cve-2024-3400, CVE-2023-4863,,cve-2024-3400 ")
	want := []string{"CVE-2024-3400", "CVE-2023-4863"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := exposureParseCVEs(""); got == nil || len(got) != 0 {
		t.Fatalf("empty input should give an empty non-nil slice, got %#v", got)
	}
}

func TestExposureMatch(t *testing.T) {
	app := exposureVulnJSON("d", "g", "Google Chrome", "CVE-2024-3400", 90, "SEVERITY_LEVEL_HIGH", "")
	osv := exposureVulnJSON("d", "g", "os", "CVE-2024-3400", 70, "SEVERITY_LEVEL_MEDIUM", "")
	pkg := exposureVulnJSON("d", "g", "package", "CVE-2022-0778", 50, "SEVERITY_LEVEL_LOW", "")
	noCVE := exposureVulnJSON("d", "g", "Google Chrome", "", 10, "SEVERITY_LEVEL_LOW", "")
	vs := exposureDecodeFixture(t, app, osv, pkg, noCVE)
	cases := []struct {
		name string
		q    exposureQuery
		want []bool // app, os, package, noCVE
	}{
		{"cve matches application and os, case-insensitive", exposureQuery{CVEs: []string{"cve-2024-3400"}}, []bool{true, true, false, false}},
		{"cve matches package", exposureQuery{CVEs: []string{"CVE-2022-0778"}}, []bool{false, false, true, false}},
		{"multiple cves", exposureQuery{CVEs: []string{"CVE-2022-0778", "CVE-2024-3400"}}, []bool{true, true, true, false}},
		{"cve is exact, not a prefix", exposureQuery{CVEs: []string{"CVE-2024-340"}}, []bool{false, false, false, false}},
		{"app substring, case-insensitive, application only", exposureQuery{App: "CHROME"}, []bool{true, false, false, true}},
		{"app with exact version", exposureQuery{App: "chrome", Version: "126.0.6478.127"}, []bool{true, false, false, true}},
		{"app with other version", exposureQuery{App: "chrome", Version: "126.0"}, []bool{false, false, false, false}},
		{"app does not match package display name", exposureQuery{App: "openssl"}, []bool{false, false, false, false}},
		{"empty query matches nothing", exposureQuery{}, []bool{false, false, false, false}},
	}
	for _, c := range cases {
		for i, v := range vs {
			if got := exposureMatch(c.q, v); got != c.want[i] {
				t.Errorf("%s: vuln %d (%s) = %v, want %v", c.name, i, v.kind(), got, c.want[i])
			}
		}
	}
}

func TestExposureBuildCVEAcrossCustomers(t *testing.T) {
	scope := exposureFixtureScope(t)
	vulns := exposureDecodeFixture(t,
		exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_MEDIUM", "2026-09-10T08:00:00Z", "p-front"),
		exposureVulnJSON("dev-recep", "acme", "os", "cve-2024-3400", 95, "SEVERITY_LEVEL_HIGH", "2026-08-01T08:00:00Z"),
		// same device twice (two applications carry the CVE): counted once
		exposureVulnJSON("dev-front", "acme-hq", "Chromium", "CVE-2024-3400", 60, "SEVERITY_LEVEL_LOW", "2026-09-20T08:00:00Z", "p-front", "p-front-2"),
		exposureVulnJSON("dev-beta", "beta", "package", "CVE-2024-3400", 40, "SEVERITY_LEVEL_LOW", "2026-10-01T08:00:00Z"),
		// not the queried CVE
		exposureVulnJSON("dev-xray", "acme-hq", "Google Chrome", "CVE-2023-4863", 99, "SEVERITY_LEVEL_HIGH", "2026-01-01T00:00:00Z", "p-x"),
	)
	rows, totals := exposureBuild(exposureQuery{CVEs: []string{"CVE-2024-3400"}}, vulns, scope, 50)
	if totals.AffectedDevices != 3 || totals.Customers != 2 {
		t.Fatalf("totals = %+v, want 3 devices across 2 customers", totals)
	}
	acme, beta := rows[0], rows[1]
	if acme.Customer != "Acme Dental" || beta.Customer != "Beta Logistics" {
		t.Fatalf("order = %s, %s; want most affected (Acme) first", acme.Customer, beta.Customer)
	}
	if acme.AffectedDevices != 2 || acme.MaxRiskScore != 95 || acme.Severity != "HIGH" {
		t.Errorf("acme row = %+v", acme)
	}
	if acme.EarliestFirstDetect != "2026-08-01T08:00:00Z" {
		t.Errorf("earliest = %s", acme.EarliestFirstDetect)
	}
	if !acme.PatchAvailable || !reflect.DeepEqual(acme.AvailablePatchUUIDs, []string{"p-front", "p-front-2"}) {
		t.Errorf("patches = %v %v (want deduped)", acme.PatchAvailable, acme.AvailablePatchUUIDs)
	}
	wantDevices := []exposureDeviceRef{{UUID: "dev-front", Name: "FRONTDESK-01"}, {UUID: "dev-recep", Name: "RECEPTION-02"}}
	if !reflect.DeepEqual(acme.Devices, wantDevices) || acme.DevicesTruncated {
		t.Errorf("devices = %+v", acme.Devices)
	}
	if beta.PatchAvailable || len(beta.AvailablePatchUUIDs) != 0 || beta.AvailablePatchUUIDs == nil {
		t.Errorf("beta has no patch; got %+v", beta)
	}
	if !reflect.DeepEqual(acme.CVEs, []string{"CVE-2024-3400"}) {
		t.Errorf("cves = %v", acme.CVEs)
	}
}

func TestExposureBuildEdgeCases(t *testing.T) {
	scope := exposureFixtureScope(t)
	rows, totals := exposureBuild(exposureQuery{CVEs: []string{"CVE-2024-3400"}}, nil, scope, 50)
	if rows == nil || len(rows) != 0 || totals != (exposureTotals{}) {
		t.Fatalf("empty input: rows=%#v totals=%+v", rows, totals)
	}

	// Device cap, unknown device name, and a stale group falling back to the
	// device's synced parent group.
	vulns := exposureDecodeFixture(t,
		exposureVulnJSON("dev-front", "gone-group", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
		exposureVulnJSON("dev-xray", "acme-hq", "Zoom", "CVE-2024-1111", 30, "", "not-a-time"),
		exposureVulnJSON("dev-new", "acme-hq", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_UNKNOWN", ""),
	)
	rows, _ = exposureBuild(exposureQuery{App: "zoom"}, vulns, scope, 2)
	if len(rows) != 1 || rows[0].Customer != "Acme Dental" {
		t.Fatalf("stale group should fall back to the device's group: %+v", rows)
	}
	r := rows[0]
	if r.AffectedDevices != 3 || len(r.Devices) != 2 || !r.DevicesTruncated {
		t.Fatalf("cap: %+v", r)
	}
	if r.Devices[0].UUID != "dev-new" || r.Devices[0].Name != "" {
		t.Errorf("unknown device should sort first with an empty name: %+v", r.Devices)
	}
	if r.EarliestFirstDetect != "" || r.Severity != "LOW" {
		t.Errorf("unparseable times ignored, unknown severity ranks lowest: %+v", r)
	}

	// A group the tree does not know, on a device the mirror does not know,
	// is never guessed: it gets its own UUID-less "(group not in synced tree)"
	// row. A known group outranks the device's parent group, and a known
	// group outside every COMPANY is "(unassigned)", a different bucket.
	vulns = exposureDecodeFixture(t,
		exposureVulnJSON("dev-front", "gone-group", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
		exposureVulnJSON("dev-ghost", "gone-group", "Zoom", "CVE-2024-1111", 70, "SEVERITY_LEVEL_HIGH", ""),
		exposureVulnJSON("dev-beta", "acme-hq", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
		exposureVulnJSON("dev-msp", "msp", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
	)
	rows, totals = exposureBuild(exposureQuery{App: "zoom"}, vulns, scope, 0)
	got := map[string]exposureCustomerRow{}
	for _, r := range rows {
		got[r.Customer] = r
	}
	// Customers counts Acme only: (unassigned), the MSP's own device, is
	// listed but is not a customer (customer-counts-exclude-unassigned); the
	// unknown-group row's device is unknown-owner.
	if len(rows) != 3 || totals != (exposureTotals{AffectedDevices: 4, Customers: 1, UnknownOwnerDevices: 1}) {
		t.Fatalf("want Acme, the unknown-group bucket and (unassigned): %+v %+v", rows, totals)
	}
	if a := got["Acme Dental"]; a.CustomerUUID != "acme" || a.AffectedDevices != 2 || !reflect.DeepEqual(a.Devices, []exposureDeviceRef{{UUID: "dev-beta", Name: "BETA-WH-01"}, {UUID: "dev-front", Name: "FRONTDESK-01"}}) {
		t.Errorf("acme row = %+v", a)
	}
	if u := got[esetCustomerUnknownGroup]; u.CustomerUUID != "" || u.AffectedDevices != 1 || u.MaxRiskScore != 70 || u.Severity != "HIGH" || !reflect.DeepEqual(u.Devices, []exposureDeviceRef{{UUID: "dev-ghost"}}) {
		t.Errorf("unknown group row = %+v", u)
	}
	if n := got[esetCustomerNone]; n.CustomerUUID != "" || n.AffectedDevices != 1 {
		t.Errorf("a known group outside every customer is (unassigned): %+v", n)
	}
}

func TestExposureCustomerForNearestCompany(t *testing.T) {
	scope := exposureFixtureScope(t)
	// A device synced into a group the synced tree does not hold (the device
	// list was synced after the group list).
	scope.devices["dev-orphan"] = esetDevice{UUID: "dev-orphan", ParentGroupUUID: "group-after-sync"}
	cases := []struct {
		name, group, device, wantUUID, wantName string
	}{
		{"nested company keeps its own rows", "acme-lab", "dev-lab", "acme-lab", "Acme Labs"},
		{"site resolves to its company", "acme-hq", "", "acme", "Acme Dental"},
		{"another known customer is never reassigned", "beta", "dev-beta", "beta", "Beta Logistics"},
		{"a known group outranks the device's parent", "beta", "dev-front", "beta", "Beta Logistics"},
		{"known group outside every company is unassigned", "msp", "dev-front", "", esetCustomerNone},
		{"unknown group falls back to the device's synced parent", "brand-new-group", "dev-front", "acme", "Acme Dental"},
		{"no group uses the device's synced parent", "", "dev-beta", "beta", "Beta Logistics"},
		{"unknown group and unknown device are not guessed", "brand-new-group", "dev-gone", "", esetCustomerUnknownGroup},
		{"no group and unknown device are not guessed", "", "dev-gone", "", esetCustomerUnknownGroup},
		{"unknown group whose device's parent is unknown too", "brand-new-group", "dev-orphan", "", esetCustomerUnknownGroup},
		{"no group and a device whose parent is unknown", "", "dev-orphan", "", esetCustomerUnknownGroup},
	}
	for _, c := range cases {
		if got := scope.customerFor(c.group, c.device); got.UUID != c.wantUUID || got.Name != c.wantName {
			t.Errorf("%s: customerFor(%q, %q) = %+v, want uuid %q name %q", c.name, c.group, c.device, got, c.wantUUID, c.wantName)
		}
	}
	unsynced := newExposureScope(nil, nil)
	unsynced.unsynced = true
	if got := unsynced.customerFor("acme", "dev-front"); got.Name != exposureUnsyncedBucket || got.UUID != "" {
		t.Errorf("unsynced scope = %+v", got)
	}
}

func TestExposureFilterRows(t *testing.T) {
	scope := exposureFixtureScope(t)
	vulns := exposureDecodeFixture(t,
		exposureVulnJSON("dev-front", "acme-hq", "Zoom", "CVE-2024-1111", 30, "", ""),
		// nested customer: Acme Labs', not Acme's
		exposureVulnJSON("dev-lab", "acme-lab", "Zoom", "CVE-2024-1111", 30, "", ""),
		// a group created after the last sync on an unsynced device: ownership
		// cannot be proven, so it is not credited to the target
		exposureVulnJSON("dev-new", "group-after-sync", "Zoom", "CVE-2024-1111", 30, "", ""),
		// a new group on a synced Acme device: attributed through the device
		exposureVulnJSON("dev-xray", "group-after-sync", "Zoom", "CVE-2024-1111", 30, "", ""),
	)
	kept, other, unknown := exposureFilterRows(vulns, exposureVulnKey, scope, "acme")
	ids := make([]string, 0, len(kept))
	for _, v := range kept {
		ids = append(ids, v.DeviceUUID)
	}
	if !reflect.DeepEqual(ids, []string{"dev-front", "dev-xray"}) || other != 1 || len(unknown) != 1 || unknown[0].DeviceUUID != "dev-new" {
		t.Fatalf("kept = %v other = %d unknown = %+v; want the nested Acme Labs row as another customer's and the unknown-group row apart as owner-unknown", ids, other, unknown)
	}
	if kept, other, unknown := exposureFilterRows(vulns, exposureVulnKey, scope, "acme-lab"); len(kept) != 1 || kept[0].DeviceUUID != "dev-lab" || other != 2 || len(unknown) != 1 {
		t.Errorf("--customer Acme Labs keeps only its own row: %+v other %d unknown %d", kept, other, len(unknown))
	}
	if all, n, unknown := exposureFilterRows(vulns, exposureVulnKey, scope, ""); len(all) != 4 || n != 0 || len(unknown) != 0 {
		t.Errorf("no target keeps every row: %d %d %d", len(all), n, len(unknown))
	}
	scope.unsynced = true
	if all, n, unknown := exposureFilterRows(vulns, exposureVulnKey, scope, "acme"); len(all) != 4 || n != 0 || len(unknown) != 0 {
		t.Errorf("an unsynced scope cannot attribute, so it keeps every row: %d %d %d", len(all), n, len(unknown))
	}
}

func TestExposureDecodeVulnsCountsRejects(t *testing.T) {
	good := exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "")
	badCVE := exposureVulnJSON("dev-recep", "acme", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "")
	badCVE["applicationVulnerability"].(map[string]any)["cveNumber"] = 20243400
	badRisk := exposureVulnJSON("dev-xray", "acme-hq", "os", "CVE-2024-3400", 0, "", "")
	badRisk["osVulnerability"].(map[string]any)["riskScore"] = "high"
	var rej esetRejects
	got := exposureDecodeVulns(exposureItems(t, good, badCVE, badRisk), &rej)
	if len(got) != 1 || got[0].DeviceUUID != "dev-front" {
		t.Fatalf("decoded = %+v", got)
	}
	if rej.N != 2 || !strings.HasPrefix(rej.First, "vulnerability row 1:") {
		t.Errorf("rejects = %+v; want 2, first labelled vulnerability row 1", rej)
	}
	if empty := exposureDecodeVulns(nil, &esetRejects{}); empty == nil || len(empty) != 0 {
		t.Errorf("empty input: %#v", empty)
	}

	// null and scalar rows decode "successfully" into an empty row, and a row
	// without exactly one variant names no software: all are rejected, never
	// read as a vulnerability that simply does not match.
	rej = esetRejects{}
	items := append(exposureItems(t, good),
		json.RawMessage(`null`),
		json.RawMessage(` 7 `),
		json.RawMessage(`"CVE-2024-3400"`),
		json.RawMessage(`{"deviceUuid":"dev-x","deviceGroupUuid":"acme","availablePatchUuids":["p-1"]}`),
		json.RawMessage(`{"deviceUuid":"dev-x","applicationVulnerability":null,"osVulnerability":null}`),
		json.RawMessage(`{"deviceUuid":"dev-x","osVulnerability":{"cveNumber":"CVE-2024-3400"},"packageVulnerability":{"cveNumber":"CVE-2024-3400"}}`),
		json.RawMessage(`{"deviceUuid":"dev-y","packageVulnerability":{}}`),
	)
	got = exposureDecodeVulns(items, &rej)
	if len(got) != 2 || got[0].DeviceUUID != "dev-front" || got[1].DeviceUUID != "dev-y" || got[1].kind() != "package" {
		t.Fatalf("only the good row and the empty-but-present package variant decode: %+v", got)
	}
	if rej.N != 6 || rej.First != "vulnerability row 1: not a JSON object" {
		t.Errorf("rejects = %+v; want 6, first naming the null row", rej)
	}
	for _, raw := range items[4:7] {
		var r esetRejects
		if out := exposureDecodeVulns([]json.RawMessage{raw}, &r); len(out) != 0 || r.N != 1 || !strings.Contains(r.First, "applicationVulnerability") {
			t.Errorf("variant check: %s -> %+v %+v", raw, out, r)
		}
	}
}

// A null, scalar or variant-less row in the live list is counted in
// rejected_rows and named in the note, so "no match" is never read as clean.
func TestExposureCommandRejectsNullScalarAndVariantlessRows(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       exposureFixtureDevices(time.Now()),
	})
	good := exposureItems(t, exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", ""))[0]
	variantless := json.RawMessage(`{"deviceUuid":"dev-beta","deviceGroupUuid":"beta","availablePatchUuids":["p-9"]}`)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": func(map[string]string) (esetPageResult, error) {
			return esetPageResult{Items: []json.RawMessage{good, json.RawMessage(`null`), json.RawMessage(`42`), variantless}, Pages: 1}, nil
		},
	}}
	exposureStub(t, &exposureNewLive, fake)

	out, errOut, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view exposureView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if view.ScannedVulnerabilities != 4 || view.RejectedRows != 3 || view.Totals != (exposureTotals{AffectedDevices: 1, Customers: 1}) || view.Customers[0].Customer != "Acme Dental" {
		t.Fatalf("view = %+v", view)
	}
	if !strings.Contains(view.Note, "3 vulnerability row(s) could not be decoded") {
		t.Errorf("note = %q", view.Note)
	}
	if !strings.Contains(errOut, "warning: 3 row(s) could not be decoded") || !strings.Contains(errOut, "vulnerability row 1: not a JSON object") {
		t.Errorf("stderr = %q", errOut)
	}

	// Only bad rows: no match, but the note says rows went unchecked.
	fake.lists["/v1/device-vulnerabilities"] = func(map[string]string) (esetPageResult, error) {
		return esetPageResult{Items: []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`"x"`), variantless}, Pages: 1}, nil
	}
	out, _, err = exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	view = exposureView{}
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Customers) != 0 || view.RejectedRows != 3 || !strings.Contains(view.Note, "no device vulnerability matched (3 scanned)") || !strings.Contains(view.Note, "3 vulnerability row(s) could not be decoded") {
		t.Errorf("all-bad view = %+v", view)
	}
}

func TestExposureIsUUIDAndResolveTarget(t *testing.T) {
	for in, want := range map[string]bool{
		"6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f": true,
		"6F1C2D3E-4A5B-4C6D-8E7F-9A0B1C2D3E4F": true,
		"Acme Dental":                          false,
		"6f1c2d3e4a5b4c6d8e7f9a0b1c2d3e4f":     false,
		"6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4g": false,
		"":                                     false,
	} {
		if exposureIsUUID(in) != want {
			t.Errorf("exposureIsUUID(%q) != %v", in, want)
		}
	}
	unsynced := newExposureScope(nil, nil)
	unsynced.unsynced = true
	if g, err := exposureResolveTarget(unsynced, " 6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f "); err != nil || g.UUID != "6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f" {
		t.Errorf("unsynced UUID: %+v %v", g, err)
	}
	var ce *cliError
	if _, err := exposureResolveTarget(unsynced, "Acme Dental"); !errors.As(err, &ce) || ce.code != 2 || !strings.Contains(err.Error(), "sync --resources devices,device-groups") {
		t.Errorf("unsynced name must be a usage error naming the sync command: %v", err)
	}
	if g, err := exposureResolveTarget(exposureFixtureScope(t), "acme dental"); err != nil || g.UUID != "acme" {
		t.Errorf("synced name: %+v %v", g, err)
	}
}

func TestExposureSingleTenantRollsUp(t *testing.T) {
	scope := newExposureScope([]esetGroup{{UUID: "all", Name: "All"}}, nil)
	vulns := exposureDecodeFixture(t, exposureVulnJSON("d1", "all", "Firefox", "CVE-2024-2222", 10, "SEVERITY_LEVEL_LOW", ""))
	rows, _ := exposureBuild(exposureQuery{CVEs: []string{"CVE-2024-2222"}}, vulns, scope, 50)
	if len(rows) != 1 || rows[0].Customer != esetCustomerTenant {
		t.Fatalf("single tenant: %+v", rows)
	}
}

func TestExposureNote(t *testing.T) {
	if n := exposureNote(0, 1000, 1, true, 0); !strings.Contains(n, "--max-scan-pages") {
		t.Errorf("zero matches + truncated should suggest a wider scan: %q", n)
	}
	if n := exposureNote(0, 10, 1, false, 0); !strings.Contains(n, "10 scanned") {
		t.Errorf("zero matches: %q", n)
	}
	if n := exposureNote(2, 1000, 3, true, 0); !strings.Contains(n, "incomplete") {
		t.Errorf("partial: %q", n)
	}
	if n := exposureNote(2, 10, 1, false, 0); n != "" {
		t.Errorf("complete scan with matches needs no note: %q", n)
	}
	if n := exposureNote(0, 10, 1, false, 3); !strings.Contains(n, "10 scanned") || !strings.Contains(n, "3 vulnerability row(s) could not be decoded") {
		t.Errorf("rejected rows must be named so no match is not read as clean: %q", n)
	}
	if n := exposureJoinNotes("", "a", "", "b"); n != "a; b" {
		t.Errorf("join = %q", n)
	}
}

// ---- command ----

func TestExposureCommandEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       exposureFixtureDevices(time.Now()),
	})
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposurePage(t, false,
			exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "2026-09-10T08:00:00Z", "p1"),
			exposureVulnJSON("dev-beta", "beta", "os", "CVE-2024-3400", 70, "SEVERITY_LEVEL_MEDIUM", "2026-09-11T08:00:00Z"),
			exposureVulnJSON("dev-xray", "acme-hq", "Google Chrome", "CVE-2023-4863", 99, "SEVERITY_LEVEL_HIGH", ""),
		),
	}}
	exposureStub(t, &exposureNewLive, fake)

	out, _, err := exposureRunCLI(t, "exposure", "--cve", "cve-2024-3400", "--db", db, "--json")
	if err != nil {
		t.Fatalf("exposure: %v\n%s", err, out)
	}
	var view exposureView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if view.Totals.AffectedDevices != 2 || view.Totals.Customers != 2 || view.ScannedVulnerabilities != 3 || view.Pages != 1 {
		t.Fatalf("view = %+v", view)
	}
	if view.Customers[0].Devices[0].Name == "" {
		t.Errorf("device names should come from the mirror: %+v", view.Customers[0].Devices)
	}
	if _, scoped := fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"]; scoped {
		t.Errorf("no --customer: the scan must be account-wide, got %v", fake.params)
	}

	// --customer scopes the live call to the COMPANY group; --app adds the
	// application scope.
	out, _, err = exposureRunCLI(t, "exposure", "--app", "chrome", "--customer", "acme", "--db", db, "--json")
	if err != nil {
		t.Fatalf("exposure --app: %v\n%s", err, out)
	}
	p := fake.params["/v1/device-vulnerabilities"]
	if p["deviceGroupUuid"] != "acme" || p["vulnerabilityScope"] != exposureScopeApplication {
		t.Errorf("params = %v", p)
	}
	view = exposureView{}
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Customers) != 1 || view.Customers[0].AffectedDevices != 2 || view.Query.Customer != "Acme Dental" {
		t.Errorf("app view = %+v", view)
	}
}

func TestExposureCommandUsageAndUnknownCustomer(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"exposure", "--cve", "CVE-2024-3400", "--app", "chrome"},
		{"exposure", "--json"},
		{"exposure", "--cve", "CVE-2024-3400", "--version", "1.0"},
	} {
		_, _, err := exposureRunCLI(t, args...)
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != 2 {
			t.Errorf("%v: want usage error (exit 2), got %v", args, err)
		}
	}
	_, _, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--customer", "nobody", "--db", exposureSeedDB(t, map[string][]map[string]any{"device-groups": exposureFixtureGroups()}))
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 3 {
		t.Errorf("unknown customer: want not-found (exit 3), got %v", err)
	}
}

// A missing mirror must not short-circuit to an empty result: the live scan
// still runs and every hit lands in the unsynced bucket by device UUID.
func TestExposureCommandNoMirrorStillScans(t *testing.T) {
	testenv.Isolate(t)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposurePage(t, false,
			exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "2026-09-10T08:00:00Z", "p1"),
			exposureVulnJSON("dev-beta", "beta", "os", "CVE-2024-3400", 70, "SEVERITY_LEVEL_MEDIUM", "2026-09-11T08:00:00Z"),
			exposureVulnJSON("dev-xray", "acme-hq", "Google Chrome", "CVE-2023-4863", 99, "SEVERITY_LEVEL_HIGH", ""),
		),
	}}
	exposureStub(t, &exposureNewLive, fake)

	missing := filepath.Join(t.TempDir(), "absent.db")
	out, errOut, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", missing, "--json")
	if err != nil {
		t.Fatalf("missing mirror: %v\n%s", err, out)
	}
	var view exposureView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	// No tree, so no customer is proven: the bucket's devices are
	// unknown-owner, never one customer.
	if view.ScannedVulnerabilities != 3 || view.Totals.AffectedDevices != 2 || view.Totals.Customers != 0 || view.Totals.UnknownOwnerDevices != 2 || len(view.Customers) != 1 {
		t.Fatalf("a missing mirror must still report live hits: %+v", view)
	}
	row := view.Customers[0]
	if row.Customer != exposureUnsyncedBucket || row.CustomerUUID != "" {
		t.Errorf("bucket = %+v", row)
	}
	wantDevices := []exposureDeviceRef{{UUID: "dev-beta"}, {UUID: "dev-front"}}
	if !reflect.DeepEqual(row.Devices, wantDevices) {
		t.Errorf("devices are keyed by UUID with no names: %+v", row.Devices)
	}
	if !strings.Contains(view.Note, "sync --resources devices,device-groups") || !strings.Contains(view.SyncedHint, "no local mirror") {
		t.Errorf("note = %q synced_hint = %q", view.Note, view.SyncedHint)
	}
	if !strings.Contains(errOut, "sync") {
		t.Errorf("missing mirror should still print the sync hint, got %q", errOut)
	}
	if _, scoped := fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"]; scoped {
		t.Errorf("no --customer: account-wide scan, got %v", fake.params)
	}

	// --customer with no tree: a group UUID scopes the live call ...
	const groupUUID = "6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	out, _, err = exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--customer", groupUUID, "--db", missing, "--json")
	if err != nil {
		t.Fatalf("--customer UUID without a mirror: %v\n%s", err, out)
	}
	if got := fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"]; got != groupUUID {
		t.Errorf("deviceGroupUuid = %q, want %s", got, groupUUID)
	}
	view = exposureView{}
	_ = json.Unmarshal([]byte(out), &view)
	if view.Query.Customer != groupUUID || len(view.Customers) != 1 || view.Customers[0].Customer != exposureUnsyncedBucket || !strings.Contains(view.Note, "nested customers") {
		t.Errorf("scoped unsynced view = %+v", view)
	}

	// ... and a name is refused before any live call.
	fake.params = nil
	_, _, err = exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--customer", "Acme Dental", "--db", missing, "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 2 {
		t.Errorf("a customer name without a mirror must be a usage error, got %v", err)
	}
	if len(fake.params) != 0 {
		t.Errorf("a refused name must not reach ESET: %v", fake.params)
	}

	// A mirror with no device groups is the same unsynced state.
	noGroups := exposureSeedDB(t, map[string][]map[string]any{"devices": exposureFixtureDevices(time.Now())})
	out, _, err = exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", noGroups, "--json")
	if err != nil {
		t.Fatal(err)
	}
	view = exposureView{}
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Customers) != 1 || view.Customers[0].Customer != exposureUnsyncedBucket || !strings.Contains(view.SyncedHint, "no device groups") {
		t.Errorf("no-groups view = %+v", view)
	}
}

func TestExposureCommandNestedCustomerAndRejects(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       exposureFixtureDevices(time.Now()),
	})
	bad := exposureVulnJSON("dev-recep", "acme", "Google Chrome", "CVE-2024-3400", 90, "SEVERITY_LEVEL_HIGH", "")
	bad["applicationVulnerability"].(map[string]any)["cveNumber"] = []string{"CVE-2024-3400"}
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposureScopedPage(t,
			exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", ""),
			// Acme Labs is a customer nested under Acme: the recursive scope
			// returns its row, but it is Acme Labs' exposure, not Acme's.
			exposureVulnJSON("dev-lab", "acme-lab", "Google Chrome", "CVE-2024-3400", 95, "SEVERITY_LEVEL_HIGH", ""),
			// A group created after the last sync, on a device the mirror does
			// not know: the recursive scope returns it for Acme, but that does
			// not prove Acme is its nearest COMPANY, so it is never Acme's.
			exposureVulnJSON("dev-new", "group-after-sync", "Google Chrome", "CVE-2024-3400", 70, "SEVERITY_LEVEL_HIGH", ""),
			bad,
			exposureVulnJSON("dev-beta", "beta", "Google Chrome", "CVE-2024-3400", 60, "SEVERITY_LEVEL_HIGH", ""),
		),
	}}
	exposureStub(t, &exposureNewLive, fake)

	out, errOut, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--customer", "Acme Dental", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view exposureView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(view.Customers) != 1 || view.Customers[0].CustomerUUID != "acme" || view.Customers[0].AffectedDevices != 1 || view.Customers[0].MaxRiskScore != 80 ||
		!reflect.DeepEqual(view.Customers[0].Devices, []exposureDeviceRef{{UUID: "dev-front", Name: "FRONTDESK-01"}}) {
		t.Fatalf("only Acme's own row counts (not the nested customer's, not the unknown group's): %+v", view.Customers)
	}
	// unknown_owner_devices counts the unknown-group row's device before
	// --customer drops the row, so it agrees with unattributed_device_uuids
	// (adversarial round 4).
	if view.Totals != (exposureTotals{AffectedDevices: 1, Customers: 1, UnknownOwnerDevices: 1}) {
		t.Errorf("totals = %+v", view.Totals)
	}
	// Acme Labs' row is another customer's; the unknown-group row may be
	// Acme's, so it is counted apart as unattributed, never as another
	// customer's (customer-views-count-unattributed-rows).
	if view.OtherCustomerRows != 1 || !strings.Contains(view.Note, "nested under Acme Dental") {
		t.Errorf("other_customer_rows = %d note = %q", view.OtherCustomerRows, view.Note)
	}
	if view.UnattributedRows != 1 || !reflect.DeepEqual(view.UnattributedDeviceUUIDs, []string{"dev-new"}) {
		t.Errorf("unattributed_rows = %d %v, want the unknown-group row", view.UnattributedRows, view.UnattributedDeviceUUIDs)
	}
	if view.RejectedRows != 1 || view.ScannedVulnerabilities != 4 {
		t.Errorf("rejected_rows = %d scanned = %d", view.RejectedRows, view.ScannedVulnerabilities)
	}
	if !strings.Contains(errOut, "1 row(s) could not be decoded") || !strings.Contains(errOut, "vulnerability row 3") {
		t.Errorf("stderr warning = %q", errOut)
	}

	// Without --customer the nested customer gets its own row.
	out, _, err = exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	view = exposureView{}
	_ = json.Unmarshal([]byte(out), &view)
	got := map[string]int{}
	for _, r := range view.Customers {
		got[r.Customer] = r.AffectedDevices
		if r.Customer == esetCustomerUnknownGroup && (r.CustomerUUID != "" || !reflect.DeepEqual(r.Devices, []exposureDeviceRef{{UUID: "dev-new"}})) {
			t.Errorf("unknown-group row = %+v", r)
		}
	}
	want := map[string]int{"Acme Dental": 1, "Acme Labs": 1, "Beta Logistics": 1, esetCustomerUnknownGroup: 1}
	if !reflect.DeepEqual(got, want) || view.OtherCustomerRows != 0 || view.RejectedRows != 1 || view.UnattributedRows != 1 {
		t.Errorf("account-wide rows = %v (other %d, rejected %d), want %v", got, view.OtherCustomerRows, view.RejectedRows, want)
	}
	if !strings.Contains(fmt.Sprint(view.Customers), "dev-lab") {
		t.Errorf("nested customer's device missing: %+v", view.Customers)
	}
}

func TestExposureHumanOutputAndDogfoodCap(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       exposureFixtureDevices(time.Now()),
	})
	var gotMax int
	fake := &exposureFakeLive{}
	exposureStub(t, &exposureNewLive, fake)
	orig := exposureNewLive
	exposureNewLive = func(f *rootFlags) (exposureLiveAPI, error) {
		api, err := orig(f)
		inner := api.list
		api.list = func(ctx context.Context, url string, params map[string]string, key string, maxPages int) (esetPageResult, error) {
			gotMax = maxPages
			return inner(ctx, url, params, key, maxPages)
		}
		return api, err
	}
	fake.lists = map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposurePage(t, true,
			exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "2026-09-10T08:00:00Z", "p1")),
	}
	out, _, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--max-scan-pages", "7", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CUSTOMER", "Acme Dental", "CVE-2024-3400", "incomplete"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if gotMax != 7 {
		t.Errorf("--max-scan-pages = %d, want 7", gotMax)
	}
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	if _, _, err := exposureRunCLI(t, "exposure", "--cve", "CVE-2024-3400", "--db", db, "--json"); err != nil {
		t.Fatal(err)
	}
	if gotMax != 1 {
		t.Errorf("dogfood should cap the scan at 1 page, got %d", gotMax)
	}
}
