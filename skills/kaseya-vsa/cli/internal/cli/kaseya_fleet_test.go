package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func mustAsset(t *testing.T, js string, synced time.Time) *fleetAsset {
	t.Helper()
	a := &fleetAsset{}
	if err := json.Unmarshal([]byte(js), a); err != nil {
		t.Fatal(err)
	}
	if a.hasUpdateData() {
		if err := json.Unmarshal(a.AvailableUpdates, &a.updates); err != nil {
			t.Fatal(err)
		}
	}
	if a.hasSoftwareData() {
		if err := json.Unmarshal(a.InstalledSoftware, &a.software); err != nil {
			t.Fatal(err)
		}
	}
	a.syncedAt = synced
	return a
}

func TestEvaluateHealth(t *testing.T) {
	synced := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	opts := healthOptions{checks: healthChecks, offlineFor: 7 * 24 * time.Hour, diskBelow: 10, cpuAbove: 90, memAbove: 90, now: synced}
	cases := []struct {
		name string
		js   string
		want map[string]string // check -> state
	}{
		{"healthy", `{"Identifier":"a","Name":"OK-1","LastSeenOnline":"2026-10-15T11:00:00Z","CpuUsage":10,"MemoryUsage":20,
			"FirewallEnabled":true,"AntivirusEnabled":"enabled","AntivirusUpToDate":"yes","UacEnabled":true,
			"Disks":[{"Name":"C:","FreePercentage":50}]}`, map[string]string{}},
		{"everything wrong", `{"Identifier":"b","Name":"BAD-1","LastSeenOnline":"2026-09-01T00:00:00Z","CpuUsage":99,"MemoryUsage":95,
			"FirewallEnabled":false,"AntivirusEnabled":"disabled","AntivirusUpToDate":"yes","UacEnabled":false,
			"Disks":[{"Name":"C:","FreePercentage":4},{"Name":"D:","FreePercentage":80}]}`,
			map[string]string{"offline": "fail", "av": "fail", "firewall": "fail", "uac": "fail", "disk": "fail", "cpu": "fail", "memory": "fail"}},
		{"stale definitions only", `{"Identifier":"c","Name":"AV-1","LastSeenOnline":"2026-10-15T11:00:00Z","CpuUsage":1,"MemoryUsage":1,
			"FirewallEnabled":true,"AntivirusEnabled":"enabled","AntivirusUpToDate":"no","UacEnabled":true,"Disks":[{"Name":"C:","FreePercentage":50}]}`,
			map[string]string{"av": "fail"}},
		// Missing or unrecognized values are unknown, never a pass.
		{"nothing reported", `{"Identifier":"d","Name":"BLANK-1","AntivirusEnabled":"weird-state"}`,
			map[string]string{"offline": "unknown", "av": "unknown", "firewall": "unknown", "uac": "unknown", "disk": "unknown", "cpu": "unknown", "memory": "unknown"}},
	}
	for _, tc := range cases {
		got := map[string]string{}
		for _, r := range evaluateHealth(mustAsset(t, tc.js, synced), opts) {
			got[r.Check] = r.State
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%s: check %s = %q, want %q (all: %v)", tc.name, k, got[k], v, got)
			}
		}
	}
}

// A disk that reports no free-space value is unknown, never healthy.
func TestEvaluateHealthNullDisk(t *testing.T) {
	a := mustAsset(t, `{"Identifier":"f","Name":"D","Disks":[{"Name":"C:","FreePercentage":null},{"Name":"D:","FreePercentage":50}]}`, time.Now())
	rows := evaluateHealth(a, healthOptions{checks: []string{"disk"}, diskBelow: 10})
	if len(rows) != 1 || rows[0].State != "unknown" {
		t.Fatalf("null disk measurement: %+v", rows)
	}
}

// Two low disks on one device are one disk failure.
func TestEvaluateHealthTwoLowDisks(t *testing.T) {
	a := mustAsset(t, `{"Identifier":"g","Name":"E","Disks":[{"Name":"C:","FreePercentage":4},{"Name":"D:","FreePercentage":2},{"Name":"E:","FreePercentage":null}]}`, time.Now())
	rows := evaluateHealth(a, healthOptions{checks: []string{"disk"}, diskBelow: 10})
	if len(rows) != 1 || rows[0].State != "fail" || rows[0].Value != "2" {
		t.Fatalf("two low disks: %+v", rows)
	}
}

