// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"watchguard-pp-cli/internal/store"
)

func mustMaps(t *testing.T, s, key string) []map[string]any {
	t.Helper()
	if !json.Valid([]byte(s)) {
		t.Fatalf("fixture is not valid JSON: %s", s)
	}
	return asMaps(json.RawMessage(s), key)
}

func TestAsMapsEnvelopes(t *testing.T) {
	cases := []struct {
		name, body, key string
		want            int
	}{
		{"data array", `{"data":[{"id":"a"},{"id":"b"}],"total_items":2}`, "data", 2},
		{"bare array", `[{"id":"a"}]`, "data", 1},
		{"keyless object", `{"total_items":0}`, "data", 0},
		{"items", `{"items":[{"accountId":"ACC-1"}],"pageControls":{"totalItems":1}}`, "items", 1},
		{"wrapped rows", `[{"data":{"id":"a"}},{"data":{"id":"b"}}]`, "data", 2},
		{"empty", `{"data":[]}`, "data", 0},
	}
	for _, c := range cases {
		if got := len(mustMaps(t, c.body, c.key)); got != c.want {
			t.Errorf("%s: got %d rows, want %d", c.name, got, c.want)
		}
	}
}

func TestProtectionIssues(t *testing.T) {
	healthy := map[string]any{"accumulated_protection_status": 1.0, "isolation_state": 0.0, "protection_engine_update_status": 2.0, "license_status": 0.0}
	if got := protectionIssues(healthy); len(got) != 0 {
		t.Fatalf("healthy device flagged: %v", got)
	}
	sick := map[string]any{"accumulated_protection_status": 5.0, "isolation_state": 1.0, "protection_engine_update_status": 1.0, "license_status": 1.0, "reboot_requested": true}
	want := []string{"protection without license", "isolated", "engine outdated", "license not assigned", "reboot requested"}
	if got := protectionIssues(sick); !reflect.DeepEqual(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
}

func TestPostureFromRows(t *testing.T) {
	sub := subscriber{ID: "ACC-1", Name: "Acme"}
	prot := mustMaps(t, `{"data":[
		{"host_name":"OK-1","device_id":"d1","accumulated_protection_status":1,"isolation_state":0,"protection_engine_update_status":2,"license_status":0},
		{"host_name":"BAD-1","device_id":"d2","accumulated_protection_status":2,"isolation_state":0,"protection_engine_update_status":2,"license_status":0}]}`, "data")
	unmanaged := mustMaps(t, `{"data":[{"host_name":"PRINTER","device_id":"u1","status":1}]}`, "data")
	rows := postureFromRows(sub, prot, unmanaged, false)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (BAD-1 + unmanaged)", len(rows))
	}
	for _, r := range rows {
		if r.HostName == "OK-1" {
			t.Fatalf("healthy device leaked into at-risk list")
		}
		if r.AccountName != "Acme" {
			t.Fatalf("account name not tagged: %+v", r)
		}
	}
	if rows[0].Protection != "deactivated" || rows[1].Managed || rows[1].Issues[0] != "unmanaged: no endpoint agent (discovered)" {
		t.Fatalf("unexpected decode: %+v", rows)
	}
	if got := postureFromRows(sub, prot, nil, true); len(got) != 2 {
		t.Fatalf("--all should keep healthy rows, got %d", len(got))
	}
}

