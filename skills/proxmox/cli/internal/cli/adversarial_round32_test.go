// Adversarial round 32 scratch receipts.

// Adversarial round 32 receipts: a former destination's retention, chained retargets, cost on a year of hourly runs, the rollup's no-point count.

package cli

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// adv32FormerTargetSetup: job1 (daily 21:00, VM 100) wrote NFS "nas" until
// moveAgo; the MSP then pointed it at pbs1 (keep-all, no PBS prune job) and
// clicked "Run now" (a pbs1 backup right after the move). nas carries its own
// storage-level retention (prune-backups in storage.cfg), so every job1 run
// that wrote nas pruned nas by that policy. pbs1 holds one old manual test
// backup from before the window (so nothing caps the window). Every run made
// its archive.
func adv32FormerTargetSetup(t *testing.T, nasPolicy string, moveAgoDays float64, keptNasDays []float64) *fixture {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "nas", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas", "content": "backup", "prune-backups": nasPolicy})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "nas", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	day := int64(86400)
	newest := f.now.Add(-12 * time.Hour).Unix()
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cluster/jobs/schedule-analyze"):
			from, _ := strconv.ParseInt(r.URL.Query().Get("starttime"), 10, 64)
			out := []map[string]any{}
			for ts := newest - 60*day; len(out) < 100; ts += day {
				if ts >= from {
					out = append(out, map[string]any{"timestamp": ts})
				}
			}
			writeData(w, out)
			return true
		case strings.HasSuffix(r.URL.Path, "/vzdump/defaults") && r.URL.Query().Get("storage") == "nas":
			// Like PVE: the defaults API overlays the storage's own retention.
			writeData(w, map[string]any{"prune-backups": nasPolicy})
			return true
		}
		return false
	})
	addPointAt(f, 100, -40*24*time.Hour, "ok") // old manual test backup on pbs1
	move := f.now.Add(-time.Duration(moveAgoDays * 24 * float64(time.Hour))).Unix()
	addPointAt(f, 100, time.Unix(move+300, 0).Sub(f.now), "ok") // "Run now" right after the move
	for r := newest - 60*day; r <= newest; r += day {
		if r >= move {
			addPointAt(f, 100, time.Unix(r+1200, 0).Sub(f.now), "ok")
		}
	}
	var nas []any
	for _, d := range keptNasDays { // what nas's own retention kept of job1's pre-move archives
		nas = append(nas, vzdumpEntry("nas", 100, newest-int64(d*float64(day))+1200))
	}
	f.pveRoutes["/nodes/pve1/storage/nas/content"] = nas
	return f
}

// keep-daily=7 on nas, moved 5 days ago, rpo --days 30: nas kept the newest
// 7 pre-move archives (runs 5..11 days before the newest run); the 18 older
// ones were made, then pruned by nas's retention.
func TestRound32FormerTargetRetentionProvenMisses(t *testing.T) {
	f := adv32FormerTargetSetup(t, "keep-daily=7", 5, []float64{5, 6, 7, 8, 9, 10, 11})
	out, errOut, err := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("runs=%d missed=%d unproven=%d evidence=%s met=%v capped=%v unread=%v note=%q", r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.PointsUnread, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--by-client", "--json")
	t.Logf("by-client: %s", out2)
	_ = f
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) (evidence=%s); each run made its archive on nas, whose own storage retention (%s, readable in storage.cfg and vzdump/defaults?storage=nas) pruned it", certain, r.Evidence, "keep-daily=7")
	}
}

// keep-last=1 on nas, moved 3.4 days ago, rpo --days 7: evidence full.
func TestRound32FormerTargetRetentionProvenMissesEvidenceFull(t *testing.T) {
	adv32FormerTargetSetup(t, "keep-last=1", 3.4, []float64{3})
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("runs=%d missed=%d unproven=%d evidence=%s met=%v capped=%v unread=%v times=%v note=%q", r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.PointsUnread, r.MissedRunTimes, r.Note)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) with evidence=%s; nas's own retention (keep-last=1) pruned them", certain, r.Evidence)
	}
}

