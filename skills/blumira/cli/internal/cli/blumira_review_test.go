// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Review regressions (2026-11 reprint, Codex round 1): each test drives a
// documented Blumira response shape (spec.json) through the real command or
// engine path and fails if the MSP would be shown a wrong answer.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"blumira-pp-cli/internal/cliutil/testenv"
	"blumira-pp-cli/internal/store"
)

// ---- fake Blumira API ------------------------------------------------------

// fakeBlumira serves documented list envelopes ({data, links, meta, status}).
// pages maps a request path to its pages (page 1 first); a path mapped to a
// status code instead answers with that error.
type fakeBlumira struct {
	mu     sync.Mutex
	pages  map[string][][]map[string]any
	fail   map[string]int
	hits   map[string]int
	server *httptest.Server
}

func newFakeBlumira(t *testing.T) *fakeBlumira {
	t.Helper()
	f := &fakeBlumira{pages: map[string][][]map[string]any{}, fail: map[string]int{}, hits: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	t.Setenv("BLUMIRA_BASE_URL", f.server.URL)
	t.Setenv("BLUMIRA_API_TOKEN", "test-token")
	return f
}

func (f *fakeBlumira) set(path string, pages ...[]map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[path] = pages
	delete(f.fail, path)
}

func (f *fakeBlumira) failWith(path string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[path] = code
}

func (f *fakeBlumira) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[r.URL.Path]++
	w.Header().Set("Content-Type", "application/json")
	if code, ok := f.fail[r.URL.Path]; ok {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"status":"ERROR","message":"injected failure"}`))
		return
	}
	pages, ok := f.pages[r.URL.Path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"ERROR","message":"not found"}`))
		return
	}
	page := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		page = n
	}
	total := 0
	for _, p := range pages {
		total += len(p)
	}
	data := []map[string]any{}
	if page >= 1 && page <= len(pages) {
		data = pages[page-1]
	}
	var next any
	if page < len(pages) {
		next = r.URL.Path + "?page=" + strconv.Itoa(page+1)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":   data,
		"links":  map[string]any{"next": next, "prev": nil, "self": r.URL.Path},
		"meta":   map[string]any{"page": page, "page_size": 50, "total_items": total, "total_pages": len(pages)},
		"status": "OK",
	})
}

func runRoot(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errOut.String())
	}
	return out.Bytes()
}

func jsonRows(t *testing.T, out []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	return rows
}

func findingRecord(id, org string, status int, statusName string) map[string]any {
	return map[string]any{
		"finding_id": id, "org_id": org, "name": "Finding " + id, "priority": 2,
		"status": status, "status_name": statusName,
		"created": "2026-10-01T00:00:00.000", "modified": "2026-10-01T00:00:00.000",
	}
}

// ---- class: ownership read (owners documented as role-keyed object; absent on /org/findings) ----

func TestReviewOwnershipRoleKeyedAndUnknown(t *testing.T) {
	// /msp/accounts/findings items are finding_detail_record: owners is
	// {analysts, managers, responders}, each a response_owners array.
	detail := map[string]any{
		"finding_id": "f-msp", "org_id": "acct-1", "name": "Assigned", "priority": 1, "status": 10, "status_name": "Open",
		"created": "2026-10-01T00:00:00.000",
		"owners": map[string]any{
			"analysts":   []any{map[string]any{"email": "analyst@example.com", "id": "u1"}},
			"managers":   []any{},
			"responders": []any{},
		},
	}
	// /org/findings items are findings_data_record: no owners field at all.
	direct := map[string]any{"finding_id": "f-org", "org_id": "org-1", "name": "Direct", "priority": 2, "status": 10, "status_name": "Open", "created": "2026-10-01T00:00:00.000"}
	findings := []findingRec{decodeFinding(detail), decodeFinding(direct)}

	workload, _ := json.Marshal(computeWorkload(findings, extraNow))
	if !strings.Contains(string(workload), `"assignee":"analyst@example.com"`) {
		t.Errorf("an analyst assigned through the documented owners object is missing from workload: %s", workload)
	}
	if strings.Contains(string(workload), `"(unassigned)"`) {
		t.Errorf("workload reports a finding as unassigned although its ownership is assigned or unknown: %s", workload)
	}
	reconcile, _ := json.Marshal(computeReconcile(findings, nil))
	if !strings.Contains(string(reconcile), `"assignees":"analyst@example.com"`) {
		t.Errorf("reconcile lost the documented analyst assignee: %s", reconcile)
	}
	if !strings.Contains(string(reconcile), `"ownership":"unknown"`) {
		t.Errorf("reconcile must mark a /org/findings row (no owners field) as ownership unknown, not as unassigned: %s", reconcile)
	}
}

