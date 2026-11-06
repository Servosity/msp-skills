// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/cliutil/testenv"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"action1-pp-cli/internal/store"
)

// Codex r1 (action1 reprint on 4.34.2) class incomplete paging read as
// complete: /vulnerabilities/{orgId} answers a ResultPageAbridged
// (spec.json#/components/schemas/ResultPageAbridged: total_items, limit, from,
// items; no next_page), here one record per page whatever limit is asked.
// Sync must read every page of every organization.
func TestSyncReadsEveryAbridgedPage(t *testing.T) {
	cves := []map[string]any{
		{"cve_id": "CVE-2005-2300", "cvss_score": "9.8", "endpoints_count": 1},
		{"cve_id": "CVE-2005-2301", "cvss_score": "7.5", "endpoints_count": 2},
		{"cve_id": "CVE-2005-2302", "cvss_score": "5.0", "endpoints_count": 3},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/organizations":
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "ResultPage", "items": []map[string]any{{"id": "org-a", "name": "Alpha"}}, "total_items": "1", "next_page": ""})
		case strings.HasPrefix(r.URL.Path, "/vulnerabilities/"):
			from, _ := strconv.Atoi(r.URL.Query().Get("from"))
			items := []map[string]any{}
			if from < len(cves) {
				items = cves[from : from+1]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total_items": strconv.Itoa(len(cves)), "limit": "1", "from": strconv.Itoa(from), "items": items})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "ResultPage", "items": []any{}, "total_items": "0", "next_page": ""})
		}
	}))
	defer srv.Close()
	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir) // Windows reads USERPROFILE, not HOME
	t.Setenv("ACTION1_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("ACTION1_BASE_URL", srv.URL)
	t.Setenv("ACTION1_OAUTH2", "abridged-static-token-0123456789")
	db := filepath.Join(dir, "fleet.db")
	run := func(args ...string) []byte {
		t.Helper()
		var out, errb bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errb.String())
		}
		return out.Bytes()
	}
	run("sync", "--db", db, "--full", "--resources", "organizations,vulnerabilities", "--json")
	triage := string(run("fleet", "vuln-triage", "--db", db, "--json"))
	for _, c := range cves {
		if !strings.Contains(triage, c["cve_id"].(string)) {
			t.Errorf("sync stopped early: %s is missing from vuln-triage:\n%s", c["cve_id"], triage)
		}
	}
}

// The continuation rule on its own: any signal that more records remain wins.
func TestAction1PageContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, data, cursor string
		items              int
		wantNext           string
		wantMore           bool
	}{
		{"abridged first page", `{"total_items":"2","limit":"1","from":"0","items":[{}]}`, "", 1, "1", true},
		{"abridged last page", `{"total_items":"2","limit":"1","from":"1","items":[{}]}`, "1", 1, "", false},
		{"numbers, no from: the request's", `{"total_items":5,"items":[{},{}]}`, "2", 2, "4", true},
		{"result page, next_page empty but more remain", `{"total_items":"3","next_page":"","items":[{}]}`, "", 1, "1", true},
		{"no position stated", `{"items":[{}]}`, "", 1, "", false},
	} {
		next, more := action1PageContinuation([]byte(tc.data), tc.cursor, tc.items, "", false)
		if next != tc.wantNext || more != tc.wantMore {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, next, more, tc.wantNext, tc.wantMore)
		}
	}
	if next, more := action1PageContinuation([]byte(`{"total_items":"1","items":[{}]}`), "", 1, "7", true); next != "7" || !more {
		t.Errorf("a next_page cursor is kept: got (%q, %v)", next, more)
	}
}

// fleetReviewStore seeds endpoints (and optional other resources) in the
// shapes spec.json documents.
func fleetReviewStore(t *testing.T, resources map[string][]map[string]any) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for rt, rows := range resources {
		for i, r := range rows {
			id, _ := r["id"].(string)
			if id == "" {
				id, _ = r["cve_id"].(string)
			}
			if id == "" {
				id = fmt.Sprintf("%s-%d", rt, i)
			}
			seedUpsert(t, db, rt, id, r)
		}
	}
	return dbPath
}

// Codex r1 class wrong timestamp parsing from documented response: Endpoint
// last_seen is documented as YYYY-MM-DD_HH-mm-ss
// (spec.json#/components/schemas/Endpoint/properties/last_seen, example
// "2022-08-29_12-22-11"). An endpoint last seen in 2022 is stale by its age,
// and health-score reads its age.
func TestFleetReadsDocumentedLastSeen(t *testing.T) {
	db := fleetReviewStore(t, map[string][]map[string]any{"endpoints_managed": {
		{"id": "ep-a", "name": "WS-A", "organization_id": "org-a", "last_seen": "2022-08-29_12-22-11", "online_status": "ERROR",
			"missing_updates": map[string]any{"critical": 0, "other": 0}, "vulnerabilities": map[string]any{"critical": 0, "other": 0}},
	}})
	var rows []staleRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetStaleCmd, db, "--days", "7", "--include-offline=false"), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DaysSinceSeen < 365 || rows[0].Reason != "last seen > threshold" {
		t.Fatalf("an endpoint last seen 2022-08-29 is stale by its age: %+v", rows)
	}
	var scores []healthScoreRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetHealthScoreCmd, db), &scores); err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].DaysSinceSeen < 365 {
		t.Fatalf("health-score must read the documented last_seen: %+v", scores)
	}
}

