// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written behavior tests for fleet-sync and the cross-tenant commands.

package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/client"
	"blackpoint-pp-cli/internal/cliutil/testenv"
)

// fakeCompassOne serves just enough of the API for fleet-sync. Tenant-scoped
// routes reject calls without x-tenant-id, like the live API.
type fakeCompassOne struct {
	mu         sync.Mutex
	detections map[string][]map[string]any
	tenants    []map[string]any
	missingHdr int
}

func (f *fakeCompassOne) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	page := func(rows any) map[string]any {
		return map[string]any{"data": rows, "meta": map[string]any{"currentPage": 1, "totalPages": 1}}
	}
	tenant := r.Header.Get("x-tenant-id")
	scoped := func() bool {
		if tenant == "" {
			f.missingHdr++
			w.WriteHeader(400)
			write(map[string]string{"message": "x-tenant-id header required"})
			return false
		}
		return true
	}
	switch r.URL.Path {
	case "/tenants":
		if f.tenants == nil {
			f.tenants = []map[string]any{
				{"id": "t1", "name": "Acme Dental", "type": "MDR", "contactGroupId": "cg1"},
				{"id": "t2", "name": "Northwind Legal", "type": "MDR"},
			}
		}
		write(page(f.tenants))
	case "/alert-groups":
		if scoped() {
			rows := f.detections[tenant]
			write(map[string]any{"data": rows, "meta": map[string]any{"skip": 0, "take": 200, "totalItems": len(rows)}})
		}
	case "/vulnerability-management/darkweb/scan/exposures":
		if scoped() {
			write(page([]map[string]any{{"id": "dw-" + tenant, "username": "ops@acme.example", "password": "hunter2"}}))
		}
	case "/security-posture/rating":
		if scoped() {
			write(map[string]any{"id": "pr-" + tenant, "score": 30, "maximumScore": 50, "metricCalculationResults": []map[string]any{
				{"nistFunction": "Protect", "deduction": 5, "metricApplied": true, "metricPassed": false},
				{"nistFunction": "Detect", "deduction": 9, "metricApplied": false, "metricPassed": false},
			}})
		}
	case "/vulnerability-management/vulnerabilities":
		if scoped() {
			w.WriteHeader(403)
			write(map[string]string{"message": "not entitled"})
		}
	default:
		w.WriteHeader(404)
		write(map[string]string{"message": "not found"})
	}
}

func runFleetCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func TestFleetSyncSendsTenantHeaderAndReplacesSlices(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeCompassOne{detections: map[string][]map[string]any{
		"t1": {
			{"id": "ag1", "status": "OPEN", "type": "MDR", "alertCount": 3, "hostname": "ws-01", "created": time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339), "ticket": map[string]any{"status": "ESCALATE"}},
			{"id": "ag2", "status": "OPEN", "type": "CR", "alertCount": 1, "created": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)},
		},
		"t2": {{"id": "ag3", "status": "OPEN", "type": "MDR", "alertCount": 2, "created": time.Now().Add(-30 * time.Hour).UTC().Format(time.RFC3339)}},
	}}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()
	t.Setenv("BLACKPOINT_BASE_URL", srv.URL)
	t.Setenv("BLACKPOINT_API_TOKEN", "test-token")
	db := filepath.Join(t.TempDir(), "fleet.db")

	if _, err := runFleetCmd(t, "fleet-sync", "--kinds", "detections,darkweb,posture", "--db", db, "--json"); err != nil {
		t.Fatalf("fleet-sync: %v", err)
	}
	if fake.missingHdr != 0 {
		t.Fatalf("%d tenant-scoped calls went out without x-tenant-id", fake.missingHdr)
	}

	out, err := runFleetCmd(t, "triage", "--db", db, "--json")
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("triage output not JSON: %v\n%s", err, out)
	}
	if len(rows) != 3 || rows[0]["detection_id"] != "ag1" || rows[0]["tenant"] != "Acme Dental" {
		t.Fatalf("want 3 rows oldest-first starting with ag1/Acme Dental, got %v", rows)
	}

	// Fleet rows are indexed into the generated store, so `search` finds them.
	if out, err := runFleetCmd(t, "search", "ws-01", "--db", db, "--json"); err != nil || !strings.Contains(out, "ag1") {
		t.Fatalf("search ws-01 should find detection ag1 (err=%v): %s", err, out)
	}

	// Filters: --older-than drops the 2h-old detection, --type MDR drops the CR one.
	out, _ = runFleetCmd(t, "triage", "--db", db, "--json", "--older-than", "24h", "--type", "MDR")
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 2 {
		t.Fatalf("filtered triage want 2 rows, got %d: %s", len(rows), out)
	}

	// ag1 is resolved upstream; a re-sync must drop it locally, not keep it as OPEN.
	fake.mu.Lock()
	fake.detections["t1"] = fake.detections["t1"][1:]
	fake.mu.Unlock()
	if _, err := runFleetCmd(t, "fleet-sync", "--kinds", "detections", "--db", db, "--json"); err != nil {
		t.Fatalf("second fleet-sync: %v", err)
	}
	out, _ = runFleetCmd(t, "triage", "--db", db, "--json")
	if strings.Contains(out, `"ag1"`) {
		t.Fatalf("resolved detection ag1 still listed after re-sync: %s", out)
	}

	// A tenant dropped from the inventory disappears from local results.
	fake.mu.Lock()
	fake.tenants = fake.tenants[:1]
	fake.mu.Unlock()
	if _, err := runFleetCmd(t, "fleet-sync", "--kinds", "detections", "--db", db, "--json"); err != nil {
		t.Fatalf("third fleet-sync: %v", err)
	}
	if out, _ := runFleetCmd(t, "triage", "--db", db, "--json"); strings.Contains(out, "ag3") || strings.Contains(out, "Northwind") {
		t.Fatalf("removed tenant t2 still listed: %s", out)
	}
	if out, _ := runFleetCmd(t, "search", "ag3", "--db", db, "--json"); strings.Contains(out, "ag3") {
		t.Fatalf("removed tenant's detection still searchable: %s", out)
	}

	// Dark-web passwords never reach the local store.
	out, _ = runFleetCmd(t, "overview", "Acme Dental", "--db", db, "--json")
	if strings.Contains(out, "hunter2") {
		t.Fatalf("exposed password leaked into overview: %s", out)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("overview not JSON: %v\n%s", err, out)
	}
	if rep["darkweb_passwords_exposed"].(float64) != 1 {
		t.Fatalf("want 1 password-exposed row, got %v", rep["darkweb_passwords_exposed"])
	}

	// Only t1 remains after the inventory shrank.
	out, _ = runFleetCmd(t, "leaderboard", "--db", db, "--json")
	var board []map[string]any
	_ = json.Unmarshal([]byte(out), &board)
	rows = board
	if len(rows) != 1 || rows[0]["percent"].(float64) != 60 || rows[0]["weakest_nist_function"] != "Protect" {
		t.Fatalf("leaderboard want 60%% with weakest Protect (unapplied Detect ignored), got %v", rows)
	}
}

func TestFleetReadersEmptyStoreExitZero(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "missing.db")
	for _, args := range [][]string{
		{"triage"}, {"stale-agents"}, {"cve-spread"}, {"leaderboard"}, {"gaps"}, {"geo-audit"},
	} {
		out, err := runFleetCmd(t, append(args, "--db", db, "--json")...)
		if err != nil {
			t.Fatalf("%s on empty store: %v", args[0], err)
		}
		if strings.TrimSpace(out) != "[]" {
			t.Fatalf("%s on empty store want [], got %q", args[0], out)
		}
	}
}

func TestBuildStaleAgentRows(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	idx := map[string]fleetTenant{"t1": {ID: "t1", Name: "Acme"}}
	recs := []fleetRecord{
		{Scope: "t1", ID: "fresh", Obj: map[string]any{"name": "fresh", "agentLastSeenOn": "2026-10-14T00:00:00Z"}},
		{Scope: "t1", ID: "old", Obj: map[string]any{"name": "old", "agentLastSeenOn": "2026-09-01T00:00:00Z"}},
		{Scope: "t1", ID: "never", Obj: map[string]any{"name": "never"}},
		{Scope: "t1", ID: "gone", Obj: map[string]any{"name": "gone", "agentLastSeenOn": "2026-01-01T00:00:00Z", "decommissioned": "true"}},
	}
	rows := buildStaleAgentRows(recs, idx, now, 14, false)
	if len(rows) != 2 || rows[0].DeviceID != "never" || rows[1].DeviceID != "old" {
		t.Fatalf("want [never old], got %+v", rows)
	}
	if got := buildStaleAgentRows(recs, idx, now, 14, true); len(got) != 3 {
		t.Fatalf("include-decommissioned want 3 rows, got %d", len(got))
	}
}

