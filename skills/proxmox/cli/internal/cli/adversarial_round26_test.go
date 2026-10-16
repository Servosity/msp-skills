package cli

import (
	"net/http"
	"strings"
	"sync/atomic"
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

// Any pruning policy on the target storage (keep-daily+weekly, or keep-last
// with an older survivor) may have deleted a run's point when a newer one was
// kept: unproven. keep-all deletes nothing: the same misses are proven.
func TestRetentionThinnedRunsAreUnproven(t *testing.T) {
	for _, tc := range []struct {
		policy   string
		unproven bool
	}{{"keep-daily=2,keep-weekly=4", true}, {"keep-last=1", true}, {"keep-all=1", false}} {
		f := newFixture(t)
		f.pbsPrune = tc.policy
		f.pbsRoutes["/admin/prune"] = []any{}
		addPointAt(f, 100, -170*time.Hour, "ok") // a weekly survivor before the window
		out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
		r := decodeJSON[rpoReport](t, out).Rows[0]
		if r.MissedRuns == 0 {
			t.Fatalf("%s: want missed runs between the 170h and 50h points: %+v", tc.policy, r)
		}
		if got := r.MissedRunsUnproven == r.MissedRuns && r.Evidence == "partial"; got != tc.unproven {
			t.Errorf("%s: missed %d unproven %d evidence %s", tc.policy, r.MissedRuns, r.MissedRunsUnproven, r.Evidence)
		}
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

// Two jobs on one target at one time owe two points; with keep-all the
// missing one is proven, with pruning it is unproven (the second may have
// been pruned as the older of the two).
func TestTwoJobsOneTarget(t *testing.T) {
	for _, tc := range []struct {
		policy   string
		unproven bool
	}{{"keep-all=1", false}, {"keep-daily=2", true}} {
		f := newFixture(t)
		f.pbsPrune = tc.policy
		addJob3(f, "pbs1")
		out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
		r := decodeJSON[rpoReport](t, out).Rows[0]
		if r.MissedRuns == 0 || r.MetSchedule || (r.MissedRunsUnproven > 0) != tc.unproven {
			t.Errorf("%s: missed %d unproven %d met %v", tc.policy, r.MissedRuns, r.MissedRunsUnproven, r.MetSchedule)
		}
	}
}

// vzdump.conf's policy is asked for the job's target storage (the defaults
// API overlays that storage's own retention), and a PBS prune job the token
// cannot see makes retention unknown.
func TestRetentionReadsTargetDefaultsAndHiddenPruneJobs(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	addPointAt(f, 100, -170*time.Hour, "ok")
	var asked atomic.Int64
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/vzdump/defaults") && r.URL.Query().Get("storage") == "pbs1" {
			asked.Add(1)
		}
		return false
	})
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; asked.Load() == 0 || r.MissedRunsUnproven != 0 {
		t.Errorf("defaults asked for pbs1 %d time(s); unproven %d", asked.Load(), r.MissedRunsUnproven)
	}
	f.pbsRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": 1}}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MissedRunsUnproven != r.MissedRuns {
		t.Errorf("hidden prune jobs: missed %d unproven %d", r.MissedRuns, r.MissedRunsUnproven)
	}
}

// remove=0 on the job: vzdump prunes nothing after a backup, so its
// prune-backups deletes nothing and the misses stand as proven.
func TestRemoveOffPrunesNothing(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["remove"], jobs[0]["prune-backups"] = 0, "keep-daily=2"
	addPointAt(f, 100, -170*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.MissedRunsUnproven != 0 {
		t.Errorf("remove=0: missed %d unproven %d", r.MissedRuns, r.MissedRunsUnproven)
	}
}
