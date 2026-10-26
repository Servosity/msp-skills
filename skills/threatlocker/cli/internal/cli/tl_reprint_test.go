// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: behavior tests for the press 4.34.0 reprint's hand-fixes and
// new novels, driven through the real command tree against a seeded store and
// a local HTTP fixture. No tenant data, no real credentials.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"threatlocker-pp-cli/internal/cliutil/testenv"
	"threatlocker-pp-cli/internal/store"
)

const tlFixtureToken = "0000000000000000000000000000000000000000000000000000000000000001"

// tlRecorder is a fixture ThreatLocker API that records every request.
type tlRecorder struct {
	mu   sync.Mutex
	reqs []tlRecordedRequest
}

type tlRecordedRequest struct {
	Method, Path, Org string
	Body              map[string]any
}

func (r *tlRecorder) seen() []tlRecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]tlRecordedRequest(nil), r.reqs...)
}

func newTLFixture(t *testing.T, handle func(path string, body map[string]any) (int, any)) (*tlRecorder, string) {
	t.Helper()
	rec := &tlRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, tlRecordedRequest{req.Method, req.URL.Path, req.Header.Get("ManagedOrganizationId"), body})
		rec.mu.Unlock()
		status, out := handle(req.URL.Path, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return rec, srv.URL
}

