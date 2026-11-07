// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/store"
)

// Review round 1 (Codex): one test per class, each against documented
// shapes in spec.json.

// emptyFleetStore is a store whose every novel partition finished a sync with
// no rows, so a test can seed only what it needs.
func emptyFleetStore(t *testing.T) (string, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "veeam-test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	markNovelPartitionsSynced(t, db)
	return dbPath, db
}

func runNovelErr(cmd *cobra.Command, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// Class: tenant attribution. BackupServerJob.organizationUid is the server
// owner; mappedOrganizationUid is the organization the job is assigned to.
func TestJobsAttributeToTheAssignedTenant(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	seedPath(t, db, veeamPathCompanies,
		map[string]any{"instanceUid": "owner", "name": "Provider"},
		map[string]any{"instanceUid": "tenant", "name": "Tenant Co"})
	seedPath(t, db, veeamPathServerJobs, map[string]any{"instanceUid": "j1", "name": "Tenant-DB",
		"organizationUid": "owner", "mappedOrganizationUid": "tenant", "status": "Failed",
		"isEnabled": true, "lastRun": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})
	db.Close()
	v := runNovelJSON(t, newNovelCompanyOverviewCmd(&rootFlags{asJSON: true}), "--db", dbPath, "--company", "tenant")
	if v["jobs_failed"].(float64) != 1 {
		t.Errorf("tenant jobs_failed = %v, want 1 (job is mapped to the tenant)", v["jobs_failed"])
	}
	for _, c := range arr(runNovelJSON(t, newNovelFleetHealthCmd(&rootFlags{asJSON: true}), "--db", dbPath), "companies") {
		if c["company_uid"] == "owner" && c["failed"].(float64) != 0 {
			t.Errorf("server owner charged with the tenant's failed job: %v", c)
		}
	}
}

// Class: incomplete read reported as complete.
func TestNovelsRefuseAnIncompleteMirror(t *testing.T) {
	dbPath := seedFleetStore(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress(veeamSyncNames(veeamPathServerJobs)[0], "100", 100); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err := runNovelErr(newNovelFleetHealthCmd(&rootFlags{asJSON: true}), "--db", dbPath)
	if err == nil || !strings.Contains(err.Error(), veeamPathServerJobs) {
		t.Errorf("fleet-health over a partially synced job list: err=%v out=%s, want an error naming %s", err, out, veeamPathServerJobs)
	}

	never := filepath.Join(t.TempDir(), "never.db")
	nd, err := store.Open(never)
	if err != nil {
		t.Fatal(err)
	}
	nd.Close()
	if _, err := runNovelErr(newNovelStaleBackupsCmd(&rootFlags{asJSON: true}), "--db", never); err == nil {
		t.Error("stale-backups over a store that never synced the job lists returned a report")
	}
	if _, err := runNovelErr(newNovelAtRiskCmd(&rootFlags{asJSON: true}), "--db", filepath.Join(t.TempDir(), "absent.db")); err == nil {
		t.Error("at-risk with no local mirror returned a report")
	}
}

// Class: wrong timestamp field. lastRun is when the latest session started;
// lastEndTime is when it ended.
func TestSinceTimesJobEventsBySessionEnd(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	now := time.Now()
	seedPath(t, db, veeamPathServerJobs, map[string]any{"instanceUid": "j1", "name": "Long-Job",
		"organizationUid": "o", "status": "Failed", "isEnabled": true,
		"lastRun": now.Add(-72 * time.Hour).UTC().Format(time.RFC3339), "lastEndTime": now.Add(-time.Hour).UTC().Format(time.RFC3339)})
	db.Close()
	found := false
	for _, e := range arr(runNovelJSON(t, newNovelSinceCmd(&rootFlags{asJSON: true}), "--db", dbPath, "24h"), "events") {
		found = found || e["name"] == "Long-Job"
	}
	if !found {
		t.Error("a failure that ended an hour ago is missing from since 24h")
	}
}

// Class: display name used as tenant identity.
func TestCompanyNamesAreNotIdentities(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	seedPath(t, db, veeamPathCompanies,
		map[string]any{"instanceUid": "a", "name": "Acme"},
		map[string]any{"instanceUid": "b", "name": "Acme"})
	seedPath(t, db, veeamPathAlarmTemplates, map[string]any{"instanceUid": "tpl", "name": "Job failed"})
	seedPath(t, db, veeamPathActiveAlarms,
		map[string]any{"instanceUid": "al1", "alarmTemplateUid": "tpl", "object": map[string]any{"organizationUid": "a", "objectName": "srv"},
			"lastActivation": map[string]any{"status": "Error", "time": time.Now().UTC().Format(time.RFC3339)}},
		map[string]any{"instanceUid": "al2", "alarmTemplateUid": "tpl", "object": map[string]any{"organizationUid": "b", "objectName": "srv"},
			"lastActivation": map[string]any{"status": "Error", "time": time.Now().UTC().Format(time.RFC3339)}})
	db.Close()
	if _, err := runNovelErr(newNovelCompanyOverviewCmd(&rootFlags{asJSON: true}), "--db", dbPath, "--company", "Acme"); err == nil {
		t.Error("company-overview --company Acme matched one of two companies named Acme")
	}
	if got := len(arr(runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", dbPath), "alarms")); got != 2 {
		t.Errorf("alarms-triage merged two tenants named Acme: %d groups, want 2", got)
	}
}

// Class: resolved alarms reported as active (AlarmActivation.status Resolved).
func TestResolvedAlarmsAreNotActive(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	seedPath(t, db, veeamPathCompanies, map[string]any{"instanceUid": "a", "name": "Alpha"})
	seedPath(t, db, veeamPathActiveAlarms,
		map[string]any{"instanceUid": "al1", "object": map[string]any{"organizationUid": "a"}, "lastActivation": map[string]any{"status": "Resolved", "time": time.Now().UTC().Format(time.RFC3339)}},
		map[string]any{"instanceUid": "al2", "object": map[string]any{"organizationUid": "a"}, "lastActivation": map[string]any{"status": "Warning", "time": time.Now().UTC().Format(time.RFC3339)}})
	db.Close()
	v := runNovelJSON(t, newNovelFleetHealthCmd(&rootFlags{asJSON: true}), "--db", dbPath)
	if n := arr(v, "companies")[0]["active_alarms"].(float64); n != 1 {
		t.Errorf("fleet-health active_alarms = %v, want 1 (one alarm is Resolved)", n)
	}
	if n := runNovelJSON(t, newNovelCompanyOverviewCmd(&rootFlags{asJSON: true}), "--db", dbPath, "--company", "a")["active_alarms"].(float64); n != 1 {
		t.Errorf("company-overview active_alarms = %v, want 1", n)
	}
	if got := len(arr(runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", dbPath), "alarms")); got != 1 {
		t.Errorf("alarms-triage lists %d alarms, want 1 (Resolved is not active)", got)
	}
}

// Class: a value the local data cannot know. Alarm history is not synced, so
// triage must not claim a first-seen time.
func TestAlarmsTriageClaimsNoFirstSeen(t *testing.T) {
	v := runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", seedFleetStore(t))
	for _, a := range arr(v, "alarms") {
		if _, ok := a["first_seen"]; ok {
			t.Errorf("alarms-triage reports first_seen %v from the latest activation alone", a["first_seen"])
		}
	}
}

// Class: inventory counted from references. BackupServer.organizationUid
// names the owner; a server with no jobs still exists.
func TestCompanyOverviewCountsServerInventory(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	seedPath(t, db, veeamPathCompanies, map[string]any{"instanceUid": "c", "name": "Co"})
	seedPath(t, db, veeamPathBackupServers, map[string]any{"instanceUid": "s1", "organizationUid": "c", "name": "VBR1"})
	db.Close()
	if n := runNovelJSON(t, newNovelCompanyOverviewCmd(&rootFlags{asJSON: true}), "--db", dbPath, "--company", "c")["backup_servers"].(float64); n != 1 {
		t.Errorf("backup_servers = %v, want 1 (synced server inventory)", n)
	}
}

// Class: documented enum value left unclassified. Every value spec.json
// documents for a job status maps to a class.
func TestJobHealthCoversEveryDocumentedStatus(t *testing.T) {
	raw, err := os.ReadFile("../../spec.json")
	if err != nil {
		t.Skip("spec.json not next to the module:", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	type schema struct {
		Ref        string             `json:"$ref"`
		Enum       []string           `json:"enum"`
		AllOf      []schema           `json:"allOf"`
		Properties map[string]*schema `json:"properties"`
	}
	lookup := func(name string) schema {
		var s schema
		_ = json.Unmarshal(spec.Components.Schemas[name], &s)
		return s
	}
	var enumOf func(s schema) []string
	enumOf = func(s schema) []string {
		if s.Ref != "" {
			return enumOf(lookup(s.Ref[strings.LastIndex(s.Ref, "/")+1:]))
		}
		if len(s.Enum) > 0 {
			return s.Enum
		}
		for _, a := range s.AllOf {
			if e := enumOf(a); len(e) > 0 {
				return e
			}
		}
		return nil
	}
	idle := map[string]bool{"Unknown": true, "None": true, "Idle": true}
	for _, name := range []string{"BackupServerJob", "BackupAgentJob"} {
		st := lookup(name).Properties["status"]
		if st == nil {
			t.Fatalf("%s has no status property", name)
		}
		values := enumOf(*st)
		if len(values) == 0 {
			t.Fatalf("%s.status has no documented enum", name)
		}
		for _, v := range values {
			if got := veeamJobHealth(v); got == "other" && !idle[v] {
				t.Errorf("%s.status %q is unclassified", name, v)
			}
		}
	}
}

// P2: the license_usage MCP tool is read-only, so a call over MCP records
// no snapshot.
func TestLicenseUsageOverMCPRecordsNoSnapshot(t *testing.T) {
	dbPath := seedFleetStore(t)
	t.Setenv("VEEAM_MCP_CHILD", "1")
	runNovelJSON(t, newNovelLicenseUsageCmd(&rootFlags{asJSON: true}), "--db", dbPath)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM veeam_usage_snapshots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("license_usage over MCP recorded %d snapshots", n)
	}
}

// P2: snapshots order by time even when fractional seconds differ in length.
func TestUsageSnapshotsSortChronologically(t *testing.T) {
	_, db := emptyFleetStore(t)
	defer db.Close()
	base := time.Date(2026, 11, 2, 14, 0, 0, 0, time.UTC)
	if err := db.RecordUsageSnapshot(t.Context(), "o", 100, base.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordUsageSnapshot(t.Context(), "o", 120, base.Add(110*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	got, _, ok, err := db.LatestUsageSnapshot(t.Context(), "o", base.Add(time.Second))
	if err != nil || !ok || got != 120 {
		t.Errorf("latest snapshot = %v (ok=%v err=%v), want 120", got, ok, err)
	}
}
