// Adversarial round 31 receipts: a moved job's backup taken by another job, retention on other nodes and disabled jobs, notes.

package cli

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// job4 (now DISABLED) wrote VM 100 to nas with prune-backups
// keep-daily=1,keep-weekly=4 and ran until 2 days ago; job1 (enabled, no
// retention) writes VM 100 to nas too. job4's prunes thinned job1's archives.
func TestRound31DisabledJobRetention(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["storage"] = "nas"
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job4", "enabled": 0, "schedule": "22:00", "storage": "nas", "vmid": "100", "prune-backups": "keep-daily=1,keep-weekly=4"})
	f.pveRoutes["/cluster/backup/job4/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "nas", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas", "content": "backup"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "nas", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	var nas []any
	for _, d := range []float64{19.5, 12.5, 5.5, 2.5, 1.5, 0.5} {
		nas = append(nas, vzdumpEntry("nas", 100, f.now.Add(-time.Duration(d*24*float64(time.Hour))+20*time.Minute).Unix()))
	}
	f.pveRoutes["/nodes/pve1/storage/nas/content"] = nas
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("jobs=%v missed=%d unproven=%d evidence=%s met=%v times=%v note=%q", r.Jobs, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.MissedRunTimes, r.Note)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN misses with evidence=%s; the disabled job4's retention on the same storage is never consulted", certain, r.Evidence)
	}
}

// job1 (hourly, VM 100 only) was moved from local to pbs1 at moveAgo; before
// that each run wrote its archive to local. job3 (daily 22:00-ish, many VMs)
// writes local and reaches VM 100 late in its queue (2.5h in). Every run of
// both jobs made a restore point. keep-all everywhere; pbs1 holds one old
// manual backup (no window cap).
func adv32MovedSetup(t *testing.T, startAgo, moveAgo, job3NewestAgo time.Duration) *fixture {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["schedule"] = "hourly"
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "22:00", "storage": "local", "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	day := int64(86400)
	j1Newest := f.now.Add(-30 * time.Minute).Unix()
	j3Newest := f.now.Add(-job3NewestAgo).Unix()
	runsServer(t, f, map[string][2]int64{"hourly": {j1Newest, 3600}, "22:00": {j3Newest, day}})
	addPointAt(f, 100, -200*time.Hour, "ok") // old manual backup on pbs1
	var local []any
	start := f.now.Add(-startAgo).Unix()
	move := f.now.Add(-moveAgo).Unix()
	for r := j1Newest - 60*3600; r <= j1Newest; r += 3600 {
		if r < start-3600 {
			continue
		}
		if r < move {
			local = append(local, vzdumpEntry("local", 100, r+60))
		} else {
			addPointAt(f, 100, time.Unix(r+60, 0).Sub(f.now), "ok")
		}
	}
	for r := j3Newest - 3*day; r <= j3Newest; r += day {
		local = append(local, vzdumpEntry("local", 100, r+9000)) // 2.5h into job3's queue
	}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = local
	return f
}

// Realistic: the move was 30h ago, inside a 2-day window.
func TestRound31MovedJobPointConsumedByOtherJob(t *testing.T) {
	adv32MovedSetup(t, 48*time.Hour, 30*time.Hour, 20*time.Hour)
	out, errOut, err := runCLI(t, "rpo", "--days", "2", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("missed=%d unproven=%d evidence=%s met=%v capped=%v times=%v note=%q", r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.MissedRunTimes, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "2", "--guest", "100", "--by-client", "--json")
	t.Logf("by-client: %s", out2)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s); every job1 run before the move has its archive on local inside its window, and job3 had its own archive", certain)
	}
}

// Same, window starting just before the move: one pre-move run, evidence full.
func TestRound31MovedJobPointConsumedEvidenceFull(t *testing.T) {
	adv32MovedSetup(t, 48*time.Hour, 47*time.Hour, 23*time.Hour+45*time.Minute)
	out, errOut, err := runCLI(t, "rpo", "--days", "2", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("missed=%d unproven=%d evidence=%s met=%v capped=%v points_unread=%v times=%v note=%q", r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.PointsUnread, r.MissedRunTimes, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "2", "--guest", "100", "--by-client", "--json")
	t.Logf("by-client: %s", out2)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) with evidence=%s", certain, r.Evidence)
	}
}