// seedTLStore opens the store at a temp path and runs the given inserts.
func seedTLStore(t *testing.T, inserts ...string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range inserts {
		if _, err := db.DB().Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	return dbPath
}

func runTL(t *testing.T, baseURL string, args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("THREATLOCKER_API_KEY", tlFixtureToken)
	t.Setenv("THREATLOCKER_ORG_ID", "")
	if baseURL != "" {
		t.Setenv("THREATLOCKER_BASE_URL", baseURL)
	}
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

const tlPendingApprovals = `INSERT INTO approvals (id, data, approval_request_id, organization_id, organization_name, computer_id, file_name, hash, status_id)
	VALUES ('a1', '{}', 'a1', 'org-1', 'Tenant 1', 'c1', 'setup.exe', 'aaaa', 1),
	       ('a2', '{}', 'a2', 'org-2', 'Tenant 2', 'c2', 'setup.exe', 'aaaa', 1)`

// A live approve-batch without --yes must refuse and send nothing: the
// command approves files across tenants, so an unconfirmed run is a write
// without confirmation.
func TestApproveBatchRefusesWithoutYes(t *testing.T) {
	rec, base := newTLFixture(t, func(string, map[string]any) (int, any) { return 200, map[string]any{} })
	dbPath := seedTLStore(t, tlPendingApprovals)
	out, err := runTL(t, base, "approvals", "approve-batch", "--hash", "AAAA", "--all-tenants", "--db", dbPath)
	if err == nil {
		t.Fatalf("approve-batch without --yes succeeded; output:\n%s", out)
	}
	if n := len(rec.seen()); n != 0 {
		t.Fatalf("approve-batch without --yes sent %d request(s)", n)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("refusal does not name --yes: %v", err)
	}
}

// With --yes each approval is sent under its own tenant's header.
func TestApproveBatchWithYesScopesEachTenant(t *testing.T) {
	rec, base := newTLFixture(t, tlAnswerRecheckPending(func(string, map[string]any) (int, any) { return 200, map[string]any{} }))
	dbPath := seedTLStore(t, tlPendingApprovals)
	if out, err := runTL(t, base, "approvals", "approve-batch", "--hash", "aaaa", "--all-tenants", "--yes", "--db", dbPath); err != nil {
		t.Fatalf("approve-batch --yes: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, r := range rec.seen() {
		if r.Method == http.MethodGet && strings.HasSuffix(r.Path, tlStaleGetByID) {
			continue // the live re-check before each write (tl_stale_snapshot_test.go)
		}
		if !strings.HasSuffix(r.Path, "/ApprovalRequest/ApprovalRequestPermitApplication") {
			t.Fatalf("unexpected request %s %s", r.Method, r.Path)
		}
		if r.Org != r.Body["organizationId"] {
			t.Fatalf("request for %v sent under tenant header %q", r.Body["organizationId"], r.Org)
		}
		got[r.Org] = true
	}
	if !got["org-1"] || !got["org-2"] || len(got) != 2 {
		t.Fatalf("approvals sent for tenants %v, want org-1 and org-2", got)
	}
}

// maintenance open: started-and-not-ended windows only, open-ended windows
// count, future windows are scheduled, unparseable starts are counted.
func TestMaintenanceOpenClassifiesWindows(t *testing.T) {
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	dbPath := seedTLStore(t,
		`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, organization_name) VALUES
			('c1','{}','c1','PC-1','org-1','Tenant 1'), ('c2','{}','c2','PC-2','org-2','Tenant 2')`,
		`INSERT INTO maintenance (id, data, computer_id, maintenance_type, start_date_time, end_date_time, created_by) VALUES
			('m1','{}','c1','Learning','`+ts(-72*time.Hour)+`','`+ts(48*time.Hour)+`','tech'),
			('m2','{}','c1','Installation','`+ts(-2*time.Hour)+`','','tech'),
			('m3','{}','c2','Learning','`+ts(-96*time.Hour)+`','`+ts(-1*time.Hour)+`','tech'),
			('m4','{}','c2','Learning','`+ts(3*time.Hour)+`','`+ts(5*time.Hour)+`','tech'),
			('m5','{}','c2','Learning','not a date','','tech')`)
	out, err := runTL(t, "", "maintenance", "open", "--all-tenants", "--older-than", "24h", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("maintenance open: %v\n%s", err, out)
	}
	var got struct {
		Open []struct {
			StartDateTime string `json:"startDateTime"`
			Overdue       bool   `json:"overdue"`
		} `json:"open"`
		OpenCount     int `json:"open_count"`
		OverdueCount  int `json:"overdue_count"`
		Scheduled     int `json:"scheduled"`
		UnparsedStart int `json:"unparsed_start"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if got.OpenCount != 2 || got.OverdueCount != 1 || got.Scheduled != 1 || got.UnparsedStart != 1 {
		t.Fatalf("open=%d overdue=%d scheduled=%d unparsed=%d; want 2 1 1 1\n%s", got.OpenCount, got.OverdueCount, got.Scheduled, got.UnparsedStart, out)
	}
	if !got.Open[0].Overdue {
		t.Fatalf("longest-open window is not first: %+v", got.Open)
	}
}

// organizations posture counts each table per tenant and ages audit rows by
// instant, not by string order.
func TestOrganizationsPostureCountsPerTenant(t *testing.T) {
	dbPath := seedTLStore(t,
		`INSERT INTO organizations (id, data, organization_id, name) VALUES ('org-1','{}','org-1','Tenant 1'), ('org-2','{}','org-2','Tenant 2')`,
		`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, organization_name, is_online, is_isolated) VALUES
			('c1','{}','c1','PC-1','org-1','Tenant 1',1,0), ('c2','{}','c2','PC-2','org-1','Tenant 1',1,1), ('c3','{}','c3','PC-3','org-2','Tenant 2',1,0)`,
		tlPendingApprovals,
		`INSERT INTO audit (id, data, organization_id, date_created) VALUES
			('l1','{}','org-1','2026-10-09T00:00:00Z'), ('l2','{}','org-1','10/20/2026 00:00:00')`)
	out, err := runTL(t, "", "organizations", "posture", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("posture: %v\n%s", err, out)
	}
	var got struct {
		Tenants []tenantPosture `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	by := map[string]tenantPosture{}
	for _, c := range got.Tenants {
		by[c.OrganizationID] = c
	}
	one := by["org-1"]
	if one.Endpoints != 2 || one.Online != 1 || one.Isolated != 1 || one.PendingApprovals != 1 {
		t.Fatalf("org-1 card %+v", one)
	}
	if one.NewestAudit != "10/20/2026 00:00:00" {
		t.Fatalf("org-1 newest audit %q, want the 10/20 row (string order would pick 2026-10-09)", one.NewestAudit)
	}
	if by["org-2"].Endpoints != 1 || by["org-2"].AuditAgeDays != nil {
		t.Fatalf("org-2 card %+v", by["org-2"])
	}
}

// audit pull pages both windowed audits into the store and reports complete;
// a failed page makes the run exit non-zero while keeping what it fetched.
func TestAuditPullPagesBothSourcesAndFailsClosed(t *testing.T) {
	page := func(prefix string, n, start int) []map[string]any {
		rows := []map[string]any{}
		for i := 0; i < n; i++ {
			rows = append(rows, map[string]any{prefix: prefix + "-" + string(rune('a'+start+i)), "organizationId": "org-1", "dateCreated": "2026-10-20T00:00:00Z"})
		}
		return rows
	}
	failSystem := false
	rec, base := newTLFixture(t, func(path string, body map[string]any) (int, any) {
		p, _ := body["pageNumber"].(float64)
		switch {
		case strings.HasSuffix(path, "/ActionLog/ActionLogGetByParametersV2"):
			if p == 1 {
				return 200, page("eActionLogId", 2, 0)
			}
			return 200, page("eActionLogId", 1, 2)
		case strings.HasSuffix(path, "/SystemAudit/SystemAuditGetByParameters"):
			if failSystem {
				return 200, map[string]any{"success": false, "data": nil}
			}
			return 200, page("systemAuditId", 1, 0)
		}
		return 404, map[string]any{}
	})
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	out, err := runTL(t, base, "audit", "pull", "--since", "30d", "--page-size", "2", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("audit pull: %v\n%s", err, out)
	}
	if n := len(rec.seen()); n != 3 {
		t.Fatalf("audit pull sent %d requests, want 3 (two audit pages, one system-audit page)", n)
	}
	for _, r := range rec.seen() {
		if r.Body["startDate"] == nil || r.Body["endDate"] == nil {
			t.Fatalf("request without a window: %+v", r.Body)
		}
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var audits, systems int
	_ = db.DB().QueryRow(`SELECT count(*) FROM audit`).Scan(&audits)
	_ = db.DB().QueryRow(`SELECT count(*) FROM system_audit`).Scan(&systems)
	db.Close()
	if audits != 3 || systems != 1 {
		t.Fatalf("stored audit=%d system_audit=%d, want 3 and 1", audits, systems)
	}

	failSystem = true
	out, err = runTL(t, base, "audit", "pull", "--since", "30d", "--page-size", "2", "--db", dbPath, "--json")
	if err == nil || !strings.Contains(out, `"complete": false`) {
		t.Fatalf("a failed system-audit page did not fail the pull: err=%v\n%s", err, out)
	}
}

// The press names the application-files dependent application_files while
// the store's typed dispatch knows application-files, so every row landed in
// the generic table only. Every dependent must have a typed table.
func TestEveryDependentLandsInItsTypedTable(t *testing.T) {
	for _, dep := range dependentResourceDefs() {
		if reconcileTypedTables[dep.Name] == "" {
			t.Errorf("dependent %q has no typed table; its rows would reach only the generic store", dep.Name)
		}
	}
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row := json.RawMessage(`{"applicationFileId":"f1","applicationId":"app-1","fullPath":"c:\\x.exe","hash":"ab"}`)
	if _, _, _, err := db.UpsertBatchDetailed("application-files", []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM application_files`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("typed application_files rows = %d (err %v), want 1", n, err)
	}
}

// Sync must never run the GET /ScheduledAgentAction/List route (HTTP 417
// without a scheduledType) and must keep the dropdown twins out of the default
// set; scheduled-actions syncs the paginated POST under its own name.
func TestSyncSurfaceUsesTheRealCollections(t *testing.T) {
	path, err := syncResourcePath("scheduled-actions")
	if err != nil || path != "/ScheduledAgentAction/GetByParameters" {
		t.Fatalf("scheduled-actions syncs %q (%v)", path, err)
	}
	for _, r := range defaultSyncResources() {
		if strings.Contains(r, "dropdown") || strings.Contains(r, "for-move") || strings.Contains(r, "get-by-parameters") {
			t.Errorf("default sync set carries %q", r)
		}
	}
	if syncResourceMethod("scheduled-actions") != "POST" {
		t.Errorf("scheduled-actions sync method %q", syncResourceMethod("scheduled-actions"))
	}
}

// ThreatLocker's default host is instance g's real API root, not a
// placeholder: doctor must probe it rather than call it unconfigured.
func TestDoctorTreatsTheShippedHostAsReal(t *testing.T) {
	if doctorShippedBaseURL != "" {
		t.Fatalf("doctorShippedBaseURL = %q; a real vendor host here makes doctor skip every healthy instance-g install", doctorShippedBaseURL)
	}
	if doctorBaseURLIsPlaceholder("https://portalapi.g.threatlocker.com/portalapi") || doctorBaseURLIsShippedPlaceholder("https://portalapi.g.threatlocker.com/portalapi") {
		t.Fatal("doctor treats portalapi.g.threatlocker.com as a placeholder")
	}
	if !doctorBaseURLIsShippedPlaceholder("https://portalapi.{instance}.threatlocker.com/portalapi") {
		t.Fatal("an unresolved template is no longer refused")
	}
}

// --org and THREATLOCKER_ORG_ID reach the tenant headers through the client
// hook; the env var alone never filters local rows.
func TestTenantHeadersReachTheWire(t *testing.T) {
	rec, base := newTLFixture(t, func(string, map[string]any) (int, any) { return 200, map[string]any{"count": 0} })
	if out, err := runTL(t, base, "--org", "org-9", "approvals", "count", "--json"); err != nil {
		t.Fatalf("approvals count --org: %v\n%s", err, out)
	}
	reqs := rec.seen()
	if len(reqs) == 0 || reqs[len(reqs)-1].Org != "org-9" {
		t.Fatalf("--org did not reach ManagedOrganizationId: %+v", reqs)
	}
	if tenantOrgFlag != "org-9" {
		t.Fatalf("tenantOrgFlag = %q after --org org-9", tenantOrgFlag)
	}
}

// doctor's credential probe must be a command that runs with no input. press
// 4.34.0 marks required-input leaves pp:requires-input, and the probe used to
// pick `applications get` (needs --application-id), so a healthy install read
// "base_url is not this vendor's API root".
func TestDoctorProbeNeedsNoInput(t *testing.T) {
	testenv.Isolate(t)
	root := RootCmd()
	apiPath, cmdPath := doctorReadProbe(root)
	if apiPath == "" {
		t.Fatal("doctor found no argument-free GET to probe")
	}
	leaf, _, err := root.Find(strings.Fields(cmdPath))
	if err != nil {
		t.Fatalf("probe %q does not resolve: %v", cmdPath, err)
	}
	if leaf.Annotations["pp:requires-input"] == "true" {
		t.Fatalf("doctor probes %q (%s), which requires input", cmdPath, apiPath)
	}
	leaf.Flags().VisitAll(func(f *pflag.Flag) {
		if _, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; ok {
			t.Fatalf("doctor probes %q, which has required flag --%s", cmdPath, f.Name)
		}
	})
}
