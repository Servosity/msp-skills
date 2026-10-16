package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func setCreated(f *fixture, vmid int, hoursAgo float64) {
	f.pveRoutes[fmt.Sprintf("/nodes/pve1/qemu/%d/config", vmid)] = map[string]any{
		"name": "x", "meta": fmt.Sprintf("creation-qemu=8.1.2,ctime=%d", ago(f.now, hoursAgo))}
}

// VM 100 was deleted and a new VM 100 created 30 minutes ago; the old VM's
// retained backups are not the new VM's, on every surface that credits them.
func TestReusedVMIDNotCreditedWithEarlierGuestsBackups(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 0.5)
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Severity != "red" || !strings.Contains(r.Reason, "predate this VM's creation") {
		t.Errorf("posture credited an earlier guest's backups: %s/%s %q", r.Verdict, r.Severity, r.Reason)
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Count != 0 || g.PredatingGuest == 0 {
		t.Errorf("restore-points listed an earlier guest's backups: count %d predating %d", g.Count, g.PredatingGuest)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 0 {
		t.Errorf("rpo counted an earlier guest's backups: %d", r.RestorePoints)
	}
}

// A restore keeps the VM's original creation time: its older backups stay its own.
func TestRestoredVMKeepsItsBackups(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 24*30)
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict != "fresh" {
		t.Errorf("an old ctime dropped the VM's own backups: %s %q", r.Verdict, r.Reason)
	}
}

// A failed task from before the VM's creation does not carry its name.
func TestFailuresDoNotNameAnEarlierGuest(t *testing.T) {
	f := newFixture(t)
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	tasks[0]["starttime"], tasks[0]["endtime"] = ago(f.now, 5), ago(f.now, 4.5)
	setCreated(f, 108, 2)
	out, _, _ := runCLI(t, "failures", "--json")
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.VMID == 108 && r.GuestName != "" {
			t.Errorf("a task from before VM 108 existed is named %q", r.GuestName)
		}
	}
}

// A covered guest whose newest (future-dated) point failed verification is
// verify-failed even when the rest would read never or stale.
func TestFutureFailedDecidesOverNeverAndStale(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 105, 3*time.Hour, "failed") // 105: no other point
	addPointAt(f, 101, 3*time.Hour, "failed") // 101: newest real point is 50h old
	out, _, _ := runCLI(t, "posture", "--json")
	for _, id := range []int64{105, 101} {
		if r := postureRowFor(t, out, id); r.Verdict != "verify-failed" || r.VerifyState != "failed" {
			t.Errorf("%d: want verify-failed/failed, got %s/%s %q", id, r.Verdict, r.VerifyState, r.Reason)
		}
	}
}