func TestBuildCveSpreadRows(t *testing.T) {
	idx := map[string]fleetTenant{"t1": {Name: "A"}, "t2": {Name: "B"}}
	recs := []fleetRecord{
		{Scope: "t1", ID: "1", Obj: map[string]any{"name": "CVE-2026-0001", "exploitability": "Attacked", "assetsAmount": 4.0, "baseScore": 9.8, "severity": "Critical"}},
		{Scope: "t2", ID: "2", Obj: map[string]any{"name": "cve-2026-0001", "exploitability": "Attacked", "assetsAmount": 1.0, "baseScore": 9.8}},
		{Scope: "t1", ID: "3", Obj: map[string]any{"name": "CVE-2026-0002", "exploitability": "Unreported", "assetsAmount": 50.0}},
	}
	rows := buildCveSpreadRows(recs, idx, true, false, 1)
	if len(rows) != 1 || rows[0].CVE != "CVE-2026-0001" || rows[0].Tenants != 2 || rows[0].Assets != 5 || rows[0].Severity != "Critical" {
		t.Fatalf("exploited spread wrong: %+v", rows)
	}
	if rows := buildCveSpreadRows(recs, idx, false, false, 2); len(rows) != 1 {
		t.Fatalf("--min-tenants 2 want 1 row, got %+v", rows)
	}
}

func TestClassifyException(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	for _, tc := range []struct{ start, end, want string }{
		{"", "2026-10-01T00:00:00Z", "expired"},
		{"", "2026-10-18T00:00:00Z", "expiring"},
		{"", "2026-12-01T00:00:00Z", "active"},
		{"", "", "open-ended"},
		{"2026-11-01T00:00:00Z", "2026-11-10T00:00:00Z", "upcoming"},
	} {
		if got, _ := classifyException(tc.start, tc.end, now, week); got != tc.want {
			t.Errorf("classifyException(%q,%q) = %s, want %s", tc.start, tc.end, got, tc.want)
		}
	}
}

