// Adversarial round 33 scratch receipts (not part of the repo).

// Adversarial round 33 receipts: disabled PBS prune jobs, a stray manual backup, failures log-cap warnings.

package cli

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// adv33GFS: job1 (daily, VM 100) -> pbs1 (datastore main), keep-all on the
// PVE side. A PBS prune job on main (keep-daily=7, keep-weekly=4) pruned
// VM 100's group every day until the MSP disabled it 2 days ago (legal hold,
// ransomware response, migration). What is left is exactly what that policy
// keeps: the newest 9 dailies and one per week before that. Every run made
// its archive.
func adv33GFS(t *testing.T, disabled bool) rpoRow {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
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
	pj := map[string]any{"id": "p1", "store": "main", "ns": "", "schedule": "daily", "keep-daily": 7, "keep-weekly": 4,
		// PBS reports when it last ran: 2 days ago, inside the window.
		"last-run-state": "OK", "last-run-endtime": f.now.Add(-48 * time.Hour).Unix(), "last-run-upid": "UPID:pbs:prunejob"}
	if disabled {
		pj["disable"] = true
	}
	f.pbsRoutes["/admin/prune"] = []map[string]any{pj}
	for _, d := range []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 13, 20, 27, 34} {
		addPointAt(f, 100, time.Unix(newest-d*day+1200, 0).Sub(f.now), "ok")
	}
	out, errOut, err := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("disabled=%v runs=%d missed=%d unproven=%d evidence=%s met=%v capped=%v note=%q", disabled, r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.WindowCapped, r.Note)
	out2, _, _ := runCLI(t, "rpo", "--days", "30", "--guest", "100", "--by-client", "--json")
	t.Logf("by-client: %s", out2)
	return r
}

// Control: the same prune job enabled leaves every miss unproven.
func TestRound33EnabledPBSPruneJobUnproven(t *testing.T) {
	r := adv33GFS(t, false)
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("control: %d proven misses with an enabled prune job", c)
	}
}

// The disabled one: the CLI read it, saw its keep-* policy, and skipped it.
func TestRound33DisabledPBSPruneJobProvenMisses(t *testing.T) {
	r := adv33GFS(t, true)
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed runs, evidence=%s; a PBS prune job (keep-daily=7,keep-weekly=4) on the datastore, disabled now, pruned them (a disabled vzdump job is treated as may-have-run; a disabled prune job is not)", c, r.Evidence)
	}
}

// Rule 1 over-triggers: keep-all everywhere, job1 -> pbs1 failed 3 nights
// (real misses); the only other backup of VM 100 is a one-off manual
// "Backup now" to local 60 days ago (before an upgrade). No job ever wrote
// local, and local keeps everything.
func TestRound33StrayManualBackupDemotesRealMisses(t *testing.T) {
	var got [2]rpoRow
	for i, stray := range []bool{false, true} {
		f := newFixture(t)
		f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
		f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
		addPointAt(f, 100, -40*24*time.Hour, "ok") // old point on pbs1: no cap
		// fixture runs at 7.5d..0.5d ago; runs at 4.5, 3.5, 2.5 failed
		for _, d := range []float64{7.5, 6.5, 5.5, 1.5, 0.5} {
			addPointAt(f, 100, -time.Duration(d*24*float64(time.Hour))+20*time.Minute, "ok")
		}
		if stray {
			f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{vzdumpEntry("local", 100, f.now.Add(-60*24*time.Hour).Unix())}
		}
		out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
		got[i] = decodeJSON[rpoReport](t, out).Rows[0]
	}
	// A manual backup elsewhere, outside every run's window, changes nothing.
	if got[0].MissedRuns != 3 || got[1].MissedRuns != got[0].MissedRuns || got[1].MissedRunsUnproven != got[0].MissedRunsUnproven || got[1].Evidence != got[0].Evidence {
		t.Errorf("without %+v\nwith a stray backup %+v", got[0], got[1])
	}
}

// failures human table: rows shown, cap hit, no word that logs went unread.
func TestRound33FailuresHumanTableCapSilent(t *testing.T) {
	newFixture(t)
	out, errOut, _ := runCLI(t, "failures", "--type", "vzdump", "--since", "48h", "--max-logs", "1", "--human-friendly")
	t.Logf("stdout:\n%s\nstderr:\n%s", out, errOut)
	j, _, _ := runCLI(t, "failures", "--type", "vzdump", "--since", "48h", "--max-logs", "1", "--json")
	t.Logf("json: %s", j)
	if !strings.Contains(out+errOut, "max-logs") {
		t.Errorf("human output lists failures but never says only some logs were read")
	}
}

func TestRound33FailuresPlainQuietCap(t *testing.T) {
	newFixture(t)
	for _, fl := range []string{"--plain", "--quiet", "--compact"} {
		out, errOut, _ := runCLI(t, "failures", "--type", "vzdump", "--since", "48h", "--max-logs", "1", fl)
		if out != "" && !strings.Contains(errOut, "--max-logs 1") {
			t.Errorf("%s: rows without the log-cap warning:\n%s\nstderr:\n%s", fl, out, errOut)
		}
	}
}

// Cost: the wide graph's worst shape. Jobs B (target Y) and C (target Z)
// hourly, offset 30 min, every run paid; each B window holds the next Z
// point and each C window the next Y point, so the wide graph is one long
// alternating chain. Job A (target X) hourly at the same times as B, every
// run missed: each elsewhereCouldPay walks the chain to its end.
func TestRound33UnitCostChainWorstCase(t *testing.T) {
	now := time.Now()
	n := 5000
	var a, b, c []int64
	var y, z []restorePoint
	tgtX := []restorePoint{{Time: now.Add(-400 * 24 * time.Hour).Unix(), canonical: "pve:x@n:old"}}
	for i := n; i >= 1; i-- {
		r := now.Add(-time.Duration(i) * time.Hour).Unix()
		a = append(a, r)
		b = append(b, r)
		c = append(c, r+1800)
		y = append(y, restorePoint{Time: r + 60, canonical: "pve:y@n:" + strconv.Itoa(i)})
		z = append(z, restorePoint{Time: r + 1860, canonical: "pve:z@n:" + strconv.Itoa(i)})
	}
	y = append(y, restorePoint{Time: now.Add(-500 * 24 * time.Hour).Unix(), canonical: "pve:y@n:old"})
	z = append(z, restorePoint{Time: now.Add(-500 * 24 * time.Hour).Unix(), canonical: "pve:z@n:old"})
	all := append(append(append([]restorePoint{}, tgtX...), y...), z...)
	t0 := time.Now()
	r := evaluateJobsRPO(all, []rpoTarget{
		{jobRuns: [][]int64{a}, points: tgtX},
		{jobRuns: [][]int64{b}, points: y},
		{jobRuns: [][]int64{c}, points: z},
	}, now.Add(-366*24*time.Hour), now)
	t.Logf("elapsed=%v runs=%d missed=%d unproven=%d", time.Since(t0), r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven)
	if el := time.Since(t0); el > 20*time.Second {
		t.Errorf("rpo took %v for one guest", el)
	}
}