func TestPivotExposure(t *testing.T) {
	a := accountPatches{sub: subscriber{ID: "ACC-1"}, rows: mustMaps(t, `{"data":[
		{"device_id":"d1","host_name":"H1","patch_name":"KB1","patch_criticality":201,"patch_cve_ids":["CVE-1","CVE-2"]},
		{"device_id":"d2","host_name":"H2","patch_name":"KB1","patch_criticality":201,"patch_cve_ids":["CVE-1"]},
		{"device_id":"d1","host_name":"H1","patch_name":"Reader","patch_criticality":101,"patch_cve_ids":[]}]}`, "data")}
	b := accountPatches{sub: subscriber{ID: "ACC-2"}, rows: mustMaps(t, `{"data":[
		{"device_id":"d9","host_name":"H9","patch_name":"KB2","patch_criticality":204,"patch_cve_ids":["CVE-1"]}]}`, "data")}
	rows := pivotExposure([]accountPatches{a, b}, "", 0)
	if len(rows) != 3 || rows[0].CVE != "CVE-1" || rows[0].Devices != 3 || rows[0].Accounts != 2 || rows[0].Criticality != "critical" {
		t.Fatalf("unexpected pivot: %+v", rows)
	}
	if rows[2].CVE != "no-cve:Reader" {
		t.Fatalf("CVE-less patch should be grouped last by name, got %+v", rows[2])
	}
	if got := pivotExposure([]accountPatches{a, b}, "CVE-2", 0); len(got) != 1 || got[0].Devices != 1 {
		t.Fatalf("cve filter: %+v", got)
	}
	if got := pivotExposure([]accountPatches{a, b}, "", 7); len(got) != 2 {
		t.Fatalf("min critical should drop low and non-security rows: %+v", got)
	}
	if got := pivotExposure(nil, "", 0); got == nil || len(got) != 0 {
		t.Fatalf("empty input must be [] not nil")
	}
}

func TestQueueFromIncidents(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	items := mustMaps(t, `{"items":[
	 {"id":"low-old","account":"ACC-1","severity":"Low","threatScore":2,"status":"new","timestamp":"2026-10-01T12:00:00Z"},
	 {"id":"crit","account":"ACC-2","severity":"Critical","threatScore":9,"status":"new","timestamp":"2026-10-14T10:00:00Z",
	  "entities":{"g1":{"recommendedActions":[{"command":"isolate"},{"command":"quarantineFile"}],"actionPerformed":{"command":"quarantineFile"}}}},
	 {"id":"closed","account":"ACC-1","severity":"Critical","threatScore":10,"status":"closed","timestamp":"2026-10-14T09:00:00Z"},
	 {"id":"med-read","account":"ACC-1","severity":"Medium","threatScore":5,"status":"read","timestamp":"2026-10-12T12:00:00Z","comments":[{"id":"c1"}]}]}`, "items")
	q := queueFromIncidents(items, map[string]bool{"new": true, "read": true}, 0, map[string]string{"ACC-2": "Beta"}, now)
	ids := []string{}
	for _, r := range q {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"crit", "med-read", "low-old"}) {
		t.Fatalf("ranking = %v", ids)
	}
	crit := q[0]
	if crit.AccountName != "Beta" || crit.Untouched || !reflect.DeepEqual(crit.RecommendedNotPerformed, []string{"isolate"}) || !reflect.DeepEqual(crit.Performed, []string{"quarantineFile"}) {
		t.Fatalf("crit row = %+v", crit)
	}
	if !q[2].Untouched || q[1].Untouched || q[2].AgeHours != 312 {
		t.Fatalf("untouched/age wrong: %+v / %+v", q[1], q[2])
	}
	if got := queueFromIncidents(items, map[string]bool{"new": true}, sevRank("high"), nil, now); len(got) != 1 || got[0].ID != "crit" {
		t.Fatalf("severity filter: %+v", got)
	}
}