// Codex r1 class wrong status interpretation from documented response:
// OnlineStatus SUCCESS means last seen within 7 days
// (spec.json#/components/schemas/OnlineStatus); WARNING and ERROR are older.
func TestFleetReadsDocumentedOnlineStatus(t *testing.T) {
	recent := time.Now().UTC().Add(-time.Hour).Format("2006-01-02_15-04-05")
	db := fleetReviewStore(t, map[string][]map[string]any{"endpoints_managed": {
		{"id": "ep-ok", "name": "WS-OK", "organization_id": "org-a", "last_seen": recent, "online_status": "SUCCESS"},
		{"id": "ep-warn", "name": "WS-WARN", "organization_id": "org-a", "last_seen": recent, "online_status": "WARNING"},
	}})
	var rows []staleRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetStaleCmd, db), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EndpointID != "ep-warn" {
		t.Fatalf("SUCCESS is online, WARNING is not: %+v", rows)
	}
}

// Codex r1 class wrong field read from documented response: an installed
// software row is InstalledSoftwareReportDataFields, its columns under
// "fields" (Name, Version, Vendor, and Endpoints, the number of endpoints).
func TestFleetSoftwareRollupReadsReportFields(t *testing.T) {
	db := fleetReviewStore(t, map[string][]map[string]any{"installed_software_data": {
		{"id": "app-a", "parent_id": "org-a", "fields": map[string]any{"Name": "Google Chrome", "Version": "120.0", "Vendor": "Google", "Endpoints": "42"}},
		{"id": "app-b", "parent_id": "org-b", "fields": map[string]any{"Name": "Google Chrome", "Version": "120.0", "Vendor": "Google", "Endpoints": "8"}},
	}})
	var rows []softwareRollupRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetSoftwareRollupCmd, db), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "Google Chrome" || rows[0].Version != "120.0" || rows[0].Publisher != "Google" ||
		rows[0].Installs != 50 || rows[0].Endpoints != 50 || rows[0].Orgs != 2 {
		t.Fatalf("want Google Chrome 120.0 by Google on 50 endpoints in 2 organizations: %+v", rows)
	}
}

// The same class in vuln-triage: Vulnerability.software is
// VulnerabilitySoftware, a list of {product_name, ...}.
func TestFleetVulnTriageReadsProductName(t *testing.T) {
	db := fleetReviewStore(t, map[string][]map[string]any{"vulnerabilities": {
		{"cve_id": "CVE-2024-0001", "cvss_score": "9.8", "cisa_kev": "Yes", "endpoints_count": 2, "parent_id": "org-a",
			"software": []any{map[string]any{"product_name": "Adobe Acrobat", "platform": "Windows", "endpoints_count": 2},
				map[string]any{"product_name": "Adobe Reader", "endpoints_count": 1}}},
	}})
	var rows []vulnTriageRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetVulnTriageCmd, db), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Software != "Adobe Acrobat, Adobe Reader" {
		t.Fatalf("want the documented product names: %+v", rows)
	}
}

// Codex r1 class missing endpoint reported as remediated: an endpoint the
// current snapshot did not observe (gone from the sync, or read without its
// missing_updates summary) is not_observed, never remediated; and an unknown
// posture is never zero in the other views.
func TestFleetAbsentObservationIsUnknown(t *testing.T) {
	ep := func(id string, crit int) map[string]any {
		return map[string]any{"id": id, "name": id, "parent_id": "org-a", "missing_updates": map[string]any{"critical": crit, "other": 0},
			"vulnerabilities": map[string]any{"critical": 0, "other": 0}}
	}
	dbPath := fleetReviewStore(t, map[string][]map[string]any{"endpoints_managed": {ep("ep-a", 5), ep("ep-b", 0)}})
	_ = runFleet(t, newNovelFleetPatchDriftCmd, dbPath) // baseline
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = 'endpoints_managed' AND json_extract(data, '$.id') = 'ep-a'`); err != nil {
		t.Fatal(err)
	} else if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("test setup: removed %d rows for ep-a", n)
	}
	seedUpsert(t, db, "endpoints_managed", "ep-c", map[string]any{"id": "ep-c", "name": "ep-c", "parent_id": "org-a"}) // no summary
	db.Close()
	time.Sleep(10 * time.Millisecond) // a distinct snapshot time
	var res driftResult
	if err := json.Unmarshal(runFleet(t, newNovelFleetPatchDriftCmd, dbPath), &res); err != nil {
		t.Fatal(err)
	}
	if res.Summary.Remediated != 0 || res.Summary.Cleared != 0 {
		t.Fatalf("no endpoint was remediated: %+v", res)
	}
	var sawGone bool
	for _, c := range res.Changes {
		if c.EndpointID == "ep-a" {
			sawGone = c.Status == "not_observed" && c.CurrentMiss == nil && c.Delta == nil && c.PreviousMiss != nil && *c.PreviousMiss == 5
		}
		if c.EndpointID == "ep-c" {
			t.Errorf("an endpoint with no missing_updates summary is not an observation: %+v", c)
		}
	}
	if !sawGone {
		t.Fatalf("ep-a is gone from the current snapshot: want not_observed with no current count: %+v", res)
	}
	var posture []patchPostureRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetPatchPostureCmd, dbPath), &posture); err != nil {
		t.Fatal(err)
	}
	for _, r := range posture {
		if r.EndpointID == "ep-c" && (r.PostureKnown || posture[0].EndpointID != "ep-c") {
			t.Errorf("patch-posture: ep-c has no summary: want missing_updates_known false, listed first: %+v", posture)
		}
	}
	var scores []healthScoreRow
	if err := json.Unmarshal(runFleet(t, newNovelFleetHealthScoreCmd, dbPath), &scores); err != nil {
		t.Fatal(err)
	}
	if len(scores) == 0 || scores[0].EndpointID != "ep-c" || scores[0].PostureKnown {
		t.Errorf("health-score: an unknown posture is never ranked healthy: %+v", scores)
	}
}
