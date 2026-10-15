// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// A deeper ACL replaces what it would inherit: privileges granted on / do not
// reach /storage/pbs1 when that path has its own, narrower ACL.
func TestDeeperACLOverridesInheritedBackupPrivileges(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":             map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
		"/storage/pbs1": map[string]any{"Datastore.Audit": 0},
	}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	rep, rows := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["storage pbs1"]; !strings.Contains(e, "Datastore.AllocateSpace") {
		t.Errorf("pbs1 is hidden by its own ACL; want a fetch failure, got %+v", rep.FetchFailures)
	}
	if r := rows[100]; r.Severity == "green" {
		t.Errorf("100 must not be green while pbs1 is unread: %s %q", r.Verdict, r.Reason)
	}
}

// PBS lists only the groups a token owns unless it holds Datastore.Audit on
// the namespace: a partial snapshot list must not pass as the whole store.
func TestPBSSnapshotListNeedsDatastoreAudit(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": true}, "/datastore/main": map[string]any{"Datastore.Backup": true}}
	_, stderr, err := runCLI(t, "verify-health", "--json")
	if err == nil || !strings.Contains(stderr, "Datastore.Audit") {
		t.Errorf("verify-health with an owner-only PBS token: err = %v, stderr %q; want a failure naming Datastore.Audit", err, stderr)
	}
	rep, _ := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["pbs:main"]; !strings.Contains(e, "Datastore.Audit") {
		t.Errorf("posture must report the PBS list as unread, got %+v", rep.FetchFailures)
	}
}

// Proxmox VE lists only the guests a token may audit; without VM.Audit on /vms
// the missing ones must be reported, not silently absent.
func TestGuestListWithoutVMAuditOnVMs(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":        map[string]any{"Sys.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
		"/vms/100": map[string]any{"VM.Audit": 0},
	}
	rep, _ := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["guests"]; !strings.Contains(e, "VM.Audit") {
		t.Errorf("posture must flag the incomplete guest list, got %+v", rep.FetchFailures)
	}
	out, _, err := runCLI(t, "backup", "coverage", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"scope": "guests"`) {
		t.Errorf("backup coverage must flag the incomplete guest list:\n%s", out)
	}
	if _, _, err := runCLI(t, "restore-points", "999", "--json"); err == nil || !strings.Contains(err.Error(), "could not be resolved") {
		t.Errorf("restore-points for an unseen guest must not claim 'not found': %v", err)
	}
}

// pbs_namespace names a namespace on the datastores THIS cluster writes to;
// the same namespace on another datastore can be another client's.
func TestPBSNamespaceStaysOnThisClustersDatastores(t *testing.T) {
	f := newFixture(t)
	t.Setenv("ACME_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("ACME_PBS", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main"}, {"store": "other"}}
	f.pbsRoutes["/admin/datastore/other/snapshots"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "105", "backup-time": ago(f.now, 1), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}},
	}
	writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
pbs_namespace = "prod"
`)
	_, rows := postureByVMID(t)
	if r := rows[105]; r.Verdict != "never" {
		t.Errorf("105 was credited with another datastore's snapshot: %s %q (%d points)", r.Verdict, r.Reason, r.RestorePoints)
	}
}

// An unreadable verify-job list means no outdated-after applies, so a
// verification of any age is unproven, not ok.
func TestVerifyAgeUnknownWithoutVerifyJobs(t *testing.T) {
	f := newFixture(t)
	delete(f.pbsRoutes, "/admin/verify")
	out, _, err := runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Status == "ok" {
			t.Errorf("group %s is ok although the verify-job list was unreadable", g.Group)
		}
	}
}

// A PBS usage entry without totals is unmeasured, not 0% used.
func TestCapacityMissingPBSUsageIsUnknown(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/status/datastore-usage"] = []map[string]any{{"store": "main"}}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[capacityReport](t, out)
	for _, r := range rep.Rows {
		if r.Kind == "pbs-datastore" && r.Name == "main" {
			t.Errorf("main reported from an empty usage entry: %+v", r)
		}
	}
	if e := failureByScope(rep.FetchFailures)["pbs main"]; !strings.Contains(e, "no usage") {
		t.Errorf("want a fetch failure for the unmeasured store, got %+v", rep.FetchFailures)
	}
}

// An unfilled .mcpb prompt arrives as the literal ${user_config.x}: unset.
func TestUnfilledMCPBPromptsAreUnset(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_CLUSTER", "${user_config.proxmox_cluster}")
	t.Setenv("PROXMOX_CLUSTERS_FILE", "${user_config.proxmox_clusters_file}")
	if _, stderr, err := runCLI(t, "posture", "--json"); err != nil {
		t.Fatalf("posture with unfilled prompts: %v\n%s", err, stderr)
	}
}

// The nearest listed ACL path decides alone (PVE::AccessControl::roles
// replaces inherited roles at a deeper ACL).
func TestTokenPrivsNearestListedPathDecides(t *testing.T) {
	p := tokenPrivs{"/": {"X": true}, "/storage/pbs1": {"Y": false}}
	for _, c := range []struct {
		priv, path string
		want       bool
	}{
		{"X", "/storage/local", true},   // inherited from / (propagate)
		{"X", "/storage/pbs1", false},   // replaced by the deeper ACL
		{"Y", "/storage/pbs1", true},    // granted on the path itself
		{"Y", "/storage/pbs1/x", false}, // no propagate bit
	} {
		if got := p.has(c.priv, c.path); got != c.want {
			t.Errorf("has(%s, %s) = %v, want %v", c.priv, c.path, got, c.want)
		}
	}
}
