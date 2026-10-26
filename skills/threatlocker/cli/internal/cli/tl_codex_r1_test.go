// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: one test per class the round-1 Codex review found in the
// press 4.34.0 reprint. Each failed on the tree before its fix. Seeded stores
// and a local HTTP fixture only; no tenant data, no real credentials.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/cliutil"
	"threatlocker-pp-cli/internal/cliutil/testenv"
)

// Class limit-before-aggregate: --limit caps the detail rows only. A cert hunt
// must resolve every hash the cert's file rules carry before the tenant rollup
// is built, or the "authoritative" rollup silently drops tenants.
func TestHuntCertResolvesEveryHashBeforeLimit(t *testing.T) {
	dbPath := seedTLStore(t,
		`INSERT INTO application_files (id, data, application_file_id, application_id, full_path, hash, cert) VALUES
			('f1','{}','f1','app-1','c:\a.exe','h1','Example Corp'),
			('f2','{}','f2','app-1','c:\b.exe','h2','Example Corp')`,
		`INSERT INTO approvals (id, data, approval_request_id, organization_id, organization_name, computer_id, file_name, hash, status_id) VALUES
			('a1','{}','a1','org-1','Tenant 1','c1','a.exe','h1',1),
			('a2','{}','a2','org-2','Tenant 2','c2','b.exe','h2',1)`)
	out, err := runTL(t, "", "applications", "hunt", "--cert", "Example", "--limit", "1", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("hunt: %v\n%s", err, out)
	}
	var got huntView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if len(got.TrustedFiles) != 1 {
		t.Fatalf("--limit 1 shows %d trusted-file rows, want 1", len(got.TrustedFiles))
	}
	pending := map[string]int{}
	for _, r := range got.TenantRollup {
		pending[r.Organization] = r.Pending
	}
	if pending["Tenant 1"] != 1 || pending["Tenant 2"] != 1 {
		t.Fatalf("tenantRollup %+v; want both tenants (the rollup must cover every hash the cert resolves to)", got.TenantRollup)
	}
}