func TestDiffFleet(t *testing.T) {
	prev := map[string]fleetDevice{
		"FB-1": {ID: "FB-1", State: "online", Version: "12.10.4"},
		"FB-2": {ID: "FB-2", State: "offline"},
		"FB-3": {ID: "FB-3", State: "online"},
	}
	cur := map[string]fleetDevice{
		"FB-1": {ID: "FB-1", State: "offline", Version: "12.11.0", OfflineSince: "2026-10-14T01:00:00Z"},
		"FB-2": {ID: "FB-2", State: "online"},
		"FB-4": {ID: "FB-4", State: "never connected"},
	}
	got := map[string]string{}
	for _, c := range diffFleet(prev, cur) {
		got[c.DeviceID+"/"+c.Change] = c.To
	}
	want := map[string]string{"FB-1/went_offline": "offline", "FB-1/firmware_changed": "12.11.0", "FB-2/came_online": "online", "FB-4/new": "never connected", "FB-3/removed": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
	if d := diffFleet(cur, cur); len(d) != 0 {
		t.Fatalf("identical snapshots must not report changes: %v", d)
	}
}

func TestFirmwareLag(t *testing.T) {
	if compareFirmware("12.10.4", "12.9.9") <= 0 || compareFirmware("2025.1.1", "12.11") <= 0 || compareFirmware("12.11", "12.11.0") != 0 {
		t.Fatalf("compareFirmware ordering wrong")
	}
	fleet := map[string]fleetDevice{
		"a": {ID: "a", Model: "M390", Version: "2025.1.1"},
		"b": {ID: "b", Model: "M390", Version: "12.10.4"},
		"c": {ID: "c", Model: "T80", Version: "12.10.4"},
		"d": {ID: "d", Model: "M390", Version: ""},
	}
	lag := firmwareLag(fleet)
	if len(lag) != 1 || lag[0].DeviceID != "b" || lag[0].NewestVersion != "2025.1.1" {
		t.Fatalf("lag = %+v", lag)
	}
	if got := firmwareLag(map[string]fleetDevice{"x": {ID: "x", Model: "T80", Version: "12.1"}}); len(got) != 0 {
		t.Fatalf("a model with one device cannot lag: %+v", got)
	}
}

func TestReconcileHardware(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	allocated := mustMaps(t, `{"data":[
	 {"serialOrLicense":"SER-GONE","accountId":"ACC-1","accountName":"Acme"},
	 {"serialOrLicense":"ser-off","accountId":"ACC-1"},
	 {"serialOrLicense":"SER-RECENT","accountId":"ACC-2"},
	 {"serialOrLicense":"SER-OK","accountId":"ACC-2"}]}`, "data")
	unallocated := mustMaps(t, `{"data":[{"serialOrLicense":"SER-SPARE"}]}`, "data")
	devices := mustMaps(t, `{"count":5,"data":[
	 {"id":"FB-2","serial_number":"SER-OFF","state":"offline","offline_since":"2026-09-01T00:00:00Z"},
	 {"id":"FB-3","serial_number":"SER-RECENT","state":"offline","offline_since":"2026-10-13T00:00:00Z"},
	 {"id":"FB-4","serial_number":"SER-OK","state":"online"},
	 {"id":"FB-5","serial_number":"SER-SPARE","state":"online"},
	 {"id":"FB-6","serial_number":"SER-ROGUE","state":"online"}]}`, "data")
	h := reconcileHardware(allocated, unallocated, devices, 7*24*time.Hour, now)
	ids := func(fs []hardwareFinding) []string {
		out := []string{}
		for _, f := range fs {
			out = append(out, f.Serial)
		}
		return out
	}
	if !reflect.DeepEqual(ids(h.AllocatedNotReporting), []string{"SER-GONE"}) ||
		!reflect.DeepEqual(ids(h.AllocatedOffline), []string{"SER-OFF"}) ||
		!reflect.DeepEqual(ids(h.ReportingUnallocated), []string{"SER-SPARE"}) ||
		!reflect.DeepEqual(ids(h.ReportingUntracked), []string{"SER-ROGUE"}) {
		t.Fatalf("reconcile = %+v", h)
	}
}

func TestExpiringWithin(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	rows := mustMaps(t, `{"data":[
	 {"productName":"EPDR","daysUntilExpiry":5},
	 {"productName":"AuthPoint","daysUntilExpiry":"90"},
	 {"productName":"NDR","allocationExpiryDate":"2026-10-20T00:00:00Z"},
	 {"productName":"NoDate"}]}`, "data")
	got := expiringWithin("endPoint", rows, 30*24*time.Hour, now)
	if len(got) != 2 || got[0].Product != "EPDR" || got[1].Product != "NDR" || got[1].DaysLeft != 6 {
		t.Fatalf("expiring = %+v", got)
	}
}

func TestOpenIncidentsByAccountAndFleetCounts(t *testing.T) {
	items := mustMaps(t, `{"items":[{"account":"A","severity":"High","status":"new"},{"account":"A","severity":"High","status":"closed"},{"account":"B","severity":"Low","status":"read"}]}`, "items")
	got := openIncidentsByAccount(items)
	if got["A"]["high"] != 1 || got["B"]["low"] != 1 || len(got) != 2 {
		t.Fatalf("counts = %v", got)
	}
	total, off := fleetCounts(mustMaps(t, `{"data":[{"state":"online"},{"state":"offline"},{"state":"never connected"}]}`, "data"))
	if total != 3 || off != 2 {
		t.Fatalf("fleet counts = %d/%d", off, total)
	}
}

func TestNewestTransaction(t *testing.T) {
	rows := mustMaps(t, `[{"id":"old","inception_time":100},{"id":"new","inception_time":300},{"id":"mid","inception_time":200}]`, "data")
	if got := strField(newestTransaction(rows), "id"); got != "new" {
		t.Fatalf("newest = %s", got)
	}
	if newestTransaction(nil) != nil {
		t.Fatalf("no rows must give nil")
	}
}

func TestAsMapsKeylessObjectIsNotARow(t *testing.T) {
	for _, body := range []string{`{"total_items":0}`, `{}`, `{"pageControls":{"totalItems":0}}`} {
		if got := asMaps(json.RawMessage(body), "data"); len(got) != 0 {
			t.Fatalf("%s produced %d fake rows", body, len(got))
		}
	}
	if got := firstObject(json.RawMessage(`{"id":"ddply_1","status":"pending"}`), "data"); strField(got, "id") != "ddply_1" {
		t.Fatalf("single-object transaction not read: %v", got)
	}
	if got := firstObject(json.RawMessage(`[{"id":"ddply_2"}]`), "data"); strField(got, "id") != "ddply_2" {
		t.Fatalf("one-element list transaction not read: %v", got)
	}
}

func TestPagingDone(t *testing.T) {
	cases := []struct {
		got, asked, total, have int
		want                    bool
	}{
		{0, 100, 500, 100, true},   // empty page always ends
		{50, 100, 500, 150, false}, // server capped page below the ask, total says more
		{50, 100, 0, 50, true},     // no total: short page ends
		{100, 100, 0, 100, false},  // no total: full page continues
		{100, 100, 200, 200, true}, // total reached
	}
	for _, c := range cases {
		if got := pagingDone(c.got, c.asked, c.total, c.have); got != c.want {
			t.Errorf("pagingDone%+v = %v", c, got)
		}
	}
	if totalCount(json.RawMessage(`{"pageControls":{"totalItems":7}}`)) != 7 || totalCount(json.RawMessage(`{"total_items":9}`)) != 9 {
		t.Fatalf("totalCount misses an envelope")
	}
}

func TestParseTimeThreatSyncFormat(t *testing.T) {
	for _, s := range []string{"2023-03-02 18:52:20+00:00", "2023-03-02T18:52:20Z", "2023-03-02 18:52:20.123+00:00"} {
		if ts, ok := parseTime(s); !ok || ts.UTC().Hour() != 18 {
			t.Errorf("parseTime(%q) = %v %v", s, ts, ok)
		}
	}
}

func TestIncidentRowsGroupedForm(t *testing.T) {
	got := incidentRows(json.RawMessage(`{"groups":{"_default":[{"id":"i1"},{"id":"i2"}]}}`))
	if len(got) != 2 {
		t.Fatalf("grouped incidents = %d rows", len(got))
	}
}

func TestIncidentDurationAndStatuses(t *testing.T) {
	if d, err := incidentDuration("7d"); err != nil || d != "7D" {
		t.Fatalf("7d -> %q %v", d, err)
	}
	if _, err := incidentDuration("seven"); err == nil {
		t.Fatalf("bad duration accepted")
	}
	if _, err := parseStatuses("opne"); err == nil {
		t.Fatalf("typo status accepted")
	}
	if s, err := parseStatuses("all"); err != nil || len(s) != 0 {
		t.Fatalf("all should mean no filter: %v %v", s, err)
	}
}

func TestFleetDeviceAcceptsBooleanCloudManaged(t *testing.T) {
	var d fleetDevice
	if err := json.Unmarshal([]byte(`{"id":"FB-1","cloud_managed":true,"version":"12.10.4"}`), &d); err != nil || d.CloudManaged != "yes" {
		t.Fatalf("bool cloud_managed: %v %q", err, d.CloudManaged)
	}
	if err := json.Unmarshal([]byte(`{"id":"FB-1","cloud_managed":"no"}`), &d); err != nil || d.CloudManaged != "no" {
		t.Fatalf("string cloud_managed: %v %q", err, d.CloudManaged)
	}
}

func TestDiffFleetOrderAndCameOnlineSince(t *testing.T) {
	prev := map[string]fleetDevice{"A": {ID: "A", State: "offline", OfflineSince: "2026-10-01T00:00:00Z"}, "B": {ID: "B", State: "online"}}
	cur := map[string]fleetDevice{"A": {ID: "A", State: "online"}, "C": {ID: "C", State: "online"}}
	got := diffFleet(prev, cur)
	kinds := []string{}
	for _, c := range got {
		kinds = append(kinds, c.Change)
	}
	if !reflect.DeepEqual(kinds, []string{"removed", "new", "came_online"}) {
		t.Fatalf("order = %v", kinds)
	}
	if got[2].OfflineSince != "2026-10-01T00:00:00Z" {
		t.Fatalf("came_online should carry the previous offline_since, got %q", got[2].OfflineSince)
	}
}

func TestTransactionDeviceID(t *testing.T) {
	for in, want := range map[string]string{"FB-12345": "12345", "fb-12345": "12345", "12345": "12345", "FBCL-12345": "FBCL-12345"} {
		if got := transactionDeviceID(in); got != want {
			t.Errorf("transactionDeviceID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFanoutVerdict(t *testing.T) {
	var buf bytes.Buffer
	if err := fanoutVerdict(&buf, &rootFlags{}, 0, 3); err != nil {
		t.Fatalf("no failures must be nil")
	}
	var ce *cliError
	if err := fanoutVerdict(&buf, &rootFlags{}, 3, 3); !errors.As(err, &ce) || ce.code != 5 {
		t.Fatalf("all failed must exit 5, got %v", err)
	}
	if err := fanoutVerdict(&buf, &rootFlags{}, 1, 3); !errors.As(err, &ce) || ce.code != 6 {
		t.Fatalf("partial must exit 6, got %v", err)
	}
	if err := fanoutVerdict(&buf, &rootFlags{allowPartialFailure: true}, 1, 3); err != nil {
		t.Fatalf("--allow-partial-failure must exit 0, got %v", err)
	}
}

func TestEntityActionsMatchPerEntity(t *testing.T) {
	inc := mustMaps(t, `[{"entities":{
	 "a":{"recommendedActions":[{"command":"isolate"}],"actionPerformed":{"command":"isolate"}},
	 "b":{"recommendedActions":[{"command":"isolate"},{"command":"quarantineFile","parameters":{"md5":"BBB"}}],"actionPerformed":{"command":"quarantineFile","parameters":{"md5":"AAA"}}}}}]`, "data")[0]
	performed, open := entityActions(inc)
	if !reflect.DeepEqual(performed, []string{"isolate", "quarantineFile:AAA"}) {
		t.Fatalf("performed = %v", performed)
	}
	if !reflect.DeepEqual(open, []string{"isolate", "quarantineFile:BBB"}) {
		t.Fatalf("entity b's isolate and other-file quarantine must stay open, got %v", open)
	}
}

func TestProductFromRowKeepsAbsentQuantitiesAbsent(t *testing.T) {
	sub := productFromRow("endPoint", mustMaps(t, `[{"productName":"EPDR","allocationType":"subscription","subscriptionTotal":"unlimited","usage":42}]`, "data")[0])
	if sub.Total != nil || sub.Allocated != nil || sub.Available != nil || sub.SubscriptionTotal != "unlimited" || sub.Usage == nil || *sub.Usage != 42 {
		t.Fatalf("subscription row = %+v", sub)
	}
	term := productFromRow("endPoint", mustMaps(t, `[{"productName":"EPDR","quantityTotal":50,"quantityAllocated":30,"quantityAvailable":20}]`, "data")[0])
	if term.Total == nil || *term.Total != 50 || *term.Available != 20 {
		t.Fatalf("term row = %+v", term)
	}
}

func TestFleetSnapshotChain(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	sqldb := db.DB()
	for _, ddl := range fleetSnapshotDDL {
		if _, err := sqldb.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	a := map[string]fleetDevice{"FB-1": {ID: "FB-1", State: "online", Uptime: "1 Days"}}
	b := map[string]fleetDevice{"FB-1": {ID: "FB-1", State: "online", Uptime: "2 Days"}}
	if fleetFingerprint(a) != fleetFingerprint(b) {
		t.Fatalf("uptime alone must not make a new fleet state")
	}
	first, ok, err := recordSnapshot(ctx, sqldb, "", fleetFingerprint(a), a)
	if err != nil || !ok {
		t.Fatalf("first record: %v %v", ok, err)
	}
	if _, ok, err := recordSnapshot(ctx, sqldb, "", fleetFingerprint(a), a); err != nil || ok {
		t.Fatalf("a second successor of the same parent must be ignored: %v %v", ok, err)
	}
	empty := map[string]fleetDevice{}
	if _, ok, err := recordSnapshot(ctx, sqldb, first, fleetFingerprint(empty), empty); err != nil || !ok {
		t.Fatalf("an emptied fleet must be recordable: %v %v", ok, err)
	}
	snaps, err := recentSnapshots(ctx, sqldb)
	if err != nil || len(snaps) != 2 || snaps[1].at != first {
		t.Fatalf("snapshots = %+v %v", snaps, err)
	}
	prev, _, err := readFleet(ctx, sqldb, `SELECT device_id, data FROM wg_fleet_devices WHERE snapshot_at = ?`, first)
	if err != nil {
		t.Fatal(err)
	}
	if d := diffFleet(prev, empty); len(d) != 1 || d[0].Change != "removed" {
		t.Fatalf("emptied fleet diff = %+v", d)
	}
}

func TestFirstObjectEmptyEnvelope(t *testing.T) {
	for _, body := range []string{`{"data":{}}`, `{"data":[]}`} {
		if got := firstObject(json.RawMessage(body), "data"); got != nil {
			t.Fatalf("%s must not be a transaction, got %v", body, got)
		}
	}
}

func TestRecordSnapshotRefusesStaleHead(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, sqldb := context.Background(), db.DB()
	for _, ddl := range fleetSnapshotDDL {
		if _, err := sqldb.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	a := map[string]fleetDevice{"FB-1": {ID: "FB-1", State: "online"}}
	b := map[string]fleetDevice{"FB-1": {ID: "FB-1", State: "offline"}}
	first, _, _ := recordSnapshot(ctx, sqldb, "", fleetFingerprint(a), a)
	if _, ok, _ := recordSnapshot(ctx, sqldb, first, fleetFingerprint(b), b); !ok {
		t.Fatalf("B should record after A")
	}
	// A late recorder that still believes "first" is the head must not append.
	stale := map[string]fleetDevice{"FB-1": {ID: "FB-1", State: "never connected"}}
	time.Sleep(2 * time.Millisecond)
	if _, ok, err := recordSnapshot(ctx, sqldb, "", fleetFingerprint(stale), stale); ok || err != nil {
		t.Fatalf("stale recorder appended: ok=%v err=%v", ok, err)
	}
	snaps, _ := recentSnapshots(ctx, sqldb)
	if len(snaps) != 2 || snaps[0].fingerprint != fleetFingerprint(b) {
		t.Fatalf("head moved to stale data: %+v", snaps)
	}
}