// VM 100's job1 writes to shared NFS "nas" (no prune-backups on the job or
// the storage). pve2's /etc/vzdump.conf sets prune-backups
// keep-daily=1,keep-weekly=4; pve1's sets nothing. The guest ran on pve2
// (where every run pruned by pve2's vzdump.conf) and was migrated to pve1
// two days ago. Every run made its archive; pve2's retention thinned them.
func TestRound31MigratedGuestOtherNodesVzdumpConfPrunes(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["storage"] = "nas"
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "nas", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas", "content": "backup"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	st = append(st, map[string]any{"storage": "nas", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	f.pveRoutes["/nodes/pve1/storage"] = st
	f.pveRoutes["/nodes/pve2/storage"] = st
	f.pveRoutes["/nodes"] = []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "online"}}
	f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{}
	f.pveRoutes["/nodes/pve2/vzdump/defaults"] = map[string]any{"prune-backups": "keep-daily=1,keep-weekly=4"}
	f.pveRoutes["/nodes/pve2/storage/local/content"] = []any{}
	f.pveRoutes["/nodes/pve2/storage/pbs1/content"] = []any{}
	f.pveRoutes["/nodes/pve2/tasks"] = []any{}
	// Runs (fixture): 7.5d, 6.5d, ..., 0.5d ago (on 2026-10-16: Oct 9..16).
	// Kept after pve2's last prune (the 2.5d run, Wed Oct 14): keep-daily=1
	// keeps Oct 14, keep-weekly=4 each ISO week's newest (Oct 14, Sun Oct 11,
	// Oct 4, Sep 27); then pve1's runs Oct 15 and 16 (pve1 prunes nothing).
	var nas []any
	for _, d := range []float64{19.5, 12.5, 5.5, 2.5, 1.5, 0.5} {
		nas = append(nas, vzdumpEntry("nas", 100, f.now.Add(-time.Duration(d*24*float64(time.Hour))+20*time.Minute).Unix()))
	}
	f.pveRoutes["/nodes/pve1/storage/nas/content"] = nas
	f.pveRoutes["/nodes/pve2/storage/nas/content"] = nas
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("missed=%d unproven=%d evidence=%s met=%v capped=%v points_unread=%v times=%v note=%q", r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.PointsUnread, r.MissedRunTimes, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--by-client", "--json")
	t.Logf("by-client (guest 100): %s", out2)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) %v with evidence=%s; every run made its archive and pve2's vzdump.conf retention (the node that ran them) pruned them", certain, r.MissedRunTimes, r.Evidence)
	}
}

// VM 105 (covered, no own backup) has one backup within an hour before its
// creation (either guest's), and the local storage could not be read.
func TestRound31EitherPlusUnreadNote(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 105, 84.5)
	addPointAt(f, 105, -85*time.Hour, "ok")
	delete(f.pveRoutes, "/nodes/pve1/storage/local/content")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("missed=%d unproven=%d evidence=%s unread=%v note=%q", r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.PointsUnread, r.Note)
	if strings.Contains(r.Note, "every scheduled run in the window is missed") || strings.Contains(r.Note, "no restore points retained") {
		t.Errorf("unread storage, yet note: %q", r.Note)
	}
}

// Unit receipt: job1 (hourly) moved from X to A; its run at h(10) wrote its
// archive to X at h(9.9). job3 (daily) writes X; its run at h(11) reached the
// guest late in its queue (h(8.5)). Both runs made a restore point. The
// matching hands job3's window the earliest X time (job1's archive), so
// job1's h(10) miss is PROVEN although its archive sits on X inside its window
// and job3 had its own.
func TestRound31UnitMovedPointConsumed(t *testing.T) {
	now := time.Now()
	h := func(x float64) int64 { return now.Add(-time.Duration(x * float64(time.Hour))).Unix() }
	var a []restorePoint
	a = append(a, restorePoint{Time: h(100), canonical: "pve:a@n:old"})
	var runs1 []int64
	for x := 10.0; x >= 0.5; x-- {
		runs1 = append(runs1, h(x))
		if x < 10 {
			a = append(a, restorePoint{Time: h(x) + 60, canonical: "pve:a@n:" + time.Unix(h(x), 0).String()})
		}
	}
	x := []restorePoint{{Time: h(100), canonical: "pve:x@n:old"}, {Time: h(9.9), canonical: "pve:x@n:job1"}, {Time: h(8.5), canonical: "pve:x@n:job3"}}
	all := append(slices.Clone(a), x...)
	r := evaluateJobsRPO(all, []rpoTarget{{jobRuns: [][]int64{runs1}, points: a}, {jobRuns: [][]int64{{h(11)}}, points: x}}, now.Add(-24*time.Hour), now)
	t.Logf("%+v", r)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN miss(es) %v evidence=%s met=%v; job1's archive for that run is on X inside its window and job3 had its own archive", certain, r.MissedRunTimes, r.Evidence, r.MetSchedule)
	}
}

// Codex round 43: window_start is the start rpo evaluated, the VM's creation
// when it was created inside the window.
func TestRPOWindowStartIsEvaluatedStart(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 40)
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	want := time.Unix(ago(f.now, 40), 0).UTC().Format(time.RFC3339)
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.WindowStart != want {
		t.Errorf("window_start %s, want the creation time %s", r.WindowStart, want)
	}
}
