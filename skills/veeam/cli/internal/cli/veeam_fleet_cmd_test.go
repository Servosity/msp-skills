// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/store"
)

// seedFleetStore writes a small multi-tenant fixture into a fresh temp store
// and returns the db path. Records carry the documented response shapes
// (spec.json list items) and go in through UpsertBatch under the partition
// sync uses for each documented path, so the sync key and partition names are
// exercised too. Alpha Corp is unhealthy (a failed + stale job, an agent that
// is NotRunning, an error alarm, an at-risk workload); Beta LLC is mostly fine.
func seedFleetStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "veeam-test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	markNovelPartitionsSynced(t, db)

	now := time.Now()
	iso := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	put := func(path string, objs ...map[string]any) { seedPath(t, db, path, objs...) }

	put(veeamPathCompanies,
		map[string]any{"instanceUid": "org-a", "name": "Alpha Corp", "status": "Active"},
		map[string]any{"instanceUid": "org-b", "name": "Beta LLC", "status": "Active"})

	// Two jobs share the name "Nightly" in different tenants: keyed on
	// instanceUid they stay two rows (keyed on name they collapsed into one).
	put(veeamPathServerJobs,
		map[string]any{"instanceUid": "j1", "name": "Alpha-DB", "organizationUid": "org-a",
			"status": "Failed", "lastRun": iso(5 * 24 * time.Hour), "isEnabled": true,
			"backupServerUid": "bs1", "failureMessage": "repository unreachable"},
		map[string]any{"instanceUid": "j2", "name": "Nightly", "organizationUid": "org-a",
			"status": "Success", "lastRun": iso(time.Hour), "isEnabled": true, "backupServerUid": "bs1"},
		map[string]any{"instanceUid": "j3", "name": "Nightly", "organizationUid": "org-b",
			"status": "Warning", "lastRun": iso(2 * time.Hour), "isEnabled": true, "backupServerUid": "bs2"})

	put(veeamPathAgents,
		map[string]any{"instanceUid": "a1", "name": "alpha-ws1", "organizationUid": "org-a", "status": "Active"},
		map[string]any{"instanceUid": "a2", "name": "alpha-ws2", "organizationUid": "org-a", "status": "NotRunning", "activationTime": iso(3 * time.Hour)},
		map[string]any{"instanceUid": "a3", "name": "beta-ws1", "organizationUid": "org-b", "status": "Active"})
	// The per-OS list repeats an agent without status or organizationUid
	// (WindowsBackupAgent in spec.json); it must not count again.
	put("/infrastructure/backupAgents/windows", map[string]any{"instanceUid": "a1", "name": "alpha-ws1"})

	put(veeamPathAlarmTemplates,
		map[string]any{"instanceUid": "tpl-cpu", "name": "High CPU"},
		map[string]any{"instanceUid": "tpl-disk", "name": "Low space"})
	put(veeamPathActiveAlarms,
		map[string]any{"instanceUid": "al1", "alarmTemplateUid": "tpl-cpu", "repeatCount": float64(3),
			"object":         map[string]any{"organizationUid": "org-a", "objectName": "alpha-srv"},
			"lastActivation": map[string]any{"status": "Error", "time": iso(time.Hour), "message": "CPU > 95%"}},
		map[string]any{"instanceUid": "al2", "alarmTemplateUid": "tpl-disk",
			"object":         map[string]any{"organizationUid": "org-b", "objectName": "beta-srv"},
			"lastActivation": map[string]any{"status": "Warning", "time": iso(2 * time.Hour), "message": "disk 80%"}})

	put(veeamPathWorkloadsServer,
		map[string]any{"instanceUid": "w1", "name": "srv01", "organizationUid": "org-a", "latestRestorePointDate": iso(5 * 24 * time.Hour)},
		map[string]any{"instanceUid": "w2", "name": "srv02", "organizationUid": "org-a", "latestRestorePointDate": iso(time.Hour)})

	put(veeamPathLicenseUsage, licenseUsageItem("org-a", 100, 20), licenseUsageItem("org-b", 45))
	return dbPath
}

// licenseUsageItem is an OrganizationLicenseUsage item (spec.json): points live
// in servers[].workloads[].usedUnits; an Instances workload is not points.
func licenseUsageItem(org string, points ...float64) map[string]any {
	workloads := []any{map[string]any{"unitType": "Instances", "usedUnits": float64(7)}}
	for _, p := range points {
		workloads = append(workloads, map[string]any{"unitType": "Points", "usedUnits": p})
	}
	return map[string]any{"organizationUid": org, "organizationType": "Company",
		"servers": []any{map[string]any{"serverType": "BackupAndReplication", "workloads": workloads}}}
}

// veeamNovelPaths is every documented list path the novels read.
var veeamNovelPaths = []string{veeamPathCompanies, veeamPathProviderCompanies, veeamPathServerJobs,
	veeamPathBackupServers, veeamPathAgentJobs, veeamPathAgents, veeamPathActiveAlarms, veeamPathAlarmTemplates,
	veeamPathWorkloadsServer, veeamPathWorkloadsConsole, veeamPathLicenseUsage}

// markNovelPartitionsSynced records a completed sync for every novel
// partition, as a full `sync` run leaves it.
func markNovelPartitionsSynced(t *testing.T, db *store.Store) {
	t.Helper()
	for _, p := range veeamNovelPaths {
		for _, name := range veeamSyncNames(p) {
			if err := db.SaveSyncState(name, "", 0); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// seedPath stores objs the way sync does for a documented list path.
func seedPath(t *testing.T, db *store.Store, path string, objs ...map[string]any) {
	t.Helper()
	names := veeamSyncNames(path)
	if len(names) != 1 {
		t.Fatalf("%s syncs into %v, want exactly one partition", path, names)
	}
	items := make([]json.RawMessage, 0, len(objs))
	for _, o := range objs {
		b, _ := json.Marshal(o)
		items = append(items, b)
	}
	stored, skipped, err := db.UpsertBatch(names[0], items)
	if err != nil || stored != len(items) || skipped != 0 {
		t.Fatalf("seed %s (%s): stored=%d skipped=%d err=%v", path, names[0], stored, skipped, err)
	}
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
	seedPath(t, s, veeamPathLicenseUsage, licenseUsageItem("org-a", 100, 50))
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

// TestNovelPathsResolveToSyncPartitions fails when a regeneration stops
// syncing a documented path the novels read, or splits it across partitions.
func TestNovelPathsResolveToSyncPartitions(t *testing.T) {
	for _, p := range veeamNovelPaths {
		names := veeamSyncNames(p)
		if len(names) != 1 {
			t.Errorf("%s syncs into %v, want exactly one partition", p, names)
			continue
		}
		if !slices.Contains(defaultSyncResources(), names[0]) {
			t.Errorf("%s (%s) is not in the default sync set; `sync` would leave the novels empty", p, names[0])
		}
	}
}

// TestSyncKeepsSameNamedRecordsApart pins the instanceUid sync key: two jobs
// named "Nightly" in different tenants are two rows.
func TestSyncKeepsSameNamedRecordsApart(t *testing.T) {
	dbPath := seedFleetStore(t)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, veeamSyncNames(veeamPathServerJobs)[0]).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("server jobs stored = %d, want 3 (same-named jobs collapsed)", n)
	}
}