// ---- class: partition read that sync never fills (MSP sub-account devices) ----

func TestReviewSyncFillsSubAccountDevices(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	stale := time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02T15:04:05.000")
	fresh := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000")
	api.set("/msp/accounts", []map[string]any{
		{"account_id": "acct-1", "name": "Client One", "open_findings": 0},
		{"account_id": "acct-2", "name": "Client Two", "open_findings": 0},
	})
	api.set("/org/agents/devices", []map[string]any{
		{"device_id": "dev-direct", "hostname": "ws01", "org_id": "org-1", "is_domain_controller": false, "alive": fresh},
	})
	api.set("/msp/accounts/acct-1/agents/devices", []map[string]any{
		{"device_id": "dev-sub-dc", "hostname": "dc-sub", "org_id": "acct-1", "is_domain_controller": true, "alive": stale},
	})
	api.failWith("/msp/accounts/acct-2/agents/devices", http.StatusInternalServerError)

	runRoot(t, "sync", "--resources", "msp,org-agents-devices")
	out := runRoot(t, "exposure", "--flag-dc-stale", "--json")
	var sawSubDC, sawGap bool
	for _, r := range jsonRows(t, out) {
		if r["hostname"] == "dc-sub" {
			sawSubDC = true
		}
		if r["account"] == "acct-2" && strings.Contains(r["reason"].(string), "devices-not-synced") {
			sawGap = true
		}
	}
	if !sawSubDC {
		t.Errorf("the stale domain controller in MSP sub-account acct-1 is absent from exposure: %s", out)
	}
	if !sawGap {
		t.Errorf("exposure must name sub-account acct-2, whose device list could not be read, instead of reporting it clean: %s", out)
	}
}

// ---- class: incomplete paging read as complete (evidence pages) ----

func TestReviewEvidenceFetchFollowsPages(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", 10, "Open")})
	api.set("/org/findings/f1/evidence",
		[]map[string]any{{"user": "alice", "src_ip": "192.0.2.1"}},
		[]map[string]any{{"user": "bob", "src_ip": "10.0.4.21"}},
	)
	runRoot(t, "sync", "--resources", "org-findings")
	out := runRoot(t, "evidence-search", "10.0.4.21", "--fetch", "--json")
	var view struct {
		Matches []map[string]any `json:"matches"`
	}
	if err := json.Unmarshal(out, &view); err != nil {
		t.Fatalf("evidence-search --json: %v\n%s", err, out)
	}
	if len(view.Matches) != 1 || view.Matches[0]["finding_id"] != "f1" {
		t.Fatalf("evidence on page 2 of the documented paginated evidence response was never searched: %s", out)
	}
}

// ---- class: search term read as a pattern ----

func TestReviewEvidenceSearchIsLiteral(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := storeEvidenceItems(s.DB(), "f1", "a", []string{`{"user":"svcXbackup"}`}, extraNow); err != nil {
		t.Fatal(err)
	}
	if err := storeEvidenceItems(s.DB(), "f2", "a", []string{`{"user":"ops 100% done"}`}, extraNow); err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"svc_backup", "100%x", `%`} {
		got, err := searchEvidenceCache(s.DB(), term, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range got {
			if m.FindingID == "f1" || (term != `%` && m.FindingID == "f2") {
				t.Errorf("term %q matched %s, whose evidence does not contain it literally", term, m.FindingID)
			}
		}
	}
	if got, _ := searchEvidenceCache(s.DB(), "100% DONE", 10); len(got) != 1 || got[0].FindingID != "f2" {
		t.Errorf("a literal %% term must still match case-insensitively, got %+v", got)
	}
}

// ---- class: limit applied before de-duplication ----

func TestReviewEvidenceLimitCountsFindings(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := storeEvidenceItems(s.DB(), "f1", "a", []string{`{"src_ip":"10.0.4.21"}`, `{"dst_ip":"10.0.4.21"}`}, extraNow); err != nil {
		t.Fatal(err)
	}
	if err := storeEvidenceItems(s.DB(), "f2", "a", []string{`{"src_ip":"10.0.4.21"}`}, extraNow); err != nil {
		t.Fatal(err)
	}
	got, err := searchEvidenceCache(s.DB(), "10.0.4.21", 2)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, m := range got {
		ids[m.FindingID] = true
	}
	if !ids["f1"] || !ids["f2"] {
		t.Fatalf("--limit 2 with two matching findings returned %+v; extra rows of one finding crowded out the other", got)
	}
}

