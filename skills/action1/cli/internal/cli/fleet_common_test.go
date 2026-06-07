// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"action1-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// seedFleetStore builds a temp SQLite store populated with a small two-org
// fleet: endpoints with missing-update + vulnerability summaries, CVEs (one a
// CISA KEV), installed software, and automation instances. Returns the db path.
func seedFleetStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	old := now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)

	endpoints := []map[string]any{
		{"id": "ep-1", "name": "WS-CRITICAL", "OS": "Windows 11", "organization_id": "org-a",
			"last_seen": recent, "online_status": "Online", "reboot_required": "Yes",
			"missing_updates": map[string]any{"critical": 8, "other": 12},
			"vulnerabilities": map[string]any{"critical": 3}},
		{"id": "ep-2", "name": "WS-HEALTHY", "OS": "Windows 10", "organization_id": "org-a",
			"last_seen": recent, "online_status": "Online", "reboot_required": "No",
			"missing_updates": map[string]any{"critical": 0, "other": 1},
			"vulnerabilities": map[string]any{"critical": 0}},
		{"id": "ep-3", "name": "SRV-DARK", "OS": "Windows Server 2022", "organization_id": "org-b",
			"last_seen": old, "online_status": "Offline", "reboot_required": "No",
			"missing_updates": map[string]any{"critical": 2, "other": 4},
			"vulnerabilities": map[string]any{"critical": 1}},
	}
	for _, e := range endpoints {
		seedUpsert(t, db, "endpoints", e["id"].(string), e)
	}

	vulns := []map[string]any{
		{"cve_id": "CVE-2025-0001", "cvss_score": "9.8", "cisa_kev": "Yes", "endpoints_count": 5,
			"organization_id": "org-a", "remediation_status": "Overdue",
			"software": map[string]any{"name": "Acme Reader"}},
		{"cve_id": "CVE-2025-0002", "cvss_score": "7.1", "cisa_kev": "No", "endpoints_count": 2,
			"organization_id": "org-b", "remediation_status": "Due_soon",
			"software": map[string]any{"name": "Widget Suite"}},
	}
	for _, v := range vulns {
		seedUpsert(t, db, "vulnerabilities", v["cve_id"].(string), v)
	}

	apps := []map[string]any{
		{"id": "app-1", "name": "Google Chrome", "version": "120.0", "publisher": "Google", "organization_id": "org-a", "endpoint_id": "ep-1"},
		{"id": "app-2", "name": "Google Chrome", "version": "120.0", "publisher": "Google", "organization_id": "org-b", "endpoint_id": "ep-3"},
		{"id": "app-3", "name": "7-Zip", "version": "23.01", "publisher": "Igor Pavlov", "organization_id": "org-a", "endpoint_id": "ep-2"},
	}
	for _, a := range apps {
		seedUpsert(t, db, "installed_software_data", a["id"].(string), a)
	}

	instances := []map[string]any{
		{"self": "inst-1", "status": "Completed", "percent_completed": 100, "organization_id": "org-a"},
		{"self": "inst-2", "status": "Failed", "percent_completed": 60, "organization_id": "org-a"},
		{"self": "inst-3", "status": "Running", "percent_completed": 30, "organization_id": "org-b"},
	}
	for _, i := range instances {
		seedUpsert(t, db, "automation_instances", i["self"].(string), i)
	}

	return dbPath
}

// emptyFleetStore returns a path to an initialized but empty store, to verify
// the empty-store / exit-0 contract.
func emptyFleetStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	db.Close()
	return dbPath
}

func seedUpsert(t *testing.T, db *store.Store, resourceType, id string, obj map[string]any) {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal %s/%s: %v", resourceType, id, err)
	}
	if err := db.Upsert(resourceType, id, raw); err != nil {
		t.Fatalf("upsert %s/%s: %v", resourceType, id, err)
	}
}

// runFleet executes a fleet leaf command against the seeded store and returns
// its stdout. flags.asJSON is forced on so output is machine-parseable.
func runFleet(t *testing.T, build func(*rootFlags) *cobra.Command, dbPath string, extraArgs ...string) []byte {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf) // keep stderr sync-hints out of the parsed JSON
	args := append([]string{"--db", dbPath}, extraArgs...)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s: %v\nstdout: %s\nstderr: %s", cmd.Name(), err, out.String(), errBuf.String())
	}
	return out.Bytes()
}

// The fleet_common contract says every fleet command tolerates an empty store
// by emitting valid JSON (empty rows / zeroed aggregates) with exit 0. Enforce
// it fleet-wide so a new command can't regress the contract silently.
func TestFleetCommandsEmptyStoreContract(t *testing.T) {
	builders := map[string]func(*rootFlags) *cobra.Command{
		"patch-posture":     newNovelFleetPatchPostureCmd,
		"vuln-triage":       newNovelFleetVulnTriageCmd,
		"stale":             newNovelFleetStaleCmd,
		"patch-drift":       newNovelFleetPatchDriftCmd,
		"software-rollup":   newNovelFleetSoftwareRollupCmd,
		"health-score":      newNovelFleetHealthScoreCmd,
		"automation-health": newNovelFleetAutomationHealthCmd,
		"org-scorecard":     newNovelFleetOrgScorecardCmd,
		"reboot-pending":    newNovelFleetRebootPendingCmd,
	}
	for name, build := range builders {
		db := emptyFleetStore(t)
		out := runFleet(t, build, db) // runFleet fails the test on non-zero exit
		var v any
		if err := json.Unmarshal(out, &v); err != nil {
			t.Errorf("%s: empty-store output is not valid JSON: %v\n%s", name, err, out)
		}
	}
}
