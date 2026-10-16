// Adversarial round 34 rpo receipts: retention changed, schedule changed, guest added to a job, PBS storage aliases.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// adv34Daily serves schedule-analyze for any schedule as daily runs ending
// 12h ago (60 days back), and returns the newest run time.
func adv34Daily(t *testing.T, f *fixture) int64 {
	day := int64(86400)
	newest := f.now.Add(-12 * time.Hour).Unix()
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cluster/jobs/schedule-analyze") {
			from, _ := strconv.ParseInt(r.URL.Query().Get("starttime"), 10, 64)
			out := []map[string]any{}
			for ts := newest - 60*day; len(out) < 100; ts += day {
				if ts >= from {
					out = append(out, map[string]any{"timestamp": ts})
				}
			}
			writeData(w, out)
			return true
		}
		return false
	})
	return newest
}

// okTasks puts one successful vzdump task of VM 100 at every given run time
// into PVE's task history (what PVE keeps for each run), with the prune lines
// vzdump writes when retention removes older backups.
func okTasks(f *fixture, runs []int64, pruned bool) {
	var tasks []map[string]any
	for i, r := range runs {
		upid := fmt.Sprintf("UPID:pve1:%08X:00000001:%08X:vzdump::root@pam:", i+10, r)
		tasks = append(tasks, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 600, "user": "root@pam"})
		log := []map[string]any{
			{"n": 1, "t": "INFO: starting new backup job: vzdump 100 --mode snapshot --storage pbs1"},
			{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
			{"n": 3, "t": "INFO: Finished Backup of VM 100 (00:00:42)"},
		}
		if pruned {
			log = append(log, map[string]any{"n": 4, "t": "INFO: prune older backups with retention: keep-daily=7, keep-weekly=4"}, map[string]any{"n": 5, "t": "INFO: pruned 1 backup(s)"})
		}
		log = append(log, map[string]any{"n": 9, "t": "INFO: Backup job finished successfully"})
		f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = log
	}
	f.pveRoutes["/nodes/pve1/tasks"] = tasks
}

// GFS survivors: the newest 9 dailies and one per week before that.
func gfsPoints(f *fixture, newest int64) {
	day := int64(86400)
	for _, d := range []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 13, 20, 27, 34} {
		addPointAt(f, 100, time.Unix(newest-d*day+1200, 0).Sub(f.now), "ok")
	}
}

func adv34Run(t *testing.T, label string) rpoRow {
	out, errOut, err := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("%s: err=%v stderr=%q runs=%d missed=%d unproven=%d evidence=%s met=%v capped=%v note=%q", label, err, errOut, r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--by-client", "--json")
	var c []map[string]any
	_ = json.Unmarshal([]byte(out2), &c)
	t.Logf("%s by-client: missed_runs=%v missed_runs_unproven=%v guests_unproven=%v guests_partial=%v", label, c[0]["missed_runs"], c[0]["missed_runs_unproven"], c[0]["guests_unproven"], c[0]["guests_partial_evidence"])
	fo, _, _ := runCLI(t, "failures", "--type", "vzdump", "--since", "30d", "--json")
	fr := decodeJSON[failuresReport](t, fo)
	t.Logf("%s failures --type vzdump --since 30d: scanned=%d rows=%d logs_read=%d", label, fr.ScannedTasks, len(fr.Rows), fr.LogsRead)
	return r
}

// The round-33 class (retention that pruned inside the window but is not
// active now), its vzdump-side sites. job1 daily -> pbs1 pruned VM 100 with
// keep-daily=7,keep-weekly=4 until the MSP set keep-all=1 two days ago (legal
// hold, ransomware response): on the job itself, or on the storage.
func TestRound34RetentionChangedToKeepAll(t *testing.T) {
	for _, where := range []string{"job", "storage"} {
		t.Run(where, func(t *testing.T) {
			f := newFixture(t)
			f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
			f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
			newest := adv34Daily(t, f)
			if where == "job" {
				f.pveRoutes["/cluster/backup"].([]map[string]any)[0]["prune-backups"] = "keep-all=1"
			} else {
				f.pbsPrune = "keep-all=1"
			}
			gfsPoints(f, newest)
			var runs []int64
			for ts := newest - 29*86400; ts <= newest; ts += 86400 {
				runs = append(runs, ts)
			}
			okTasks(f, runs, true)
			r := adv34Run(t, "retention changed on the "+where)
			if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
				t.Errorf("FALSE RED: %d PROVEN missed runs (evidence=%s) although PVE's task history holds a successful vzdump task of VM 100 for every one of them, each logging a prune", c, r.Evidence)
			}
		})
	}
}

// The job's schedule was made more frequent inside the window: weekly
// (every 7th day) until 3 days ago, daily since. keep-all everywhere, every
// run ever made has its archive, and the task history shows exactly those
// runs, all OK.
func TestRound34ScheduleMadeMoreFrequent(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	newest := adv34Daily(t, f)
	day := int64(86400)
	var runs []int64
	for d := int64(56); d >= 3; d-- {
		if d%7 == 0 {
			runs = append(runs, newest-d*day)
		}
	}
	for d := int64(2); d >= 0; d-- {
		runs = append(runs, newest-d*day)
	}
	for _, r := range runs {
		addPointAt(f, 100, time.Unix(r+1200, 0).Sub(f.now), "ok")
	}
	okTasks(f, runs, false)
	o7, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	r7 := decodeJSON[rpoReport](t, o7).Rows[0]
	t.Logf("weekly->daily 3d ago, --days 7: runs=%d missed=%d unproven=%d evidence=%s met=%v capped=%v missed_run_times=%v", r7.ScheduledRuns, r7.MissedRuns, r7.MissedRunsUnproven, r7.Evidence, r7.MetSchedule, r7.WindowCapped, r7.MissedRunTimes)
	r := adv34Run(t, "weekly->daily 3d ago")
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed runs (evidence=%s, met=%v) for days the job was weekly: no run was scheduled and every run that was has its restore point", c, r.Evidence, r.MetSchedule)
	}
}