// Unit: two jobs both moved storages at once. job1 (target A now) wrote X
// before; job3 (target X now) wrote Y before. job1's pre-move run h(50) left
// its archive on X at h(49.9); job3's pre-move run h(51) left its archive on
// Y at h(50.5). Both runs made a restore point. The matching gives job3's
// window the X point (job1's archive); job1's augmenting path through it
// needs job3's window to take ITS elsewhere point (Y), which only w's own
// adjacency is extended with, so job1's run is a PROVEN miss.
func TestRound32UnitChainNeedsSecondElsewhere(t *testing.T) {
	now := time.Now()
	h := func(x float64) int64 { return now.Add(-time.Duration(x * float64(time.Hour))).Unix() }
	a := []restorePoint{{Time: h(100), canonical: "pve:a@n:old"}, {Time: h(25.9), canonical: "pve:a@n:2"}, {Time: h(7.9), canonical: "pve:a@n:3"}}
	x := []restorePoint{{Time: h(100.5), canonical: "pve:x@n:old"}, {Time: h(49.9), canonical: "pve:x@n:job1pre"}, {Time: h(26.5), canonical: "pve:x@n:2"}, {Time: h(8.5), canonical: "pve:x@n:3"}}
	y := []restorePoint{{Time: h(50.5), canonical: "pve:y@n:job3pre"}}
	all := append(append(append([]restorePoint{}, a...), x...), y...)
	r := evaluateJobsRPO(all, []rpoTarget{
		{jobRuns: [][]int64{{h(50), h(26), h(8)}}, points: a},
		{jobRuns: [][]int64{{h(51), h(27), h(9)}}, points: x},
	}, now.Add(-72*time.Hour), now)
	t.Logf("runs=%d missed=%d unproven=%d evidence=%s met=%v times=%v", r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.MissedRunTimes)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN miss(es) %v evidence=%s; each run has its own archive (job1's on X, job3's on Y) inside its window", certain, r.MissedRunTimes, r.Evidence)
	}
}

// Cost: an hourly job over 366 days whose target holds nothing in the window
// (the job moved storages yesterday; the old storage keeps every archive).
func TestRound32UnitCostHourlyYearAllElsewhere(t *testing.T) {
	now := time.Now()
	var runs []int64
	var tgt, old []restorePoint
	tgt = append(tgt, restorePoint{Time: now.Add(-400 * 24 * time.Hour).Unix(), canonical: "pve:a@n:old"})
	for i := 366 * 24; i >= 1; i-- {
		r := now.Add(-time.Duration(i) * time.Hour).Unix()
		runs = append(runs, r)
		old = append(old, restorePoint{Time: r + 60, canonical: "pve:x@n:" + strconv.Itoa(i)})
	}
	all := append(append([]restorePoint{}, tgt...), old...)
	t0 := time.Now()
	r := evaluateJobsRPO(all, []rpoTarget{{jobRuns: [][]int64{runs}, points: tgt}}, now.Add(-366*24*time.Hour), now)
	t.Logf("elapsed=%v runs=%d missed=%d unproven=%d", time.Since(t0), r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven)
	if el := time.Since(t0); el > 20*time.Second {
		t.Errorf("rpo took %v for one guest", el)
	}
}

// Cost: a daily job and an hourly job on one target, keep-all, 366 days.
func TestRound32UnitCostDailyPlusHourlyOneTarget(t *testing.T) {
	now := time.Now()
	var hourly, daily []int64
	var pts []restorePoint
	for i := 366 * 24; i >= 1; i-- {
		r := now.Add(-time.Duration(i) * time.Hour).Unix()
		hourly = append(hourly, r)
		pts = append(pts, restorePoint{Time: r + 60, canonical: "pve:a@n:" + strconv.Itoa(i)})
		if i%24 == 0 {
			daily = append(daily, r+30)
			pts = append(pts, restorePoint{Time: r + 1800, canonical: "pve:a@n:d" + strconv.Itoa(i)})
		}
	}
	t0 := time.Now()
	r := evaluateJobsRPO(pts, []rpoTarget{{jobRuns: [][]int64{hourly, daily}, points: pts}}, now.Add(-366*24*time.Hour), now)
	t.Logf("elapsed=%v runs=%d missed=%d unproven=%d", time.Since(t0), r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven)
	if el := time.Since(t0); el > 20*time.Second {
		t.Errorf("rpo took %v for one guest", el)
	}
}

