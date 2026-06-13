// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/store"
)

// seedFleetStore writes a small multi-tenant fixture into a fresh temp store
// and returns the db path. Alpha Corp is unhealthy (a failed + stale job, an
// offline agent, an error alarm, an at-risk workload); Beta LLC is mostly fine.
func seedFleetStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "veeam-test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	now := time.Now()
	iso := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	put := func(rtype, id string, obj map[string]any) {
		b, _ := json.Marshal(obj)
		if err := db.Upsert(rtype, id, b); err != nil {
			t.Fatalf("upsert %s/%s: %v", rtype, id, err)
		}
	}

	put("organizations-companies", "org-a", map[string]any{"instanceUid": "org-a", "name": "Alpha Corp", "status": "Active"})
	put("organizations-companies", "org-b", map[string]any{"instanceUid": "org-b", "name": "Beta LLC", "status": "Active"})

	put("infrastructure-backup-servers-jobs", "j1", map[string]any{
		"instanceUid": "j1", "name": "Alpha-DB", "organizationUid": "org-a",
		"status": "Failed", "lastRun": iso(5 * 24 * time.Hour), "isEnabled": true,
		"backupServerUid": "bs1", "failureMessage": "repository unreachable",
	})
	put("infrastructure-backup-servers-jobs", "j2", map[string]any{
		"instanceUid": "j2", "name": "Alpha-VM", "organizationUid": "org-a",
		"status": "Success", "lastRun": iso(time.Hour), "isEnabled": true, "backupServerUid": "bs1",
	})
	put("infrastructure-backup-servers-jobs", "j3", map[string]any{
		"instanceUid": "j3", "name": "Beta-Files", "organizationUid": "org-b",
		"status": "Warning", "lastRun": iso(2 * time.Hour), "isEnabled": true, "backupServerUid": "bs2",
	})

	put("infrastructure-backup-agents", "a1", map[string]any{"instanceUid": "a1", "name": "alpha-ws1", "organizationUid": "org-a", "status": "Online"})
	put("infrastructure-backup-agents", "a2", map[string]any{"instanceUid": "a2", "name": "alpha-ws2", "organizationUid": "org-a", "status": "Offline", "activationTime": iso(3 * time.Hour)})
	put("infrastructure-backup-agents", "a3", map[string]any{"instanceUid": "a3", "name": "beta-ws1", "organizationUid": "org-b", "status": "Online"})

	put("alarms", "al1", map[string]any{
		"instanceUid": "al1", "name": "High CPU", "repeatCount": float64(3),
		"object":         map[string]any{"organizationUid": "org-a", "objectName": "alpha-srv"},
		"lastActivation": map[string]any{"status": "Error", "time": iso(time.Hour), "message": "CPU > 95%"},
	})
	put("alarms", "al2", map[string]any{
		"instanceUid": "al2", "name": "Low space",
		"object":         map[string]any{"organizationUid": "org-b", "objectName": "beta-srv"},
		"lastActivation": map[string]any{"status": "Warning", "time": iso(2 * time.Hour), "message": "disk 80%"},
	})

	put("protected-workloads-computers-managed-by-backup-server", "w1", map[string]any{
		"instanceUid": "w1", "name": "srv01", "organizationUid": "org-a", "latestRestorePointDate": iso(5 * 24 * time.Hour),
	})
	put("protected-workloads-computers-managed-by-backup-server", "w2", map[string]any{
		"instanceUid": "w2", "name": "srv02", "organizationUid": "org-a", "latestRestorePointDate": iso(time.Hour),
	})

	put("licensing-usage-organizations", "u1", map[string]any{"organizationUid": "org-a", "organizationName": "Alpha Corp", "usedPoints": float64(120)})
	put("licensing-usage-organizations", "u2", map[string]any{"organizationUid": "org-b", "organizationName": "Beta LLC", "usedPoints": float64(45)})

	return dbPath
}

func runNovelJSON(t *testing.T, cmd *cobra.Command, args ...string) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s: %v (out=%s)", cmd.Name(), err, buf.String())
	}
	var v map[string]any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("unmarshal %s: %v (out=%s)", cmd.Name(), err, buf.String())
	}
	return v
}

func arr(v map[string]any, key string) []map[string]any {
	raw, _ := v[key].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func TestFleetHealthCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelFleetHealthCmd(&rootFlags{asJSON: true}), "--db", db)
	companies := arr(v, "companies")
	if len(companies) != 2 {
		t.Fatalf("companies = %d, want 2", len(companies))
	}
	// Alpha (unhealthy) must sort first.
	alpha := companies[0]
	if alpha["company"] != "Alpha Corp" {
		t.Errorf("first company = %v, want Alpha Corp (worst-first sort)", alpha["company"])
	}
	if alpha["failed"].(float64) != 1 {
		t.Errorf("alpha failed = %v, want 1", alpha["failed"])
	}
	if alpha["agents_offline"].(float64) != 1 {
		t.Errorf("alpha agents_offline = %v, want 1", alpha["agents_offline"])
	}
	if alpha["active_alarms"].(float64) != 1 {
		t.Errorf("alpha active_alarms = %v, want 1", alpha["active_alarms"])
	}
}

func TestStaleBackupsCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelStaleBackupsCmd(&rootFlags{asJSON: true}), "--db", db, "--days", "3")
	jobs := arr(v, "jobs")
	var foundStale bool
	for _, j := range jobs {
		if j["job"] == "Alpha-VM" {
			t.Error("Alpha-VM ran 1h ago and succeeded; must not be stale")
		}
		if j["job"] == "Alpha-DB" {
			foundStale = true
			if j["days_stale"].(float64) < 4 {
				t.Errorf("Alpha-DB days_stale = %v, want ~5", j["days_stale"])
			}
		}
	}
	if !foundStale {
		t.Error("Alpha-DB (5 days stale) must be reported")
	}
}

func TestAtRiskCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelAtRiskCmd(&rootFlags{asJSON: true}), "--db", db, "--rpo", "24h")
	workloads := arr(v, "workloads")
	if len(workloads) != 1 {
		t.Fatalf("at-risk workloads = %d, want 1 (srv01)", len(workloads))
	}
	if workloads[0]["workload"] != "srv01" {
		t.Errorf("at-risk workload = %v, want srv01", workloads[0]["workload"])
	}
}

func TestAlarmsTriageCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", db)
	alarms := arr(v, "alarms")
	if len(alarms) != 2 {
		t.Fatalf("alarms = %d, want 2", len(alarms))
	}
	if alarms[0]["severity"] != "Error" {
		t.Errorf("first alarm severity = %v, want Error (most urgent first)", alarms[0]["severity"])
	}
	if alarms[0]["count"].(float64) != 3 {
		t.Errorf("error alarm count = %v, want 3 (repeatCount)", alarms[0]["count"])
	}
	// severity filter
	filtered := runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", db, "--severity", "Warning")
	if got := len(arr(filtered, "alarms")); got != 1 {
		t.Errorf("warning-only alarms = %d, want 1", got)
	}
}

func TestLicenseUsageCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelLicenseUsageCmd(&rootFlags{asJSON: true}), "--db", db, "--no-snapshot")
	orgs := arr(v, "organizations")
	if len(orgs) != 2 {
		t.Fatalf("orgs = %d, want 2", len(orgs))
	}
	if orgs[0]["organization"] != "Alpha Corp" || orgs[0]["used_points"].(float64) != 120 {
		t.Errorf("top org = %v/%v, want Alpha Corp/120", orgs[0]["organization"], orgs[0]["used_points"])
	}
	if v["total_used"].(float64) != 165 {
		t.Errorf("total_used = %v, want 165", v["total_used"])
	}
}

func TestLicenseUsageDelta(t *testing.T) {
	db := seedFleetStore(t)
	// First run records a snapshot.
	runNovelJSON(t, newNovelLicenseUsageCmd(&rootFlags{asJSON: true}), "--db", db)
	// Bump Alpha usage, then a second run should show a positive delta.
	s, err := store.Open(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	b, _ := json.Marshal(map[string]any{"organizationUid": "org-a", "organizationName": "Alpha Corp", "usedPoints": float64(150)})
	if err := s.Upsert("licensing-usage-organizations", "u1", b); err != nil {
		t.Fatalf("bump: %v", err)
	}
	s.Close()
	v := runNovelJSON(t, newNovelLicenseUsageCmd(&rootFlags{asJSON: true}), "--db", db, "--no-snapshot")
	for _, o := range arr(v, "organizations") {
		if o["organization"] == "Alpha Corp" {
			if o["delta"].(float64) != 30 {
				t.Errorf("Alpha delta = %v, want 30", o["delta"])
			}
		}
	}
}

func TestCompanyOverviewCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelCompanyOverviewCmd(&rootFlags{asJSON: true}), "--db", db, "--company", "Alpha Corp")
	if v["jobs_total"].(float64) != 2 {
		t.Errorf("jobs_total = %v, want 2", v["jobs_total"])
	}
	if v["jobs_failed"].(float64) != 1 {
		t.Errorf("jobs_failed = %v, want 1", v["jobs_failed"])
	}
	if v["agents_offline"].(float64) != 1 {
		t.Errorf("agents_offline = %v, want 1", v["agents_offline"])
	}
	if v["workloads_at_risk"].(float64) != 1 {
		t.Errorf("workloads_at_risk = %v, want 1", v["workloads_at_risk"])
	}
	if v["license_used_points"].(float64) != 120 {
		t.Errorf("license_used_points = %v, want 120", v["license_used_points"])
	}
	if v["backup_servers"].(float64) != 1 {
		t.Errorf("backup_servers = %v, want 1 (distinct backupServerUid)", v["backup_servers"])
	}
}

func TestCompanyOverviewRequiresCompany(t *testing.T) {
	// Bare invocation (no args, no flags) shows help and exits 0.
	cmd := newNovelCompanyOverviewCmd(&rootFlags{})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Errorf("bare company-overview should show help (nil), got %v", err)
	}
}

func TestSinceCommand(t *testing.T) {
	db := seedFleetStore(t)
	v := runNovelJSON(t, newNovelSinceCmd(&rootFlags{asJSON: true}), "--db", db, "240h")
	events := arr(v, "events")
	if len(events) == 0 {
		t.Fatal("since 240h must surface recent events")
	}
	var sawJob, sawAlarm bool
	for _, e := range events {
		switch e["name"] {
		case "Alpha-DB":
			sawJob = true
		case "High CPU":
			sawAlarm = true
		}
	}
	if !sawJob {
		t.Error("failed job Alpha-DB must appear as a since event")
	}
	if !sawAlarm {
		t.Error("fired alarm High CPU must appear as a since event")
	}
}
