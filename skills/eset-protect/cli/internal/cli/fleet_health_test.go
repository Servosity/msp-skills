// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestNovelFleetHealthHelpWires smoke-tests that the fleet health command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetHealthHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "health", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet health --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "health"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet health --help missing %q in output:\n%s", want, help)
		}
	}
}

// fleetHealthTestSeed writes rows (keyed by their "uuid") into a fresh mirror
// and returns its path.
func fleetHealthTestSeed(t *testing.T, rows map[string][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for resource, items := range rows {
		for _, item := range items {
			var id struct {
				UUID string `json:"uuid"`
			}
			if err := json.Unmarshal([]byte(item), &id); err != nil || id.UUID == "" {
				t.Fatalf("seed row without uuid (%v): %s", err, item)
			}
			if err := db.Upsert(resource, id.UUID, json.RawMessage(item)); err != nil {
				t.Fatalf("upsert %s/%s: %v", resource, id.UUID, err)
			}
		}
		if err := db.SaveSyncState(resource, "", len(items)); err != nil {
			t.Fatalf("save sync state %s: %v", resource, err)
		}
	}
	return path
}

// fleetHealthTestRun executes the CLI and returns stdout and the error.
func fleetHealthTestRun(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.Bytes(), err
}

// fleetHealthTestDecode parses command JSON into v, unwrapping a platform
// "data"/"results" envelope when one is present.
func fleetHealthTestDecode(t *testing.T, raw []byte, v any) {
	t.Helper()
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) == nil {
		for _, k := range []string{"data", "results"} {
			if inner, ok := env[k]; ok && env["meta"] != nil {
				raw = inner
			}
		}
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode output: %v\n%s", err, raw)
	}
}

func fleetHealthTestTime(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

// fleetHealthTestFixture: Acme Dental (3 devices, mixed health), Beta Law
// (2 healthy devices), Zeta Empty (no devices), one device outside every
// COMPANY subtree.
func fleetHealthTestFixture() map[string][]string {
	product := `[{"id":1,"name":"ESET PROTECT Entry"}]`
	return map[string][]string{
		"device-groups": {
			`{"uuid":"8b1f0c3e-0000-4000-8000-000000000001","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme Dental","parentGroupUuid":"8b1f0c3e-0000-4000-8000-000000000001","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"acme-hq","displayName":"HQ","parentGroupUuid":"acme","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_SITE"}`,
			`{"uuid":"beta","displayName":"Beta Law","parentGroupUuid":"8b1f0c3e-0000-4000-8000-000000000001","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"zeta","displayName":"Zeta Empty","parentGroupUuid":"8b1f0c3e-0000-4000-8000-000000000001","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"loose","displayName":"Lost and found","parentGroupUuid":"8b1f0c3e-0000-4000-8000-000000000001"}`,
		},
		"devices": {
			`{"uuid":"d1","displayName":"FRONTDESK-01","parentGroupUuid":"acme-hq","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK","lastSyncTime":"` + fleetHealthTestTime(time.Hour) + `","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_FINISHED","activeProducts":` + product + `}`,
			`{"uuid":"d2","displayName":"XRAY-02","parentGroupUuid":"acme","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED","functionalityProblemCount":3,"isMuted":true,"lastSyncTime":"` + fleetHealthTestTime(10*24*time.Hour) + `","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_FINISHED","activeProducts":` + product + `}`,
			`{"uuid":"d3","displayName":"OFFICE-03","parentGroupUuid":"acme","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_ATTENTION_RECOMMENDED","functionalityProblemCount":"2","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_PENDING","activeProducts":[]}`,
			`{"uuid":"b1","displayName":"PARTNER-01","parentGroupUuid":"beta","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK","lastSyncTime":"` + fleetHealthTestTime(2*time.Hour) + `","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_FINISHED","activeProducts":` + product + `}`,
			`{"uuid":"b2","displayName":"PARTNER-02","parentGroupUuid":"beta","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK","lastSyncTime":"` + fleetHealthTestTime(3*time.Hour) + `","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_FINISHED","activeProducts":` + product + `}`,
			`{"uuid":"u1","displayName":"SPARE-LAPTOP","parentGroupUuid":"loose","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED","lastSyncTime":"` + fleetHealthTestTime(time.Hour) + `","enrollmentStatus":"DEVICE_ENROLLMENT_STATUS_FAILED","activeProducts":` + product + `}`,
		},
	}
}

func TestFleetHealthEndToEndWorstFirst(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, fleetHealthTestFixture())
	out, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, out)
	}
	var v fleetHealthView
	fleetHealthTestDecode(t, out, &v)

	gotOrder := make([]string, 0, len(v.Customers))
	for _, r := range v.Customers {
		gotOrder = append(gotOrder, r.Customer)
	}
	wantOrder := []string{"Acme Dental", esetCustomerNone, "Beta Law", "Zeta Empty"}
	if strings.Join(gotOrder, "|") != strings.Join(wantOrder, "|") {
		t.Fatalf("worst-first order = %v, want %v", gotOrder, wantOrder)
	}
	acme := v.Customers[0]
	want := fleetHealthCounts{Devices: 3, OK: 1, AttentionRecommended: 1, AttentionRequired: 1, Problems: 5, Muted: 1, Stale: 2, Unenrolled: 1, NoProduct: 1, Score: 3 + 2*2 + 1 + 1 + 1}
	if acme.fleetHealthCounts != want || acme.CustomerUUID != "acme" {
		t.Fatalf("Acme row = %+v, want %+v", acme, want)
	}
	if u := v.Customers[1]; u.CustomerUUID != "" || u.Devices != 1 || u.Unenrolled != 1 || u.Score != 4 {
		t.Fatalf("(unassigned) bucket = %+v", u)
	}
	if z := v.Customers[3]; z.Devices != 0 || z.Score != 0 || z.CustomerUUID != "zeta" {
		t.Fatalf("empty customer row = %+v", z)
	}
	for _, r := range v.Customers {
		if r.Customer == esetCustomerTenant {
			t.Fatalf("(tenant) bucket must not appear in an MSP tree: %+v", v.Customers)
		}
	}
	wantTotals := fleetHealthCounts{Devices: 6, OK: 3, AttentionRecommended: 1, AttentionRequired: 2, Problems: 5, Muted: 1, Stale: 2, Unenrolled: 2, NoProduct: 1, Score: 2*3 + 2*2 + 1 + 2 + 1}
	if v.Totals != wantTotals {
		t.Fatalf("totals = %+v, want %+v", v.Totals, wantTotals)
	}
	if v.StaleDays != 7 || v.SyncedHint != "" {
		t.Fatalf("stale_days=%d synced_hint=%q", v.StaleDays, v.SyncedHint)
	}
}