func TestParseFleetDurationAndTenantResolution(t *testing.T) {
	for in, want := range map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "2w": 14 * 24 * time.Hour, "": 0} {
		if got, err := parseFleetDuration(in); err != nil || got != want {
			t.Errorf("parseFleetDuration(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := parseFleetDuration("soon"); err == nil {
		t.Error("parseFleetDuration(soon) should fail")
	}
	tenants := []fleetTenant{{ID: "a", Name: "Acme Dental"}, {ID: "b", Name: "Acme Legal"}, {ID: "c", Name: "Northwind"}}
	if got, err := resolveFleetTenant(tenants, "north"); err != nil || got.ID != "c" {
		t.Errorf("unique substring: %v %v", got, err)
	}
	if _, err := resolveFleetTenant(tenants, "acme"); err == nil || !strings.Contains(err.Error(), "matches 2 tenants") {
		t.Errorf("ambiguous name should fail with candidates, got %v", err)
	}
	if got, err := resolveFleetTenant(tenants, "b"); err != nil || got.Name != "Acme Legal" {
		t.Errorf("id match: %v %v", got, err)
	}
}

func TestSplitFleetPageShapes(t *testing.T) {
	items, meta, err := splitFleetPage(json.RawMessage(`{"data":[{"id":1},{"id":2}],"meta":{"currentPage":1,"totalPages":3}}`), false)
	if err != nil || len(items) != 2 || meta["totalPages"].(float64) != 3 {
		t.Fatalf("data envelope: %d items, meta=%v err=%v", len(items), meta, err)
	}
	items, meta, err = splitFleetPage(json.RawMessage(`{"items":[{"id":1}],"total":1}`), false)
	if err != nil || len(items) != 1 || meta["totalItems"].(float64) != 1 {
		t.Fatalf("items envelope: %d items, meta=%v err=%v", len(items), meta, err)
	}
	// meta: null plus a top-level total must not panic.
	if _, meta, err = splitFleetPage(json.RawMessage(`{"data":[{"id":1}],"meta":null,"total":1}`), false); err != nil || meta["totalItems"].(float64) != 1 {
		t.Fatalf("null meta + total: meta=%v err=%v", meta, err)
	}
	if items, _, _ = splitFleetPage(json.RawMessage(`[{"id":1}]`), false); len(items) != 1 {
		t.Fatal("bare array")
	}
	if items, _, _ = splitFleetPage(json.RawMessage(`{"score":30}`), true); len(items) != 1 {
		t.Fatal("bare object is one row on a singleton route")
	}
	// On a collection route a bare object must not become a fake one-row list.
	if _, _, err := splitFleetPage(json.RawMessage(`{}`), false); err == nil {
		t.Fatal("bare object on a collection route should fail")
	}
	// Malformed responses are errors, never an empty page that wipes rows.
	for _, bad := range []string{`<html>oops</html>`, `{"data":"nope"}`, `null`, `{"data":null}`, ``} {
		if _, _, err := splitFleetPage(json.RawMessage(bad), false); err == nil {
			t.Errorf("splitFleetPage(%s) should fail", bad)
		}
	}
}

// TestPageTracker walks scripted page sequences: each step is the body for
// page N. The walk must end complete or with errFleetIncomplete as stated.
func TestPageTracker(t *testing.T) {
	cases := []struct {
		name     string
		mode     fleetPageMode
		pages    []string
		wantDone bool // false means errFleetIncomplete expected
	}{
		{"two full pages", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalPages":2}}`, `{"data":[{"id":"b"}],"meta":{"currentPage":2,"totalPages":2}}`}, true},
		{"empty list", pageByNumber, []string{`{"data":[],"meta":{"currentPage":1,"totalPages":0,"totalItems":0}}`}, true},
		{"no meta short page", pageByNumber, []string{`[{"id":"a"}]`}, true},
		{"skip/take to total", pageBySkip, []string{`{"data":[{"id":"a"}],"meta":{"skip":0,"take":1,"totalItems":2}}`, `{"data":[{"id":"b"}],"meta":{"skip":1,"take":1,"totalItems":2}}`}, true},
		{"duplicate ids satisfy total", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalItems":2}}`, `{"data":[{"id":"a"}],"meta":{"currentPage":2,"totalItems":2}}`}, false},
		{"promised page empty", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalPages":2}}`, `{"data":[],"meta":{"currentPage":2,"totalPages":2}}`}, false},
		{"earlier total forgotten", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalItems":3}}`, `[]`}, false},
		{"forged currentPage", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":2,"totalPages":2}}`}, false},
		{"single request with more pages", pageNone, []string{`{"data":[{"id":"a"}],"meta":{"totalPages":2}}`}, false},
		{"skip echo mismatch", pageBySkip, []string{`{"data":[{"id":"a"}],"meta":{"skip":5,"totalItems":2}}`}, false},
		{"total met but more pages advertised", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalItems":1,"totalPages":2}}`, `{"data":[],"meta":{"currentPage":2,"totalItems":1,"totalPages":2}}`}, false},
		{"top-level total smaller than meta total", pageByNumber, []string{`{"data":[{"id":"a"}],"meta":{"currentPage":1,"totalItems":3},"total":1}`, `[]`}, false},
		{"single request total met but two pages", pageNone, []string{`{"data":[{"id":"a"}],"meta":{"totalItems":1,"totalPages":2}}`}, false},
	}
	for _, tc := range cases {
		pt := newPageTracker(tc.mode, 2)
		var done bool
		var err error
		for i, body := range tc.pages {
			items, meta, serr := splitFleetPage(json.RawMessage(body), false)
			if serr != nil {
				t.Fatalf("%s: split: %v", tc.name, serr)
			}
			if done, err = pt.observe(i+1, items, meta); err != nil || done {
				break
			}
		}
		if tc.wantDone && (!done || err != nil) {
			t.Errorf("%s: want complete, got done=%v err=%v", tc.name, done, err)
		}
		if !tc.wantDone && !errors.Is(err, errFleetIncomplete) {
			t.Errorf("%s: want errFleetIncomplete, got done=%v err=%v", tc.name, done, err)
		}
	}
}

func TestResolveFleetTenantRejectsDuplicateExactNames(t *testing.T) {
	tenants := []fleetTenant{{ID: "a", Name: "Acme"}, {ID: "b", Name: "Acme"}}
	if _, err := resolveFleetTenant(tenants, "acme"); err == nil || !strings.Contains(err.Error(), "pass the tenant id") {
		t.Fatalf("duplicate exact names must be ambiguous, got %v", err)
	}
	if got, err := resolveFleetTenant(tenants, "b"); err != nil || got.ID != "b" {
		t.Fatalf("id still resolves: %v %v", got, err)
	}
}

func TestReaderSortsUseUnroundedValues(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	idx := map[string]fleetTenant{"t": {ID: "t", Name: "T"}}
	// Two detections 3 minutes apart round to the same tenth of an hour.
	dets := []fleetRecord{
		{Scope: "t", ID: "newer", Obj: map[string]any{"status": "OPEN", "created": now.Add(-10*time.Hour - 1*time.Minute).Format(time.RFC3339)}},
		{Scope: "t", ID: "older", Obj: map[string]any{"status": "OPEN", "created": now.Add(-10*time.Hour - 4*time.Minute).Format(time.RFC3339)}},
	}
	if rows := buildTriageRows(dets, idx, now, 0, "", ""); rows[0].DetectionID != "older" {
		t.Fatalf("triage must sort by exact age, got %s first", rows[0].DetectionID)
	}
	// 0.6667 vs 0.6665 both round to 66.7%.
	posture := []fleetRecord{
		{Scope: "a", ID: "a", Obj: map[string]any{"score": 2000.0, "maximumScore": 3000.0}},
		{Scope: "b", ID: "b", Obj: map[string]any{"score": 1333.0, "maximumScore": 2000.0}},
	}
	if rows := buildLeaderboardRows(posture, map[string]fleetTenant{}, false); rows[0].TenantID != "b" {
		t.Fatalf("leaderboard worst-first must compare exact ratios, got %s first", rows[0].TenantID)
	}
}

func TestTruncatedFetchMergesWithoutPruning(t *testing.T) {
	testenv.Isolate(t)
	s, err := openTestFleetDB(t)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err := replaceFleetScope(s, fleetKindDevice, "t1", []fleetRow{{ID: "d1", Data: json.RawMessage(`{"id":"d1"}`)}, {ID: "d2", Data: json.RawMessage(`{"id":"d2"}`)}}, at); err != nil {
		t.Fatal(err)
	}
	// An incomplete fetch that only saw d3 must keep d1/d2 and flag the slice.
	if err := mergeFleetRows(s, fleetKindDevice, "t1", []fleetRow{{ID: "d3", Data: json.RawMessage(`{"id":"d3"}`)}}, at, "incomplete"); err != nil {
		t.Fatal(err)
	}
	recs, _ := loadFleetRows(s, fleetKindDevice, "t1")
	states, _ := loadFleetSyncState(s, "t1")
	if len(recs) != 3 || len(states) != 1 || states[0].Error != "incomplete" {
		t.Fatalf("want 3 rows and an incomplete state, got %d rows, states=%+v", len(recs), states)
	}
}

func openTestFleetDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { db.Close() })
	return db, ensureFleetSchema(db)
}

func TestScopedHealthAndUnsyncedSections(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureFleetSchema(db); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	tenants := []fleetRow{{ID: "ta", Data: json.RawMessage(`{"id":"ta","name":"Alpha"}`)}, {ID: "tb", Data: json.RawMessage(`{"id":"tb","name":"Bravo"}`)}}
	if err := replaceFleetScope(db, fleetKindTenant, "", tenants, at); err != nil {
		t.Fatal(err)
	}
	// Bravo's detections synced; Alpha's last fetch failed.
	if err := replaceFleetScope(db, fleetKindDetection, "tb", nil, at); err != nil {
		t.Fatal(err)
	}
	// Alpha has rows from an older run, but its latest refresh failed.
	if err := replaceFleetScope(db, fleetKindDetection, "ta", []fleetRow{{ID: "stale-ag", Data: json.RawMessage(`{"id":"stale-ag","status":"OPEN"}`)}}, at.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := recordFleetFailure(db, fleetKindDetection, "ta", errors.New("not entitled (403)"), at); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Fleet-wide, Alpha's retained rows from before the failure are excluded.
	if out, err := runFleetCmd(t, "triage", "--db", dbPath, "--json"); err != nil || strings.Contains(out, "stale-ag") {
		t.Fatalf("fleet-wide triage must exclude a failed tenant's retained rows (err=%v): %s", err, out)
	}
	// --tenant Alpha must not read Bravo's success as coverage.
	if out, err := runFleetCmd(t, "triage", "--tenant", "Alpha", "--db", dbPath, "--json"); err == nil {
		t.Fatalf("triage --tenant Alpha should fail on Alpha's 403, got %s", out)
	}
	if _, err := runFleetCmd(t, "triage", "--tenant", "Bravo", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("triage --tenant Bravo should succeed: %v", err)
	}
	// Devices were never synced for Bravo: totals are null, not zero.
	out, err := runFleetCmd(t, "overview", "Bravo", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("overview not JSON: %v", err)
	}
	if rep["stale_devices_total"] != nil || rep["sections"].(map[string]any)["devices"] != "not synced" {
		t.Fatalf("unsynced devices must be null with status 'not synced', got total=%v sections=%v", rep["stale_devices_total"], rep["sections"])
	}
	if rep["open_detections_total"] == nil {
		t.Fatal("synced detections should report a total (0)")
	}
}

func TestFailureTextOmitsResponseBodyAndMembersNullIsUnknown(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &client.APIError{Method: "GET", Path: "/vulnerability-management/darkweb/scan/exposures", StatusCode: 500, Body: `{"password":"hunter2"}`})
	if got := fleetErrText(err); strings.Contains(got, "hunter2") || !strings.Contains(got, "HTTP 500") {
		t.Fatalf("failure text must keep status and drop the body, got %q", got)
	}
	if _, known := contactMembers(map[string]any{"members": nil}); known {
		t.Fatal("members:null must be unknown, not a known zero")
	}
	if n, known := contactMembers(map[string]any{"members": []any{}}); !known || n != 0 {
		t.Fatal("an empty members array is a known zero")
	}
	if err := requireIDs([]fleetRow{{ID: "#0"}}); err == nil {
		t.Fatal("inventory rows without ids must be rejected")
	}
}

func TestFetchAllShortPageIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Advertises 3 pages but page 2 is empty.
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"data":[{"id":"t1"}],"meta":{"currentPage":1,"totalPages":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"meta":{"currentPage":2,"totalPages":3}}`))
	}))
	defer srv.Close()
	t.Setenv("BLACKPOINT_BASE_URL", srv.URL)
	t.Setenv("BLACKPOINT_API_TOKEN", "test-token")
	flags := &rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	c.NoCache = true
	s := &fleetSyncer{ctx: t.Context(), c: c, pageSize: 1, maxPages: 10}
	rows, err := s.fetchAll("/tenants", nil, nil, pageByNumber)
	if !errors.Is(err, errFleetIncomplete) || len(rows) != 1 {
		t.Fatalf("empty page before the advertised end must be incomplete, got rows=%d err=%v", len(rows), err)
	}
	// An inventory row keyed only by a code fallback is rejected.
	if err := requireIDs([]fleetRow{{ID: "x", Data: json.RawMessage(`{"code":"x"}`)}}); err == nil {
		t.Fatal("inventory row without an id field must be rejected")
	}
}

func TestFinalPageShortOfTotalItemsIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Claims 3 items on a single page but sends 1.
		_, _ = w.Write([]byte(`{"data":[{"id":"t1"}],"meta":{"currentPage":1,"totalPages":1,"totalItems":3}}`))
	}))
	defer srv.Close()
	t.Setenv("BLACKPOINT_BASE_URL", srv.URL)
	t.Setenv("BLACKPOINT_API_TOKEN", "test-token")
	c, err := (&rootFlags{}).newClient()
	if err != nil {
		t.Fatal(err)
	}
	c.NoCache = true
	s := &fleetSyncer{ctx: t.Context(), c: c, pageSize: 50, maxPages: 10}
	if _, err := s.fetchAll("/tenants", nil, nil, pageByNumber); !errors.Is(err, errFleetIncomplete) {
		t.Fatalf("rows short of totalItems must be incomplete, got %v", err)
	}
	if _, err := s.fetchAll("/cloud/cisco/onboardings", nil, nil, pageNone); !errors.Is(err, errFleetIncomplete) {
		t.Fatalf("single-request lists must also flag rows short of totalItems, got %v", err)
	}
	if _, err := pagedGet(t.Context(), c, "/reports", nil, nil, 5); !errors.Is(err, errFleetIncomplete) {
		t.Fatalf("pagedGet must also flag rows short of totalItems, got %v", err)
	}
}