// Offline age is measured from when the row was synced, so a stale mirror does
// not report a device that was online at sync time as offline.
func TestEvaluateHealthOfflineUsesSyncTime(t *testing.T) {
	synced := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := mustAsset(t, `{"Identifier":"e","Name":"X","LastSeenOnline":"2026-01-01T11:00:00Z"}`, synced)
	opts := healthOptions{checks: []string{"offline"}, offlineFor: time.Hour * 24, now: synced.Add(90 * 24 * time.Hour)}
	if rows := evaluateHealth(a, opts); len(rows) != 0 {
		t.Fatalf("device online at sync time reported %v", rows)
	}
}

func TestRankPatchExposure(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	assets := []*fleetAsset{
		mustAsset(t, `{"Identifier":"1","Name":"A","OrganizationId":1,"AvailableUpdates":[
			{"KbArticle":"5066835","Severity":"Critical","CvssScore":9.8,"CveCodes":["CVE-2026-1"],"ReleaseDate":"2026-10-14"},
			{"KbArticle":"5065422","Severity":"Important","CvssScore":7.5,"CveCodes":["CVE-2026-2"]}]}`, now),
		mustAsset(t, `{"Identifier":"2","Name":"B","OrganizationId":2,"AvailableUpdates":[
			{"KbArticle":"KB5066835","Severity":"Critical","CvssScore":9.1,"CveCodes":["CVE-2026-1"]}]}`, now),
		mustAsset(t, `{"Identifier":"3","Name":"C","OrganizationId":2,"AvailableUpdates":[]}`, now), // patched
		mustAsset(t, `{"Identifier":"4","Name":"D","OrganizationId":2}`, now),                       // never synced: no data
	}
	orgs := map[int64]string{1: "Acme", 2: "Bayside"}
	out, scanned, noData, withFindings := rankPatchExposure(assets, orgs, "", "kb", patchFilter{}, now)
	rows := out.([]patchKBRow)
	if scanned != 4 || noData != 1 || withFindings != 2 {
		t.Fatalf("scanned=%d noData=%d withFindings=%d", scanned, noData, withFindings)
	}
	if len(rows) != 2 || normalizeKB(rows[0].KB) != "5066835" || rows[0].Devices != 2 || rows[0].Organizations != 2 || *rows[0].MaxCvss != 9.8 {
		t.Fatalf("kb ranking wrong: %+v", rows)
	}
	if rows[0].DaysSinceRelease == nil || *rows[0].DaysSinceRelease != 1 {
		t.Fatalf("days since release: %v", rows[0].DaysSinceRelease)
	}
	// Filters: CVE, severity, and a KB nobody is missing.
	if out, _, _, n := rankPatchExposure(assets, orgs, "", "device", patchFilter{cve: "CVE-2026-2"}, now); n != 1 || len(out.([]patchDeviceRow)) != 1 {
		t.Fatalf("cve filter: %d %+v", n, out)
	}
	if _, _, _, n := rankPatchExposure(assets, orgs, "", "kb", patchFilter{severities: []string{"important"}}, now); n != 1 {
		t.Fatalf("severity filter matched %d devices", n)
	}
	if out, _, _, n := rankPatchExposure(assets, orgs, "", "kb", patchFilter{kb: "1234567"}, now); n != 0 || len(out.([]patchKBRow)) != 0 {
		t.Fatalf("unknown KB should match nothing, got %d", n)
	}
	// One device listing the same KB twice (two revisions) is still one exposed device.
	dup := []*fleetAsset{mustAsset(t, `{"Identifier":"1","Name":"A","OrganizationId":1,"AvailableUpdates":[{"KbArticle":"7"},{"KbArticle":"KB7"}]}`, now)}
	if out, _, _, _ := rankPatchExposure(dup, orgs, "", "kb", patchFilter{}, now); out.([]patchKBRow)[0].Devices != 1 {
		t.Fatalf("duplicate KB on one device counted twice: %+v", out)
	}
	// --sort cvss puts a severe KB on one device above a milder KB on more devices.
	ranked := []*fleetAsset{
		mustAsset(t, `{"Identifier":"1","Name":"A","OrganizationId":1,"AvailableUpdates":[{"KbArticle":"1","CvssScore":9.8},{"KbArticle":"2","CvssScore":7.5}]}`, now),
		mustAsset(t, `{"Identifier":"2","Name":"B","OrganizationId":1,"AvailableUpdates":[{"KbArticle":"2","CvssScore":7.5}]}`, now),
	}
	if out, _, _, _ := rankPatchExposure(ranked, orgs, "", "kb", patchFilter{}, now); out.([]patchKBRow)[0].KB != "2" {
		t.Fatalf("default sort should rank by devices: %+v", out)
	}
	if out, _, _, _ := rankPatchExposure(ranked, orgs, "", "kb", patchFilter{sortCvss: true}, now); out.([]patchKBRow)[0].KB != "1" {
		t.Fatalf("--sort cvss should rank by CVSS: %+v", out)
	}
	if _, err := parseSeverities("crit,important"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSeverities("critcal"); err == nil {
		t.Fatal("severity typo accepted")
	}
	if out, scanned, _, _ := rankPatchExposure(assets, orgs, "acme", "org", patchFilter{}, now); scanned != 1 || len(out.([]patchOrgRow)) != 1 {
		t.Fatalf("org filter scanned %d", scanned)
	}
}

func TestSearchSoftware(t *testing.T) {
	now := time.Now()
	assets := []*fleetAsset{
		mustAsset(t, `{"Identifier":"1","Name":"A","OrganizationId":1,"InstalledSoftware":[{"Name":"Google Chrome","Version":"141.0.7390.66"},{"Name":"Huntress Agent","Version":"0.14"}]}`, now),
		mustAsset(t, `{"Identifier":"2","Name":"B","OrganizationId":1,"InstalledSoftware":[{"Name":"Google Chrome","Version":"118.0.5993.70"}]}`, now),
		mustAsset(t, `{"Identifier":"3","Name":"C","OrganizationId":1}`, now), // no inventory: never "absent"
	}
	orgs := map[int64]string{1: "Acme"}
	v := softwareView{Mode: "absent", Pattern: "huntress agent", Match: "exact"}
	searchSoftware(&v, assets, orgs, "", "")
	if rows := v.Results.([]softwareAbsentRow); len(rows) != 1 || rows[0].Device != "B" || v.NoInventory != 1 ||
		v.MatchingDevices != 1 || v.AbsentDevices != 1 {
		t.Fatalf("absent: %+v noInventory=%d", v.Results, v.NoInventory)
	}
	v = softwareView{Mode: "below", Pattern: "google chrome", Match: "exact"}
	searchSoftware(&v, assets, orgs, "", "140.0")
	if rows := v.Results.([]softwareInstallRow); len(rows) != 1 || rows[0].Version != "118.0.5993.70" {
		t.Fatalf("below: %+v", v.Results)
	}
	// --below: a device with the app at a newer version still has the app.
	v = softwareView{Mode: "below", Pattern: "google chrome", Match: "exact"}
	searchSoftware(&v, assets, orgs, "", "140.0")
	if v.MatchingDevices != 2 || v.AbsentDevices != 0 || v.BelowDevices != 1 {
		t.Fatalf("below counts: with=%d without=%d below=%d", v.MatchingDevices, v.AbsentDevices, v.BelowDevices)
	}
	v = softwareView{Mode: "versions", Pattern: "CHROME", Match: "contains"}
	searchSoftware(&v, assets, orgs, "", "")
	if rows := v.Results.([]softwareVersionRow); len(rows) != 2 || rows[0].Version != "141.0.7390.66" {
		t.Fatalf("versions: %+v", v.Results)
	}
	// Exact match must not pick up a different product that merely contains the name.
	extra := mustAsset(t, `{"Identifier":"9","Name":"Z","OrganizationId":1,"InstalledSoftware":[{"Name":"Chrome Remote Desktop Host","Version":"120.0"}]}`, now)
	v = softwareView{Mode: "installs", Pattern: "Google Chrome", Match: "exact"}
	searchSoftware(&v, append(assets, extra), orgs, "", "")
	if rows := v.Results.([]softwareInstallRow); len(rows) != 2 {
		t.Fatalf("exact match leaked another product: %+v", rows)
	}
	v = softwareView{Mode: "installs", Pattern: "no such app", Match: "contains"}
	searchSoftware(&v, assets, orgs, "", "")
	if rows := v.Results.([]softwareInstallRow); len(rows) != 0 || v.MatchingDevices != 0 {
		t.Fatalf("negative search matched: %+v", v.Results)
	}
}

func TestBuildAuditDigest(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	entries := []auditEntry{
		{RecordID: 1, DateTime: "2026-10-15T10:00:00", User: "admin", CategoryID: 6, EventTypeID: 1},
		{RecordID: 2, DateTime: "2026-10-14T10:00:00", User: "admin", CategoryID: 1, EventTypeID: 3},
		{RecordID: 3, DateTime: "2026-10-13T10:00:00", User: "tech", CategoryID: 99, EventTypeID: 2},
		{RecordID: 4, DateTime: "2026-09-01T10:00:00", User: "tech", CategoryID: 6, EventTypeID: 1}, // outside window
		{RecordID: 5, DateTime: "not a date", User: "tech", CategoryID: 1, EventTypeID: 1},
	}
	// Same instant written two ways must sort by instant, then record ID.
	ordered := buildAuditDigest([]auditEntry{
		{RecordID: 10, DateTime: "2026-10-15T09:00:00Z", User: "a", CategoryID: 6},
		{RecordID: 11, DateTime: "2026-10-15T10:00:00.5", User: "a", CategoryID: 6},
		{RecordID: 12, DateTime: "2026-10-15T09:30:00", User: "a", CategoryID: 6},
	}, now.Add(-24*time.Hour), now, "", 0, 0)
	if ordered.Sensitive[0].RecordID != 11 || ordered.Sensitive[2].RecordID != 10 {
		t.Fatalf("digest order: %+v", ordered.Sensitive)
	}
	d := buildAuditDigest(entries, now.Add(-7*24*time.Hour), now, "", 0, 0)
	if d.Entries != 3 || d.Unparsed != 1 {
		t.Fatalf("entries=%d unparsed=%d", d.Entries, d.Unparsed)
	}
	if d.ByCategory["Remote Control"] != 1 || d.ByCategory["category 99"] != 1 {
		t.Fatalf("by category: %v", d.ByCategory)
	}
	if len(d.Sensitive) != 1 || len(d.ErrorsAndWarning) != 2 {
		t.Fatalf("sensitive=%d errors_and_warnings=%d", len(d.Sensitive), len(d.ErrorsAndWarning))
	}
	if d.Users[0].User != "admin" || d.Users[0].Errors != 1 {
		t.Fatalf("users: %+v", d.Users)
	}
	if got := buildAuditDigest(entries, now.Add(-7*24*time.Hour), now, "TECH", 0, 0); got.Entries != 1 {
		t.Fatalf("user filter: %d", got.Entries)
	}
	if id, err := parseAuditCategory("remote control"); err != nil || id != 6 {
		t.Fatalf("parseAuditCategory: %d %v", id, err)
	}
}

func TestRunWaitHelpers(t *testing.T) {
	if !executionDone(map[string]any{"State": "Running", "EndTime": "2026-10-15T00:00:00Z"}) ||
		!executionDone(map[string]any{"Endtime": "x"}) || !executionDone(map[string]any{"State": "Stopped"}) ||
		executionDone(map[string]any{"State": "Running"}) || executionDone(map[string]any{"State": "Pending", "EndTime": ""}) {
		t.Fatal("executionDone")
	}
	for in, want := range map[any]int{float64(0): 0, "1": 1, " 5 ": 5} {
		if got, ok := exitCodeValue(in); !ok || got != want {
			t.Fatalf("exitCodeValue(%v) = %d %v", in, got, ok)
		}
	}
	if _, ok := exitCodeValue(nil); ok {
		t.Fatal("nil exit code must be unknown")
	}
	vars, err := parseScriptVars([]string{"251=30", "252=a=b"})
	if err != nil || len(vars) != 2 || vars[1]["Value"] != "a=b" || vars[0]["Id"] != 251 {
		t.Fatalf("parseScriptVars: %v %v", vars, err)
	}
	if _, err := parseScriptVars([]string{"abc=1"}); err == nil {
		t.Fatal("non-numeric variable id accepted")
	}
}

func TestFleetHelpers(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"141.0.7390.66", "140.0", 1}, {"1.2", "1.2.0", 0}, {"118.0.5993.70", "140", -1}, {"9.0.4b", "9.0.4", 0}} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q,%q) = %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if odataQuote("o'brien") != "'o''brien'" {
		t.Fatal("odataQuote")
	}
	if string(vsaData(json.RawMessage(`{"Data":[1],"Meta":{"ResponseCode":200}}`))) != "[1]" {
		t.Fatal("vsaData")
	}
	if !orgFilter("102", 102, "x") || orgFilter("102", 101, "x") || !orgFilter("dental", 1, "Acme Dental") {
		t.Fatal("orgFilter")
	}
	if ts, ok := parseVSATime("2024-12-27T16:03:39"); !ok || ts.Location() != time.UTC {
		t.Fatal("zone-less audit timestamps must parse as UTC")
	}
}