func TestFleetHealthEndToEndFlags(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, fleetHealthTestFixture())

	var v fleetHealthView
	out, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json", "--customer", "beta")
	if err != nil {
		t.Fatalf("--customer: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if len(v.Customers) != 1 || v.Customers[0].Customer != "Beta Law" || v.Totals.Devices != 2 {
		t.Fatalf("--customer beta = %+v totals %+v", v.Customers, v.Totals)
	}

	v = fleetHealthView{}
	out, err = fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json", "--limit", "1")
	if err != nil {
		t.Fatalf("--limit: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if len(v.Customers) != 1 || v.Customers[0].Customer != "Acme Dental" || v.Totals.Devices != 6 {
		t.Fatalf("--limit 1 must keep the worst row and full totals: %+v totals %+v", v.Customers, v.Totals)
	}

	v = fleetHealthView{}
	out, err = fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json", "--stale-days", "30")
	if err != nil {
		t.Fatalf("--stale-days: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if v.Customers[0].Stale != 1 || v.StaleDays != 30 {
		t.Fatalf("--stale-days 30: XRAY-02 (10 days) must not be stale, only the never-synced device: %+v", v.Customers[0])
	}

	if _, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json", "--customer", "nobody"); err == nil {
		t.Fatal("unknown --customer must fail")
	}
	if _, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json", "--stale-days", "0"); err == nil {
		t.Fatal("--stale-days 0 must be rejected")
	}
}

func TestFleetHealthEndToEndEmpty(t *testing.T) {
	testenv.Isolate(t)
	// A mirror that exists but holds nothing.
	path := fleetHealthTestSeed(t, map[string][]string{})
	out, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json")
	if err != nil {
		t.Fatalf("empty mirror: %v", err)
	}
	if !strings.Contains(string(out), `"customers": []`) {
		t.Fatalf("customers must render as [] not null:\n%s", out)
	}
	var v fleetHealthView
	fleetHealthTestDecode(t, out, &v)
	if len(v.Customers) != 0 || v.Totals.Devices != 0 || !strings.Contains(v.SyncedHint, "no devices") {
		t.Fatalf("empty mirror view = %+v", v)
	}

	// No mirror file at all: empty result, exit 0.
	out, err = fleetHealthTestRun(t, "fleet", "health", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("missing mirror must exit 0: %v", err)
	}
	v = fleetHealthView{}
	fleetHealthTestDecode(t, out, &v)
	if v.Customers == nil || len(v.Customers) != 0 {
		t.Fatalf("missing mirror view = %+v", v)
	}
}