// The same class on PBS: job1 moved from PBS storage pbsold (datastore "old",
// pruned by a PBS prune job keep-last=1) to pbs1 (datastore "main", no prune
// job, keep-all). rpo --days 7.
func TestRound32FormerPBSDatastorePruneJob(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	port, _ := strconv.Atoi(hp[1])
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "pbsold", "type": "pbs", "server": hp[0], "port": port, "datastore": "old", "content": "backup"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbsold", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	f.pveRoutes["/nodes/pve1/storage/pbsold/content"] = []any{}
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main", "backend-type": "filesystem", "mount-status": "nonremovable"}, {"store": "old", "backend-type": "filesystem", "mount-status": "nonremovable"}}
	f.pbsRoutes["/admin/datastore/old/namespace"] = []map[string]any{{"ns": ""}}
	f.pbsRoutes["/admin/prune"] = []map[string]any{{"id": "p-old", "store": "old", "ns": "", "schedule": "daily", "keep-last": 1}}
	// Fixture runs: 7.5d, 6.5d, ..., 0.5d ago. Moved 3.4d ago + Run now onto pbs1.
	addPointAt(f, 100, -40*24*time.Hour, "ok") // old manual test backup on pbs1/main
	addPointAt(f, 100, -time.Duration(3.4*24*float64(time.Hour)), "ok")
	for _, d := range []float64{2.5, 1.5, 0.5} {
		addPointAt(f, 100, -time.Duration(d*24*float64(time.Hour))+20*time.Minute, "ok")
	}
	ts := f.now.Add(-time.Duration(3.5*24*float64(time.Hour)) + 20*time.Minute).Unix() // the only one the prune job kept
	f.pbsRoutes["/admin/datastore/old/snapshots"] = []any{map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ts, "size": 1 << 30, "files": []string{"index.json.blob"}, "owner": "root@pam"}}
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	rep := decodeJSON[rpoReport](t, out)
	r := rep.Rows[0]
	t.Logf("runs=%d missed=%d unproven=%d evidence=%s met=%v rp=%d times=%v note=%q fails=%v", r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.RestorePoints, r.MissedRunTimes, r.Note, rep.FetchFailures)
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) with evidence=%s; the PBS prune job on datastore old (the job's former target, holding VM 100's archives) pruned them", certain, r.Evidence)
	}
}

// VM 101 has a restore point 50h old (fixture). rpo --days 1 --by-client.
func TestRound32WithoutRestorePointsCountsOlderPoints(t *testing.T) {
	newFixture(t)
	out, _, _ := runCLI(t, "rpo", "--days", "1", "--guest", "101", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("row: rp=%d missed=%d unproven=%d evidence=%s note=%q", r.RestorePoints, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "1", "--guest", "101", "--by-client", "--json")
	for _, c := range decodeJSON[[]rpoClientRow](t, out2) {
		if c.GuestsWithNoData != 0 {
			t.Errorf("VM 101 has a 50h-old restore point, yet guests_without_restore_points=%d", c.GuestsWithNoData)
		}
	}
}

// Codex round 44: a pin written with spaces is the same pin at TLS time, so
// a push to this PBS whose remote carries the colon form is not offsite.
func TestSelfPushPinFormatsAgree(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", strings.ReplaceAll(f.pbsFingerprint(), ":", " "))
	f.pbsRoutes["/config/remote/dr"] = map[string]any{"name": "dr", "host": "pbs-alias.example.net", "port": 8007, "fingerprint": f.pbsFingerprint()}
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatalf("a space-separated pin must still connect: %v", err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.OffsiteLastSuccessAt != "" {
			t.Errorf("VM %d credited offsite by a push to this PBS", r.VMID)
		}
	}
}

// Codex round 44: failures --csv has no log_cap_hit column: it warns on stderr.
func TestFailuresCSVWarnsOnLogCap(t *testing.T) {
	newFixture(t)
	_, errOut, _ := runCLI(t, "failures", "--type", "vzdump", "--max-logs", "0", "--csv")
	if !strings.Contains(errOut, "--max-logs 0") {
		t.Errorf("no log-cap warning on stderr:\n%s", errOut)
	}
}
