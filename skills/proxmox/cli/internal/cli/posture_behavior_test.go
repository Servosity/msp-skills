// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func postureByVMID(t *testing.T, args ...string) (postureReport, map[int64]postureRow) {
	t.Helper()
	out, stderr, err := runCLI(t, append([]string{"posture", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("posture: %v\nstderr: %s", err, stderr)
	}
	rep := decodeJSON[postureReport](t, out)
	rows := map[int64]postureRow{}
	for _, r := range rep.Rows {
		rows[r.VMID] = r
	}
	return rep, rows
}

func TestPostureVerdicts(t *testing.T) {
	f := newFixture(t)
	rep, rows := postureByVMID(t)

	want := map[int64]string{
		100: "fresh", 101: "stale", 102: "uncovered", 103: "uncovered", 104: "verify-failed",
		105: "never", 107: "unverified", 108: "last-attempt-failed",
	}
	for vmid, verdict := range want {
		if rows[vmid].Verdict != verdict {
			t.Errorf("vmid %d verdict = %q (%s), want %q", vmid, rows[vmid].Verdict, rows[vmid].Reason, verdict)
		}
	}
	if _, ok := rows[106]; ok {
		t.Error("template 106 must be skipped by default")
	}
	if len(rep.Rows) != len(want) {
		t.Errorf("got %d rows, want %d", len(rep.Rows), len(want))
	}
	// The disabled-job blind spot: upstream not-backed-up omits 102, we must not.
	if !strings.Contains(rows[102].Reason, "disabled job") || !strings.Contains(rows[102].Reason, "job2") {
		t.Errorf("102 reason = %q, want disabled-job attribution", rows[102].Reason)
	}
	if rep.Red != 5 || rep.Amber != 2 {
		t.Errorf("red=%d amber=%d, want 5 and 2", rep.Red, rep.Amber)
	}
	// Freshness comes from the newest archive (2h), not the older one.
	if r := rows[100]; r.AgeHours < 1.5 || r.AgeHours > 2.5 || r.VerifyState != "ok" || r.LastSource != "pve-storage:pbs1" {
		t.Errorf("100 = %+v", r)
	}
	// The PBS API duplicates of PVE archives must not inflate counts.
	if rows[100].RestorePoints != 3 { // 2h + 26h from PVE, plus the PBS-only 50h point
		t.Errorf("100 restore points = %d, want 3 (dedupe)", rows[100].RestorePoints)
	}
	// Offsite evidence comes from the push sync job covering vm/100 on datastore main.
	if rows[100].OffsiteLastSuccessAt == "" {
		t.Errorf("100: push sync job not recognised as offsite evidence")
	}
	if rows[108].LastAttemptStatus != "failed" || rows[104].LastAttemptStatus != "incomplete" {
		t.Errorf("attempts: 108=%q 104=%q", rows[108].LastAttemptStatus, rows[104].LastAttemptStatus)
	}
	for _, r := range rep.Rows {
		if r.Vendor != "proxmox" || r.AssetID == "" || r.Tenant == "" {
			t.Errorf("row %d missing vendor-neutral fields: %+v", r.VMID, r)
		}
	}
	if rep.Rows[0].Severity != "red" {
		t.Errorf("rows must be sorted red first, got %q", rep.Rows[0].Severity)
	}
	if n := f.writeCalls.Load(); n != 0 {
		t.Fatalf("posture issued %d non-GET requests", n)
	}
}

func TestPostureProtectedElsewhereAndFilters(t *testing.T) {
	newFixture(t)
	_, rows := postureByVMID(t, "--protected-elsewhere-tag", "veeam")
	if rows[103].Verdict != "protected-elsewhere" || rows[103].Severity != "green" {
		t.Errorf("103 = %q/%q", rows[103].Verdict, rows[103].Severity)
	}
	if rows[102].Verdict != "uncovered" {
		t.Errorf("untagged 102 must stay uncovered, got %q", rows[102].Verdict)
	}
	rep, _ := postureByVMID(t, "--only", "red")
	for _, r := range rep.Rows {
		if r.Severity != "red" {
			t.Errorf("--only red returned %s row %d", r.Severity, r.VMID)
		}
	}
	if _, _, err := runCLI(t, "posture", "--only", "purple", "--json"); err == nil || ExitCode(err) != 2 {
		t.Errorf("bad --only must be a usage error (exit 2), got %v", err)
	}
	_, rows = postureByVMID(t, "--stale-after", "3d")
	if rows[101].Verdict != "verify-failed" && rows[101].Verdict == "stale" {
		t.Errorf("with --stale-after 3d the 50h point is not stale, got %q", rows[101].Verdict)
	}
}

func TestPostureByClientAndFailOnRed(t *testing.T) {
	newFixture(t)
	out, _, err := runCLI(t, "posture", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	clients := decodeJSON[[]postureClientRow](t, out)
	if len(clients) != 1 || clients[0].Guests != 8 || clients[0].Red != 5 || clients[0].Uncovered != 2 {
		t.Fatalf("by-client = %+v", clients)
	}
	_, _, err = runCLI(t, "posture", "--json", "--fail-on-red")
	if err == nil || ExitCode(err) != 1 {
		t.Fatalf("--fail-on-red with red rows must exit 1, got %v", err)
	}
}

func TestPostureMultiClusterIsolation(t *testing.T) {
	f := newFixture(t)
	t.Setenv("ACME_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("ACME_PBS", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	// Env pins are cleared: each cluster must carry its own.
	t.Setenv("PROXMOX_TLS_FINGERPRINT", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	writeClustersFile(t, `
[clusters.acme]
client = "Acme Dental"
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.beta]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`)
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatalf("posture: %v\n%s", err, stderr)
	}
	rep := decodeJSON[postureReport](t, out)
	if len(rep.FetchFailures) != 0 {
		t.Fatalf("fetch failures: %+v", rep.FetchFailures)
	}
	points := map[string]int{}
	for _, r := range rep.Rows {
		if r.VMID == 100 {
			points[r.Cluster] = r.RestorePoints
		}
		if r.Cluster == "acme" && r.Tenant != "Acme Dental" || r.Cluster == "beta" && r.Tenant != "beta" {
			t.Errorf("tenant mismatch: %s/%s", r.Cluster, r.Tenant)
		}
	}
	// beta has no PBS host: it must not inherit PROXMOX_PBS_HOST, so it misses the PBS-only point.
	if points["acme"] != 3 || points["beta"] != 2 {
		t.Fatalf("vmid 100 restore points per cluster = %v, want acme=3 beta=2", points)
	}
	out, _, err = runCLI(t, "--cluster", "beta", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if rep := decodeJSON[postureReport](t, out); len(rep.Clusters) != 1 || rep.Clusters[0] != "beta" {
		t.Fatalf("--cluster beta visited %v", rep.Clusters)
	}
	out, _, err = runCLI(t, "posture", "--client", "acme dental", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if rep := decodeJSON[postureReport](t, out); len(rep.Clusters) != 1 || rep.Clusters[0] != "acme" {
		t.Fatalf("--client filter visited %v", rep.Clusters)
	}
}