func TestFleetHealthRollupBuckets(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Hour).Format(time.RFC3339)

	// Single-tenant tree: every device rolls up to one (tenant) row.
	single := newEsetTree([]esetGroup{{UUID: "all", Name: "All"}, {UUID: "lab", Name: "Lab", ParentUUID: "all"}})
	rows := fleetHealthRollup([]esetDevice{
		{UUID: "x1", ParentGroupUUID: "all", LastSyncTime: fresh, FunctionalityStatus: "DEVICE_FUNCTIONALITY_STATUS_OK"},
		{UUID: "x2", ParentGroupUUID: "lab", LastSyncTime: fresh, FunctionalityStatus: "DEVICE_FUNCTIONALITY_STATUS_SOMETHING_NEW"},
	}, single, now, 7)
	if len(rows) != 1 || rows[0].Customer != esetCustomerTenant || rows[0].Devices != 2 || rows[0].OK != 1 {
		t.Fatalf("single tenant rollup = %+v", rows)
	}
	if rows[0].AttentionRequired+rows[0].AttentionRecommended != 0 {
		t.Fatalf("an unknown status must count only in devices: %+v", rows[0])
	}

	// No devices: the bucket must not appear; COMPANY customers still do.
	if got := fleetHealthRollup(nil, single, now, 7); got == nil || len(got) != 0 {
		t.Fatalf("empty single-tenant rollup = %#v, want empty non-nil", got)
	}
	msp := newEsetTree([]esetGroup{{UUID: "c1", Name: "Acme Dental", EntityType: esetCompanyEntityType}})
	got := fleetHealthRollup(nil, msp, now, 7)
	if len(got) != 1 || got[0].Customer != "Acme Dental" || got[0].Devices != 0 {
		t.Fatalf("zero-device company rollup = %+v", got)
	}
}

func TestFleetHealthRollupTieBreaks(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Hour).Format(time.RFC3339)
	tree := newEsetTree([]esetGroup{
		{UUID: "a", Name: "alpha clinic", EntityType: esetCompanyEntityType},
		{UUID: "b", Name: "Bravo Dental", EntityType: esetCompanyEntityType},
		{UUID: "c", Name: "Charlie Law", EntityType: esetCompanyEntityType},
	})
	ok := func(id, group string) esetDevice {
		return esetDevice{UUID: id, ParentGroupUUID: group, LastSyncTime: fresh, EnrollmentStatus: "DEVICE_ENROLLMENT_STATUS_FINISHED",
			ActiveProducts: []json.RawMessage{json.RawMessage(`{}`)}, FunctionalityStatus: "DEVICE_FUNCTIONALITY_STATUS_OK"}
	}
	rows := fleetHealthRollup([]esetDevice{ok("1", "c"), ok("2", "c"), ok("3", "b"), ok("4", "a")}, tree, now, 7)
	var names []string
	for _, r := range rows {
		if r.Score != 0 {
			t.Fatalf("healthy devices must score 0: %+v", r)
		}
		names = append(names, r.Customer)
	}
	// Equal score: more devices first, then case-insensitive name.
	if strings.Join(names, "|") != "Charlie Law|alpha clinic|Bravo Dental" {
		t.Fatalf("tie-break order = %v", names)
	}
}

func TestFleetHealthStaleBoundaryAndScore(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		sync string
		want bool
	}{
		{"exactly at the cutoff", now.Add(-7 * 24 * time.Hour).Format(time.RFC3339), false},
		{"one second past the cutoff", now.Add(-7*24*time.Hour - time.Second).Format(time.RFC3339), true},
		{"fresh", now.Add(-time.Minute).Format(time.RFC3339Nano), false},
		{"missing", "", true},
		{"malformed", "yesterday", true},
	}
	for _, c := range cases {
		if got := fleetHealthIsStale(esetDevice{LastSyncTime: c.sync}, now, 7); got != c.want {
			t.Errorf("%s: stale = %v, want %v", c.name, got, c.want)
		}
	}
	if !fleetHealthUnenrolled("DEVICE_ENROLLMENT_STATUS_PENDING") || !fleetHealthUnenrolled("DEVICE_ENROLLMENT_STATUS_FAILED") ||
		fleetHealthUnenrolled("DEVICE_ENROLLMENT_STATUS_FINISHED") || fleetHealthUnenrolled("") {
		t.Error("unenrolled must mean PENDING or FAILED only")
	}
	c := fleetHealthCounts{AttentionRequired: 2, Stale: 3, AttentionRecommended: 5, Unenrolled: 7, NoProduct: 11, Muted: 100, Problems: 100, OK: 100}
	if got := fleetHealthScore(c); got != 2*3+3*2+5+7+11 {
		t.Fatalf("score = %d; muted, problems and ok must not weigh in", got)
	}
}

func TestFleetHealthHumanTable(t *testing.T) {
	var buf bytes.Buffer
	if err := fleetHealthPrintTable(&buf, fleetHealthView{Customers: []fleetHealthRow{}, StaleDays: 7}); err != nil || !strings.Contains(buf.String(), "No customers") {
		t.Fatalf("empty table = %q %v", buf.String(), err)
	}
	buf.Reset()
	row := fleetHealthRow{Customer: "Acme Dental", CustomerUUID: "acme", fleetHealthCounts: fleetHealthCounts{Devices: 3, AttentionRequired: 1, Score: 3}}
	v := fleetHealthView{Customers: []fleetHealthRow{row}, Totals: row.fleetHealthCounts, StaleDays: 14}
	if err := fleetHealthPrintTable(&buf, v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CUSTOMER", "Acme Dental", "TOTAL", "14 days", "attention_required*3"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, buf.String())
		}
	}
}
