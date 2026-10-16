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

// keep-daily=2,keep-weekly=4 on the target storage thins older dailies: a
// run older than two days with a newer point retained may have had one.
// keep-last alone thins nothing: the same misses are proven.
func TestRetentionThinnedRunsAreUnproven(t *testing.T) {
	for _, tc := range []struct {
		policy   string
		unproven bool
	}{{"keep-daily=2,keep-weekly=4", true}, {"keep-last=30", false}} {
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

func TestDenseSince(t *testing.T) {
	now := time.Unix(100*86400, 0)
	var daily []int64
	for d := int64(30); d >= 1; d-- {
		daily = append(daily, now.Unix()-d*86400)
	}
	for _, tc := range []struct {
		policy keepPolicy
		want   int64
	}{
		{nil, 0},
		{keepPolicy{"keep-all": 1}, 0},
		{keepPolicy{"keep-last": 3}, 0},
		{keepPolicy{"keep-daily": 7}, now.Unix() - 7*86400},
		{keepPolicy{"keep-last": 10, "keep-weekly": 4}, daily[len(daily)-10]},
		{keepPolicy{"keep-weekly": 4}, now.Unix()}, // daily runs: weekly keeps no run reliably
		{keepPolicy{"keep-last": 40, "keep-weekly": 4}, 0},
	} {
		if got := denseSince(tc.policy, daily, now); got != tc.want {
			t.Errorf("%v: got %d want %d", tc.policy, got, tc.want)
		}
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