// ---- class: snapshot history (captured at sync; identity follows state; velocity reads the latest state) ----

func TestReviewSyncRecordsHistoryForAudit(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	for _, st := range []struct {
		code int
		name string
	}{{10, "Open"}, {40, "Resolved"}, {10, "Open"}} {
		api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", st.code, st.name)})
		runRoot(t, "sync", "--resources", "org-findings")
	}
	out := runRoot(t, "audit", "--json")
	if rows := jsonRows(t, out); len(rows) != 1 || rows[0]["finding_id"] != "f1" || rows[0]["reopens"] != float64(1) {
		t.Fatalf("a finding synced open, resolved, then open again is missing from audit: %s", out)
	}
}

func TestReviewDriftSeesOrgOnlyResync(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	api.set("/msp/accounts/findings", []map[string]any{findingRecord("f-msp", "acct-1", 10, "Open")})
	api.set("/org/findings", []map[string]any{findingRecord("f-org", "org-1", 10, "Open")})
	runRoot(t, "sync", "--resources", "msp-accounts-findings,org-findings")
	runRoot(t, "drift", "--json")
	api.set("/org/findings", []map[string]any{findingRecord("f-org", "org-1", 40, "Resolved")})
	runRoot(t, "sync", "--resources", "org-findings")
	out := runRoot(t, "drift", "--json")
	if rows := jsonRows(t, out); len(rows) != 1 || rows[0]["finding_id"] != "f-org" || rows[0]["change"] != "resolved" {
		t.Fatalf("drift missed a direct-org finding resolved by an org-only resync: %s", out)
	}
}

func TestReviewVelocityReadsLatestState(t *testing.T) {
	snaps := []historySnapshot{
		snap("2026-05-01T00:00:00Z", findingRec{ID: "x", Account: "a", Status: 10, StatusName: "Open"}),
		snap("2026-05-02T00:00:00Z", findingRec{ID: "x", Account: "a", Status: resolvedStatusCode, StatusName: "Resolved"}),
		snap("2026-05-03T00:00:00Z", findingRec{ID: "x", Account: "a", Status: 10, StatusName: "Open"}),
	}
	rows := computeVelocity(snaps, time.Time{}, false)
	if len(rows) != 1 || rows[0].Resolved != 0 || rows[0].Open != 1 || rows[0].OpenRatePct != 100 {
		t.Fatalf("a finding resolved then reopened is currently open; velocity says %+v", rows)
	}
}

func TestReviewDriftComparesTheTwoLatestSyncs(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", 10, "Open")})
	runRoot(t, "sync", "--resources", "org-findings")
	api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", 40, "Resolved")})
	runRoot(t, "sync", "--resources", "org-findings")
	for i := 0; i < 2; i++ { // re-running between syncs adds no history point
		if rows := jsonRows(t, runRoot(t, "drift", "--json")); len(rows) != 1 || rows[0]["change"] != "resolved" {
			t.Fatalf("run %d: drift between the two latest syncs = %+v, want f1 resolved", i+1, rows)
		}
	}
	runRoot(t, "sync", "--resources", "org-findings") // nothing changed
	if rows := jsonRows(t, runRoot(t, "drift", "--json")); len(rows) != 0 {
		t.Fatalf("a sync that changed nothing must show no drift, not the previous sync's change again: %+v", rows)
	}
}

