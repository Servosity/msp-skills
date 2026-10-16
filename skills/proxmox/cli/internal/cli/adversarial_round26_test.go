package cli

import (
	"testing"
	"time"
)

// Two jobs at the same time each owe the guest a restore point: job3 never
// produced one, so its runs are missed even though job1's points exist.
func TestCoincidentJobsEachNeedAPoint(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "21:00", "storage": "local", "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MetSchedule {
		t.Errorf("job3 never backed VM 100 up, yet: missed %d met %v (jobs %v)", r.MissedRuns, r.MetSchedule, r.Jobs)
	}
}

// A miss older than a restore point that was kept is unproven whatever the
// retention (keep-all included): retention may have pruned it, or the job's
// schedule or guest list may have differed then, and Proxmox keeps no
// history of either. Misses after the newest point stay proven.
func TestRetentionThinnedRunsAreUnproven(t *testing.T) {
	for _, policy := range []string{"keep-daily=2,keep-weekly=4", "keep-last=1", "keep-all=1"} {
		f := newFixture(t)
		f.pbsPrune = policy
		addPointAt(f, 100, -170*time.Hour, "ok") // an old point before the window
		out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
		r := decodeJSON[rpoReport](t, out).Rows[0]
		if r.MissedRuns == 0 || r.MissedRunsUnproven != r.MissedRuns || r.Evidence != "partial" {
			t.Errorf("%s: missed %d unproven %d evidence %s", policy, r.MissedRuns, r.MissedRunsUnproven, r.Evidence)
		}
	}
	newFixture(t)
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "101", "--json") // newest point 50h old: later runs missed
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MissedRunsUnproven != 0 {
		t.Errorf("misses after the newest point must be proven: missed %d unproven %d", r.MissedRuns, r.MissedRunsUnproven)
	}
}

// A PBS prune job on the datastore thins on its own, whatever the PVE side keeps.
func TestPBSPruneJobThinsToo(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/prune"] = []any{map[string]any{"id": "p1", "store": "main", "ns": "", "schedule": "daily", "keep-daily": 2, "keep-weekly": 4}}
	addPointAt(f, 100, -170*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MissedRunsUnproven != r.MissedRuns {
		t.Errorf("PBS prune job keep-daily=2: missed %d unproven %d", r.MissedRuns, r.MissedRunsUnproven)
	}
}

// A push sync whose remote is this same Backup Server copies nothing offsite.
func TestPushToSelfIsNotOffsite(t *testing.T) {
	for _, tc := range []struct {
		remote  map[string]any
		offsite bool
	}{
		{map[string]any{"name": "dr", "host": "dr.example.net", "port": 8007}, true},
		{map[string]any{"name": "dr", "host": "localhost", "port": 8007}, false},
		{map[string]any{"name": "dr", "host": "127.0.0.1"}, false},
		{nil, false}, // unreadable
	} {
		f := newFixture(t)
		if tc.remote == nil {
			delete(f.pbsRoutes, "/config/remote/dr")
		} else {
			f.pbsRoutes["/config/remote/dr"] = tc.remote
		}
		out, _, _ := runCLI(t, "posture", "--json")
		credited := false
		for _, r := range decodeJSON[postureReport](t, out).Rows {
			credited = credited || r.OffsiteLastSuccessAt != ""
		}
		if credited != tc.offsite {
			t.Errorf("remote %v: offsite credited %v, want %v", tc.remote, credited, tc.offsite)
		}
	}
}

// The env setup's PBS pin catches a push to this same server under another name.
func TestPushToSelfByEnvPinIsNotOffsite(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/config/remote/dr"] = map[string]any{"name": "dr", "host": "pbs-alias.example.net", "port": 8007, "fingerprint": f.pbsFingerprint()}
	out, _, _ := runCLI(t, "posture", "--json")
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.OffsiteLastSuccessAt != "" {
			t.Errorf("VM %d credited offsite by a push to this PBS under another name", r.VMID)
		}
	}
}

func addJob3(f *fixture, storage string) {
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "21:00", "storage": storage, "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
}

// Another job's points (here an extra manual backup on job1's target in
// every window) never stand in for a job whose own target holds none.
func TestOtherTargetsPointsNeverCoverAJob(t *testing.T) {
	f := newFixture(t)
	addJob3(f, "local")
	addPointAt(f, 100, -25*time.Hour, "ok")
	addPointAt(f, 100, -time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MetSchedule {
		t.Errorf("job3 (local) has no point, yet missed %d met %v", r.MissedRuns, r.MetSchedule)
	}
}

// Two jobs on one target at one time owe two points: one point per run is
// not met, whatever the retention.
func TestTwoJobsOneTarget(t *testing.T) {
	for _, policy := range []string{"keep-all=1", "keep-daily=2"} {
		f := newFixture(t)
		f.pbsPrune = policy
		addJob3(f, "pbs1")
		out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
		if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MetSchedule {
			t.Errorf("%s: missed %d met %v", policy, r.MissedRuns, r.MetSchedule)
		}
	}
}
