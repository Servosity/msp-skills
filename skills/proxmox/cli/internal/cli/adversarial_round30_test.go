// Adversarial round 30 + Codex round 42 receipts: cross-storage retention, a second job failing nightly, exact matching, unread-aware notes, ambiguous host.

package cli

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// runsServer answers schedule-analyze per schedule string from a newest run
// time and a period (like PVE: events at or after starttime, at most 100).
func runsServer(t *testing.T, f *fixture, plans map[string][2]int64) {
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/cluster/jobs/schedule-analyze") {
			return false
		}
		p, ok := plans[r.URL.Query().Get("schedule")]
		if !ok {
			return false
		}
		newest, period := p[0], p[1]
		from, _ := strconv.ParseInt(r.URL.Query().Get("starttime"), 10, 64)
		out := []map[string]any{}
		first := newest - 60*period
		for ts := first; len(out) < 100; ts += period {
			if ts >= from {
				out = append(out, map[string]any{"timestamp": ts})
			}
		}
		writeData(w, out)
		return true
	})
}

// Two NFS storage ids over ONE export (the round-29 alias setup):
// job1 -> nas-daily, daily 21:00, keeps everything; job3 -> nas-weekly,
// weekly (Sunday 22:00), prune-backups keep-weekly=4. vzdump prunes VM 100's
// archives in the export after each job3 run, so job1's archives of that
// week (Mon..Sun 21:00) are deleted and only the Sunday 22:00 one is kept.
// The archive of job1's run 7 days ago was written, then pruned by job3.
func TestRound30AliasCrossRetentionProvenMiss(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["storage"] = "nas-daily"
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "sun 22:00", "storage": "nas-weekly", "vmid": "100", "prune-backups": "keep-weekly=4"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	f.storageCfg = append(f.storageCfg,
		map[string]any{"storage": "nas-daily", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas-daily", "content": "backup"},
		map[string]any{"storage": "nas-weekly", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas-weekly", "content": "backup"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st,
		map[string]any{"storage": "nas-daily", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900},
		map[string]any{"storage": "nas-weekly", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	day := int64(86400)
	r0 := f.now.Add(-2 * time.Hour).Unix() // job1's newest run (in grace, its archive landed)
	sun := r0 - 6*day + 3600               // job3's newest run, Sunday 22:00
	runsServer(t, f, map[string][2]int64{"21:00": {r0, day}, "sun 22:00": {sun, 7 * day}})
	// Retained after job3's prune at `sun`: the newest archive of each of the
	// last 4 weeks (job3's Sundays), then job1's Mon..Sat archives since.
	var kept []int64
	for k := int64(3); k >= 0; k-- {
		kept = append(kept, sun-k*7*day+60)
	}
	for k := int64(5); k >= 0; k-- {
		kept = append(kept, r0-k*day+60)
	}
	var daily, weekly []any
	for _, ts := range kept {
		daily = append(daily, vzdumpEntry("nas-daily", 100, ts))
		weekly = append(weekly, vzdumpEntry("nas-weekly", 100, ts))
	}
	f.pveRoutes["/nodes/pve1/storage/nas-daily/content"] = daily
	f.pveRoutes["/nodes/pve1/storage/nas-weekly/content"] = weekly
	out, errOut, err := runCLI(t, "rpo", "--days", "8", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if certain := r.MissedRuns - r.MissedRunsUnproven; certain > 0 {
		t.Errorf("FALSE RED: %d PROVEN missed run(s) %v (unproven %d, evidence %s, points_unread %v): job1's archive was written and then deleted by job3's keep-weekly=4 prune over the same export, the case the docs say is unproven", certain, r.MissedRunTimes, r.MissedRunsUnproven, r.Evidence, r.PointsUnread)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "8", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// job1 -> pbs1 succeeds every night; job3 -> local (node-local dir, no
// retention anywhere) has failed every night: local is read and empty.
func TestRound30SecondJobFailsEveryNightIsOnlyUnproven(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	addJob3(f, "local")
	everyRun(f, 100)
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRunsUnproven > 0 {
		t.Errorf("WEAK: job3 -> local never wrote (local read, empty, no retention), yet %d of %d misses are unproven (evidence %s): each pbs1 point already paid job1's run at that time", r.MissedRunsUnproven, r.MissedRuns, r.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// PROXMOX_HOST is ambiguous and kept raw in TemplateVars["host"]; the
// resolved base URL is printed unredacted by CheckServiceOrigins' error paths.
func TestRound30AmbiguousHostRawResolvedPrinted(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "localhost:8007/Pa5wo@pve.example")
	for _, args := range [][]string{{"doctor", "--json"}, {"posture", "--json"}} {
		out, errOut, err := runCLI(t, args...)
		all := out + errOut
		if i := strings.Index(all, "Pa5wo"); i >= 0 {
			t.Errorf("%v: SECRET PRINTED (err=%v): ...%s...", args, err, all[max(0, i-120):min(len(all), i+80)])
		}
	}
}

// Codex round 42: two destinations, runs 10h (A) and 11h (B) ago, A holds
// points 9h and 8h ago, B one 9h ago. Paying A with 8h and B with 9h meets
// both under the one-time-one-run rule; a greedy pass could miss B.
func TestRPOExactMatchingAcrossDestinations(t *testing.T) {
	now := time.Now()
	h := func(x float64) int64 { return now.Add(-time.Duration(x * float64(time.Hour))).Unix() }
	a := []restorePoint{{Time: h(100), canonical: "pve:a@n:1"}, {Time: h(9), canonical: "pve:a@n:2"}, {Time: h(8), canonical: "pve:a@n:3"}}
	b := []restorePoint{{Time: h(100), canonical: "pve:b@n:1"}, {Time: h(9), canonical: "pve:b@n:2"}}
	r := evaluateJobsRPO(append(slices.Clone(a), b...), []rpoTarget{{jobRuns: [][]int64{{h(10)}}, points: a}, {jobRuns: [][]int64{{h(11)}}, points: b}}, now.Add(-24*time.Hour), now)
	if r.MissedRuns != 0 || r.ScheduledRuns != 2 {
		t.Errorf("want both runs paid: %+v", r)
	}
}

// Codex round 42: with storage unread, rpo never says "no restore points
// retained" or "every scheduled run is missed".
func TestRPOUnreadNeverClaimsNone(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1}}
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--json")
	for _, r := range decodeJSON[rpoReport](t, out).Rows {
		if strings.Contains(r.Note, "no restore points retained") || strings.Contains(r.Note, "every scheduled run in the window is missed") {
			t.Errorf("VM %d (evidence %s): %q", r.VMID, r.Evidence, r.Note)
		}
	}
}