// Class stale join membership: an online_devices row that a later sync no
// longer returns is never pruned. It is evidence an endpoint is online only
// when it was synced no earlier than the computers row it joins, and devices
// health and organizations posture apply the same rule.
func TestOnlineEvidenceMustBeAsFreshAsTheComputerRow(t *testing.T) {
	dbPath := seedTLStore(t,
		`INSERT INTO organizations (id, data, organization_id, name) VALUES ('org-1','{}','org-1','Tenant 1')`,
		`INSERT INTO computers (id, data, synced_at, computer_id, computer_name, organization_id, organization_name, is_online, is_isolated) VALUES
			('c1','{}','2026-10-20T00:00:00Z','c1','PC-1','org-1','Tenant 1',0,0),
			('c2','{}','2026-10-10T00:00:00Z','c2','PC-2','org-1','Tenant 1',0,0)`,
		`INSERT INTO online_devices (id, data, synced_at, computer_id, organization_id) VALUES
			('c1','{}','2026-10-10T00:00:00Z','c1','org-1'),
			('c2','{}','2026-10-20T00:00:00Z','c2','org-1')`)

	out, err := runTL(t, "", "devices", "health", "--all-tenants", "--detail", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("devices health: %v\n%s", err, out)
	}
	var devices []deviceHealth
	if err := json.Unmarshal([]byte(out), &devices); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	class := map[string]string{}
	for _, d := range devices {
		class[d.ComputerID] = d.HealthClass
	}
	if class["c1"] == "online" {
		t.Errorf("c1 is online from an online_devices row older than its computers row: %v", class)
	}
	if class["c2"] != "online" {
		t.Errorf("c2 lost the fresher online_devices evidence: %v", class)
	}

	out, err = runTL(t, "", "organizations", "posture", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("posture: %v\n%s", err, out)
	}
	var posture struct {
		Tenants []tenantPosture `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(out), &posture); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if len(posture.Tenants) != 1 || posture.Tenants[0].Online != 1 {
		t.Fatalf("posture %+v; want one tenant with exactly one online endpoint (same rule as devices health)", posture.Tenants)
	}
}

// Class superseded snapshot rows: maintenance windows carry no vendor id, so
// an edited window (end shortened) lands as a second row next to the old one.
// One row per window identity (computer, type, start), the most recently
// synced, in the scanner both maintenance open and posture read.
func TestMaintenanceCollapsesSupersededWindowVersions(t *testing.T) {
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	start := ts(-72 * time.Hour)
	dbPath := seedTLStore(t,
		`INSERT INTO organizations (id, data, organization_id, name) VALUES ('org-1','{}','org-1','Tenant 1')`,
		`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, organization_name) VALUES ('c1','{}','c1','PC-1','org-1','Tenant 1')`,
		`INSERT INTO maintenance (id, data, synced_at, computer_id, maintenance_type, start_date_time, end_date_time, created_by) VALUES
			('m-old','{}','2026-10-01T00:00:00Z','c1','Learning','`+start+`','`+ts(48*time.Hour)+`','tech'),
			('m-new','{}','2026-10-02T00:00:00Z','c1','Learning','`+start+`','`+ts(-1*time.Hour)+`','tech'),
			('m-inst','{}','2026-10-01T00:00:00Z','c1','Installation','`+ts(-2*time.Hour)+`','','tech')`)

	out, err := runTL(t, "", "maintenance", "open", "--all-tenants", "--older-than", "24h", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("maintenance open: %v\n%s", err, out)
	}
	var got struct {
		Open []struct {
			MaintenanceType string `json:"maintenanceType"`
		} `json:"open"`
		OpenCount int `json:"open_count"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if got.OpenCount != 1 || got.Open[0].MaintenanceType != "Installation" {
		t.Fatalf("open windows %+v; want only the Installation window (the Learning window's latest version is closed)", got.Open)
	}

	out, err = runTL(t, "", "organizations", "posture", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("posture: %v\n%s", err, out)
	}
	var posture struct {
		Tenants []tenantPosture `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(out), &posture); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if len(posture.Tenants) != 1 || posture.Tenants[0].OpenMaintenance != 1 {
		t.Fatalf("posture %+v; want one open maintenance window", posture.Tenants)
	}
}

// Class partial coverage read as fresh: a pull that ends early still saves the
// rows it fetched, which advances their synced_at. retention-check must read
// coverage from complete pulls only.
func TestRetentionCheckReadsOnlyCompletePulls(t *testing.T) {
	failPage2 := true
	_, base := newTLFixture(t, func(path string, body map[string]any) (int, any) {
		if !strings.HasSuffix(path, "/ActionLog/ActionLogGetByParametersV2") {
			return 404, map[string]any{}
		}
		if p, _ := body["pageNumber"].(float64); p == 1 {
			return 200, []map[string]any{{"eActionLogId": "l1", "organizationId": "org-1", "dateCreated": "2026-10-20T00:00:00Z"}}
		}
		if failPage2 {
			return 200, map[string]any{"success": false}
		}
		return 200, []map[string]any{}
	})
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	if out, err := runTL(t, base, "audit", "pull", "--source", "audit", "--since", "30d", "--page-size", "1", "--db", dbPath, "--json"); err == nil {
		t.Fatalf("a pull with a failed page succeeded:\n%s", out)
	}
	status := func() string {
		t.Helper()
		out, err := runTL(t, "", "audit", "retention-check", "--json", "--db", dbPath)
		if err != nil {
			t.Fatalf("retention-check: %v\n%s", err, out)
		}
		var rows []retentionRow
		if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
			t.Fatalf("decode %s: %v", out, err)
		}
		return rows[0].Status
	}
	if s := status(); s == "ok" {
		t.Fatalf("retention-check reports %q after only an incomplete pull", s)
	}

	failPage2 = false
	if out, err := runTL(t, base, "audit", "pull", "--source", "audit", "--since", "30d", "--page-size", "1", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("complete pull: %v\n%s", err, out)
	}
	if s := status(); s != "ok" {
		t.Fatalf("retention-check reports %q after a complete pull, want ok", s)
	}
}

// Class in-band failure ignored on a write: HTTP 200 whose body declares
// failure is a failed approval. Any failure signal wins.
func TestApproveBatchCountsInBandFailure(t *testing.T) {
	rec, base := newTLFixture(t, tlAnswerRecheckPending(func(string, map[string]any) (int, any) {
		return 200, map[string]any{"success": false, "message": "approval failed"}
	}))
	dbPath := seedTLStore(t, tlPendingApprovals)
	out, err := runTL(t, base, "approvals", "approve-batch", "--hash", "aaaa", "--all-tenants", "--yes", "--json", "--db", dbPath)
	if err == nil {
		t.Fatalf("approve-batch reported success on {success:false}:\n%s", out)
	}
	var got struct {
		Approved int `json:"approved"`
	}
	if jerr := json.Unmarshal([]byte(out[:strings.LastIndex(out, "}")+1]), &got); jerr != nil || got.Approved != 0 {
		t.Fatalf("approved=%d (decode err %v); want 0\n%s", got.Approved, jerr, out)
	}
	writes := 0
	for _, r := range rec.seen() {
		if strings.HasSuffix(r.Path, "/ApprovalRequest/ApprovalRequestPermitApplication") {
			writes++
		}
	}
	if writes != 2 {
		t.Fatalf("sent %d permit write(s), want 2: the in-band failure must be the write's own\n%s", writes, out)
	}
}

// Class local-data opt-out bypass: under THREATLOCKER_NO_LOCAL_DATA=1 nothing
// may write the local store. audit pull copies API data into it, so it is
// refused before any request; the read-only novels must not create a store.
func TestLocalDataOptOutCoversEveryStoreWriter(t *testing.T) {
	rec, base := newTLFixture(t, func(string, map[string]any) (int, any) { return 200, []map[string]any{} })
	dir := t.TempDir()
	t.Setenv(noLocalDataEnvVar, "1")

	pullDB := filepath.Join(dir, "pull.db")
	if out, err := runTL(t, base, "audit", "pull", "--since", "1d", "--db", pullDB); err == nil {
		t.Fatalf("audit pull ran under %s=1:\n%s", noLocalDataEnvVar, out)
	}
	if n := len(rec.seen()); n != 0 {
		t.Fatalf("audit pull sent %d request(s) under the opt-out", n)
	}
	if _, err := os.Stat(pullDB); err == nil {
		t.Fatalf("audit pull created %s under the opt-out", pullDB)
	}

	readDB := filepath.Join(dir, "read.db")
	_, _ = runTL(t, "", "devices", "health", "--all-tenants", "--json", "--db", readDB)
	if _, err := os.Stat(readDB); err == nil {
		t.Fatalf("devices health created a store at %s under the opt-out", readDB)
	}
}

// The class pin: every command that saves live API data into the store
// (pp:data-source live + mcp:local-write) is on the opt-out guard's list.
func TestEveryLiveStoreWriterIsOptOutGuarded(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Annotations["pp:data-source"] == "live" && c.Annotations["mcp:local-write"] == "true" {
			path := strings.Join(strings.Fields(c.CommandPath())[1:], " ")
			if !localDataWriterCommands[path] {
				t.Errorf("%q writes live API data to the store but is not refused under %s=1", path, noLocalDataEnvVar)
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(RootCmd())
}

// Class unaccounted rows: a bare null body is not an empty page, and a row the
// store skipped without counting it leaves the pull incomplete.
func TestAuditPullAccountsForEveryRow(t *testing.T) {
	body := any(nil)
	_, base := newTLFixture(t, func(path string, _ map[string]any) (int, any) {
		if strings.HasSuffix(path, "/ActionLog/ActionLogGetByParametersV2") {
			return 200, body
		}
		return 404, map[string]any{}
	})
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	if out, err := runTL(t, base, "audit", "pull", "--source", "audit", "--since", "1d", "--db", dbPath, "--json"); err == nil || !strings.Contains(out, `"complete": false`) {
		t.Fatalf("a null body passed as a complete pull: err=%v\n%s", err, out)
	}
	body = []any{map[string]any{"eActionLogId": "l1", "organizationId": "org-1"}, 7}
	if out, err := runTL(t, base, "audit", "pull", "--source", "audit", "--since", "1d", "--page-size", "5", "--db", dbPath, "--json"); err == nil || !strings.Contains(out, `"complete": false`) {
		t.Fatalf("a skipped, uncounted row passed as a complete pull: err=%v\n%s", err, out)
	}
}

// Round 2, class partial coverage read as fresh (time axis): a complete pull
// whose window starts after the last complete coverage leaves a gap. It must
// not advance coverage past the gap; a pull that overlaps the old end does.
func TestRetentionCoverageNeedsAGapFreeWindow(t *testing.T) {
	_, base := newTLFixture(t, func(path string, body map[string]any) (int, any) {
		if !strings.HasSuffix(path, "/ActionLog/ActionLogGetByParametersV2") {
			return 404, map[string]any{}
		}
		return 200, []map[string]any{{"eActionLogId": "l1", "organizationId": "org-1", "dateCreated": "2026-09-05T00:00:00Z"}}
	})
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	pull := func(args ...string) {
		t.Helper()
		full := append([]string{"audit", "pull", "--source", "audit", "--db", dbPath, "--json"}, args...)
		if out, err := runTL(t, base, full...); err != nil {
			t.Fatalf("audit pull %v: %v\n%s", args, err, out)
		}
	}
	status := func() string {
		t.Helper()
		out, err := runTL(t, "", "audit", "retention-check", "--json", "--db", dbPath)
		if err != nil {
			t.Fatalf("retention-check: %v\n%s", err, out)
		}
		var rows []retentionRow
		if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
			t.Fatalf("decode %s: %v", out, err)
		}
		return rows[0].Status
	}
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	pull("--since", old.Add(-24*time.Hour).Format(time.RFC3339), "--until", old.Format(time.RFC3339))
	pull("--since", "1h")
	if s := status(); s == "ok" {
		t.Fatalf("a recent pull that left a 40-day gap reads %q", s)
	}
	pull("--since", old.Add(-time.Hour).Format(time.RFC3339))
	if s := status(); s != "ok" {
		t.Fatalf("a pull that closes the gap reads %q, want ok", s)
	}
}

// Round 2, class local-data opt-out bypass: resolving the default store path
// claims a legacy data.db by writing its owner marker. Under the opt-out no
// command may do that, CLI or MCP.
func TestLocalDataOptOutNeverClaimsTheDefaultStore(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("THREATLOCKER_API_KEY", tlFixtureToken)
	t.Setenv(noLocalDataEnvVar, "1")
	setLegacyDBClaimSuppressed(false) // only the opt-out may suppress the claim here
	path, err := MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	unscoped := filepath.Join(dir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = defaultDBPath("threatlocker-cli")
	if _, err := MCPStorePath(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unscoped + ".owner"); err == nil {
		t.Fatalf("store path resolution wrote %s under %s=1", unscoped+".owner", noLocalDataEnvVar)
	}
}