func TestReviewAnalyticsCaptureIsIdempotent(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	findings := []findingRec{{ID: "f1", Status: 10, StatusName: "Open"}}
	for i := 0; i < 3; i++ {
		if _, err := captureFindingHistory(s.DB(), s, findings, extraNow.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	snaps, err := loadHistorySnapshots(s.DB())
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("re-running an analytics command without a sync recorded %d history points, want 1", len(snaps))
	}
	findings[0].Status, findings[0].StatusName = resolvedStatusCode, "Resolved"
	if _, err := captureFindingHistory(s.DB(), s, findings, extraNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if snaps, _ = loadHistorySnapshots(s.DB()); len(snaps) != 2 || !snaps[1].Findings[0].IsResolved() {
		t.Fatalf("a changed state must become a new, later history point: %+v", snaps)
	}
	// A sync point always lands after every earlier point, even with a clock behind them.
	if _, err := recordFindingHistory(s.DB(), s, findings, extraNow.Add(-24*time.Hour), true); err != nil {
		t.Fatal(err)
	}
	if snaps, _ = loadHistorySnapshots(s.DB()); len(snaps) != 3 || !snaps[2].SyncedAt.After(snaps[1].SyncedAt) {
		t.Fatalf("a sync point must sort after every earlier point: %+v", snaps)
	}
}

func TestReviewSubAccountDeviceWalk(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	accounts := func(ids ...string) {
		items := make([]json.RawMessage, 0, len(ids))
		for _, id := range ids {
			items = append(items, json.RawMessage(`{"account_id":"`+id+`","name":"`+id+`"}`))
		}
		if _, _, err := s.UpsertBatch("msp", items); err != nil {
			t.Fatal(err)
		}
	}
	accounts("a1", "a2", "a3")
	get := pageGetterFunc(func(path string, page int) (string, error) {
		switch path {
		case "/msp/accounts/a1/agents/devices":
			return `{"status":"OK","data":[{"device_id":"d1","hostname":"dc1","is_domain_controller":true}],"links":{"next":null},"meta":{"page":1,"total_pages":1,"total_items":1}}`, nil
		case "/msp/accounts/a2/agents/devices":
			return `{"status":"OK","data":[{"hostname":"no-id"}],"links":{"next":null},"meta":{"page":1,"total_pages":1,"total_items":1}}`, nil
		}
		return `{"status":"OK","data":[],"links":{"next":null},"meta":{"page":1,"total_pages":2,"total_items":5}}`, nil
	})
	n, gaps, err := syncMSPAccountDevices(t.Context(), get, s.DB(), s, extraNow)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(gaps) != 2 {
		t.Fatalf("stored %d devices with gaps %v; want 1 device (a1) and gaps for a2 (row without device_id) and a3 (list says more rows than it returned)", n, gaps)
	}
	cov, err := loadDeviceCoverage(s)
	if err != nil {
		t.Fatal(err)
	}
	if !cov.Complete["a1"] || cov.Complete["a2"] || cov.Complete["a3"] || len(cov.Gaps) != 2 {
		t.Fatalf("coverage = %+v", cov)
	}
	devices, err := loadDevices(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Account != "a1" || !devices[0].IsDC {
		t.Fatalf("sub-account devices = %+v", devices)
	}
	// An account the MSP list no longer carries loses its device rows.
	if _, err := s.ReconcileAll("msp", []string{"a2", "a3"}, reconcileTypedTable("msp"), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := syncMSPAccountDevices(t.Context(), get, s.DB(), s, extraNow); err != nil {
		t.Fatal(err)
	}
	if devices, _ = loadDevices(s); len(devices) != 0 {
		t.Fatalf("devices of a removed sub-account are still reported: %+v", devices)
	}
}

// A store upgraded from a build that wrote history without sync marks keeps
// drift between its two latest syncs: the first analytics run adopts the
// legacy latest point instead of duplicating it.
func TestReviewLegacyHistoryAdopted(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := ensureFindingHistory(s.DB()); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		key    string
		status int
		name   string
	}{{"2026-06-01T10:00:00Z", 10, "Open"}, {"2026-06-02T10:00:00Z", 40, "Resolved"}} {
		if _, err := s.DB().Exec(`INSERT INTO finding_history (synced_at, finding_id, status, status_name) VALUES (?, 'f1', ?, ?)`, row.key, row.status, row.name); err != nil {
			t.Fatal(err)
		}
	}
	findings := []findingRec{{ID: "f1", Status: resolvedStatusCode, StatusName: "Resolved"}}
	if _, err := captureFindingHistory(s.DB(), s, findings, extraNow); err != nil {
		t.Fatal(err)
	}
	snaps, _ := loadHistorySnapshots(s.DB())
	if len(snaps) != 2 {
		t.Fatalf("the legacy latest point already held the current state; got %d points", len(snaps))
	}
	if rows := computeDrift(snaps[0].Findings, snaps[1].Findings); len(rows) != 1 || rows[0].Change != "resolved" {
		t.Fatalf("drift after upgrade = %+v, want f1 resolved", rows)
	}
}