// The guest was moved from a weekly job into the daily job1 3 days ago
// (the weekly job then deleted, or the guest excluded from it).
func TestRound34GuestAddedToDailyJob(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	newest := adv34Daily(t, f)
	day := int64(86400)
	var runs []int64
	for d := int64(59); d >= 3; d-- {
		if d%7 == 3 { // the weekly job ran 4h later in the day
			runs = append(runs, newest-d*day+4*3600)
		}
	}
	for d := int64(2); d >= 0; d-- {
		runs = append(runs, newest-d*day)
	}
	for _, r := range runs {
		addPointAt(f, 100, time.Unix(r+1200, 0).Sub(f.now), "ok")
	}
	okTasks(f, runs, false)
	r := adv34Run(t, "moved weekly->daily job 3d ago")
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed runs (evidence=%s) from before the guest was in job1", c, r.Evidence)
	}
}

// Two PVE storage ids name ONE former PBS location (datastore "old", root
// namespace), the usual way to give two jobs their own retention:
// pbsold-a keeps everything, pbsold-z has prune-backups keep-last=1. job1
// wrote via pbsold-z until 3.4 d ago, then moved to pbs1/main (keep-all).
// vzdump pruned VM 100's group in datastore old by pbsold-z's policy after
// every run. formerStorage asks only the first storage id (sorted) that
// names the location.
func TestRound34FormerPBSLocationTwoStorageIDs(t *testing.T) {
	for _, names := range [][2]string{{"pbsold-a", "pbsold-z"}, {"pbsold-z", "pbsold-a"}} {
		keepAll, pruning := names[0], names[1]
		t.Run("pruning="+pruning, func(t *testing.T) {
			f := newFixture(t)
			f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
			f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			port, _ := strconv.Atoi(hp[1])
			f.storageCfg = append(f.storageCfg,
				map[string]any{"storage": keepAll, "type": "pbs", "server": hp[0], "port": port, "datastore": "old", "content": "backup"},
				map[string]any{"storage": pruning, "type": "pbs", "server": hp[0], "port": port, "datastore": "old", "content": "backup", "prune-backups": "keep-last=1"})
			st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
			for _, id := range names {
				st = append(st, map[string]any{"storage": id, "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
				f.pveRoutes["/nodes/pve1/storage/"+id+"/content"] = []any{}
			}
			f.pveRoutes["/nodes/pve1/storage"] = st
			pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "/vzdump/defaults") && r.URL.Query().Get("storage") == pruning {
					writeData(w, map[string]any{"prune-backups": "keep-last=1"})
					return true
				}
				return false
			})
			f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main", "backend-type": "filesystem", "mount-status": "nonremovable"}, {"store": "old", "backend-type": "filesystem", "mount-status": "nonremovable"}}
			f.pbsRoutes["/admin/datastore/old/namespace"] = []map[string]any{{"ns": ""}}
			addPointAt(f, 100, -40*24*time.Hour, "ok") // old manual test backup on pbs1/main
			addPointAt(f, 100, -time.Duration(3.4*24*float64(time.Hour)), "ok")
			for _, d := range []float64{2.5, 1.5, 0.5} {
				addPointAt(f, 100, -time.Duration(d*24*float64(time.Hour))+20*time.Minute, "ok")
			}
			ts := f.now.Add(-time.Duration(3.5*24*float64(time.Hour)) + 20*time.Minute).Unix() // what keep-last=1 kept
			f.pbsRoutes["/admin/datastore/old/snapshots"] = []any{map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ts, "size": 1 << 30, "files": []string{"index.json.blob"}, "owner": "root@pam"}}
			out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
			rep := decodeJSON[rpoReport](t, out)
			r := rep.Rows[0]
			t.Logf("err=%v stderr=%q runs=%d missed=%d unproven=%d evidence=%s met=%v times=%v note=%q", err, errOut, r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.MissedRunTimes, r.Note)
			if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
				t.Errorf("FALSE RED: %d PROVEN missed run(s) with evidence=%s; storage %s (keep-last=1) names the same PBS location and pruned them", certain, r.Evidence, pruning)
			}
		})
	}
}

// The same alias at the CURRENT destination: job1 -> pbs1 (keep-all); a
// second storage id pbs1-gfs names the same datastore and namespace with
// keep-daily=7,keep-weekly=4 (used by a job since deleted, or by manual
// "Backup now" runs, each of which prunes the group by that policy).
func TestRound34CurrentPBSLocationAliasRetention(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	port, _ := strconv.Atoi(hp[1])
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "pbs1-gfs", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "content": "backup", "prune-backups": "keep-daily=7,keep-weekly=4"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbs1-gfs", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	f.pveRoutes["/nodes/pve1/storage/pbs1-gfs/content"] = []any{}
	newest := adv34Daily(t, f)
	gfsPoints(f, newest)
	r := adv34Run(t, "alias pbs1-gfs at the current destination")
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed runs (evidence=%s); storage pbs1-gfs names the job's own PBS location with keep-daily=7,keep-weekly=4", c, r.Evidence)
	}
}

// Control: the same alias shape on NFS (two storage ids over one export) is
// caught, because the alias's listing shows the files under its own id.
