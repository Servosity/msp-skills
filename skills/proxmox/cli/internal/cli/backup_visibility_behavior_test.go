// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// auditOnly gives the fixture's PVE token PVEAuditor's privileges only. Real
// Proxmox VE then answers every storage content list with an empty list:
// PVE::Storage::check_volume_access shows a backup volume only with
// Datastore.Allocate, or Datastore.AllocateSpace plus VM.Backup.
func auditOnly(f *fixture) {
	f.pveRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1}}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{}
}

func failureByScope(fs []fetchFailure) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.Scope] = f.Error
	}
	return out
}

// An audit-only token must not turn "PVE hid the backups" into "no backups".
// The PBS storage is read through the PBS API, so only the dir storage fails.
func TestAuditOnlyTokenHiddenBackupsAreMissingEvidence(t *testing.T) {
	f := newFixture(t)
	auditOnly(f)
	rep, rows := postureByVMID(t)
	fails := failureByScope(rep.FetchFailures)
	if e := fails["storage local@pve1"]; !strings.Contains(e, "Datastore.AllocateSpace") {
		t.Errorf("hidden dir storage must be a fetch failure naming the privilege, got %q (all: %+v)", e, rep.FetchFailures)
	}
	if e, ok := fails["storage pbs1"]; ok {
		t.Errorf("pbs1 is read through the PBS API; its hidden PVE list must not fail: %s", e)
	}
	if r := rows[100]; r.Verdict != "archives-incomplete" || r.RestorePoints == 0 {
		t.Errorf("100 (fresh on PBS, dir storage unseen) = %s %q, %d points", r.Verdict, r.Reason, r.RestorePoints)
	}
	if r := rows[105]; r.Verdict != "never" || !strings.Contains(r.Reason, "could not be read") {
		t.Errorf("105 must say its storage could not be read, got %s %q", r.Verdict, r.Reason)
	}

	// restore-points and rpo read archives the same way.
	out, stderr, err := runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatalf("restore-points: %v\n%s", err, stderr)
	}
	rp := decodeJSON[restorePointsReport](t, out)
	if e := failureByScope(rp.FetchFailures)["storage local@pve1"]; !strings.Contains(e, "VM.Backup") {
		t.Errorf("restore-points must report the hidden dir storage, got %+v", rp.FetchFailures)
	}
	out, stderr, err = runCLI(t, "rpo", "--days", "7", "--json")
	if err != nil {
		t.Fatalf("rpo: %v\n%s", err, stderr)
	}
	rpo := decodeJSON[rpoReport](t, out)
	if e := failureByScope(rpo.FetchFailures)["storage local@pve1"]; e == "" {
		t.Errorf("rpo must report the hidden dir storage, got %+v", rpo.FetchFailures)
	}
	for _, r := range rpo.Rows {
		if r.MetSchedule {
			t.Errorf("guest %d: met_schedule cannot be proven while a backup storage is unreadable", r.VMID)
		}
	}
}

// Without the PBS API, the PBS storage's PVE list is the only source: hidden
// means unread there too.
func TestAuditOnlyTokenWithoutPBSAPI(t *testing.T) {
	f := newFixture(t)
	auditOnly(f)
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	rep, rows := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["storage pbs1"]; !strings.Contains(e, "Datastore.AllocateSpace") {
		t.Errorf("hidden PBS storage must be a fetch failure, got %+v", rep.FetchFailures)
	}
	if r := rows[100]; r.Verdict != "never" || !strings.Contains(r.Reason, "see fetch_failures") {
		t.Errorf("100 = %s %q, want never with the unreadable-storage reason", r.Verdict, r.Reason)
	}
}

// A snapshot with no manifest (a backup still running, or crash debris) has no
// size and cannot be restored: it must never count as the newest point.
func TestManifestlessSnapshotIsNotARestorePoint(t *testing.T) {
	f := newFixture(t)
	_, before := postureByVMID(t)

	recent := ago(f.now, 1)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = append(f.pbsRoutes["/admin/datastore/main/snapshots"].([]any),
		map[string]any{"backup-type": "vm", "backup-id": "101", "backup-time": recent, "files": []string{"index.json.blob"}, "owner": "root@pam"})
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = append(f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any),
		map[string]any{"volid": "pbs1:backup/vm/101/incomplete", "vmid": 101, "ctime": recent, "subtype": "qemu", "format": "pbs-vm"})

	_, after := postureByVMID(t)
	if r := after[101]; r.Verdict != "stale" || r.RestorePoints != before[101].RestorePoints {
		t.Errorf("101 with a manifest-less snapshot = %s %q, %d points (before: %d); want stale and unchanged",
			r.Verdict, r.Reason, r.RestorePoints, before[101].RestorePoints)
	}
}
