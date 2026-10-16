// Adversarial round 29 scratch receipts (reviewer: fresh context).

// Adversarial round 29 receipts: one archive under two names, retargets, in-grace windows, ambiguous endpoints.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"proxmox-pp-cli/internal/client"
	"strconv"
	"strings"
	"testing"
	"time"
)

func vzdumpEntry(storage string, vmid int, ts int64) map[string]any {
	return map[string]any{"volid": storage + ":backup/vzdump-qemu-" + strconv.Itoa(vmid) + "-" + time.Unix(ts, 0).UTC().Format("2006_01_02-15_04_05") + ".vma.zst", "vmid": vmid, "ctime": ts, "size": 1 << 30, "subtype": "qemu", "format": "vma.zst"}
}

// Two NFS storage ids over one export (a common way to give two jobs their
// own retention): job1 -> nas-daily, job3 -> nas-weekly. Only job1 ever
// writes; both listings show the same files.
func TestRound29NFSAliasOneExport(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["storage"] = "nas-daily"
	addJob3(f, "nas-weekly")
	f.storageCfg = append(f.storageCfg,
		map[string]any{"storage": "nas-daily", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas-daily", "content": "backup"},
		map[string]any{"storage": "nas-weekly", "type": "nfs", "server": "10.0.0.9", "export": "/backups", "path": "/mnt/pve/nas-weekly", "content": "backup"})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st,
		map[string]any{"storage": "nas-daily", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900},
		map[string]any{"storage": "nas-weekly", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	var daily, weekly []any
	for h := 179.5; h > 0; h -= 24 {
		ts := f.now.Add(-time.Duration(h * float64(time.Hour))).Unix()
		daily = append(daily, vzdumpEntry("nas-daily", 100, ts))
		weekly = append(weekly, vzdumpEntry("nas-weekly", 100, ts)) // the SAME file, listed under the other id
	}
	f.pveRoutes["/nodes/pve1/storage/nas-daily/content"] = daily
	f.pveRoutes["/nodes/pve1/storage/nas-weekly/content"] = weekly
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MetSchedule {
		t.Errorf("FALSE GREEN: job3 -> nas-weekly never wrote a backup (7 files, all job1's, 14 runs), yet met_schedule=true scheduled=%d missed=%d evidence=%s", r.ScheduledRuns, r.MissedRuns, r.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// Two PBS storages naming one datastore on one server, spelled two ways
// (IP and "localhost"); PROXMOX_PBS_HOST matches the first.
func TestRound29PBSAliasTwoSpellings(t *testing.T) {
	f := newFixture(t)
	hp := f.pbs.URL[len("https://"):]
	port := hp[lastColon(hp)+1:]
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "pbs1b", "type": "pbs", "server": "localhost", "port": port, "datastore": "main", "namespace": ""})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbs1b", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	addJob3(f, "pbs1b")
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	everyRun(f, 100) // one point per day: job1's only
	f.pveRoutes["/nodes/pve1/storage/pbs1b/content"] = f.pveRoutes["/nodes/pve1/storage/pbs1/content"]
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MetSchedule {
		t.Errorf("FALSE GREEN: job1 (pbs1) and job3 (pbs1b) both write vm/100 to main at 21:00, one point per run, yet met_schedule=true missed=%d evidence=%s restore_points=%d", r.MissedRuns, r.Evidence, r.RestorePoints)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// job3 was retargeted from pbs1 (still job1's target) to local three days
// ago. Before that both jobs wrote to pbs1 (two points a night); since then
// one point a night on each. local holds one old manual backup (no cap).
func TestRound29RetargetFromCoveringDestination(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	addJob3(f, "local")
	local := []any{vzdumpEntry("local", 100, f.now.Add(-200*time.Hour).Unix())}
	for h := 179.5; h > 0; h -= 24 {
		at := -time.Duration(h * float64(time.Hour))
		addPointAt(f, 100, at, "ok") // job1's
		if h > 72 {
			addPointAt(f, 100, at+10*time.Minute, "ok") // job3's, on its former target pbs1
		} else {
			local = append(local, vzdumpEntry("local", 100, f.now.Add(at+10*time.Minute).Unix()))
		}
	}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = local
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven < r.MissedRuns && r.Evidence == "full" {
		t.Errorf("FALSE RED: %d proven missed runs (unproven %d) with evidence full; each of job3's runs has its point (on pbs1 before the retarget, an unpaid extra point in every one of those windows)", r.MissedRuns, r.MissedRunsUnproven)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// job1's newest run started 5h ago (in grace) and is waiting on the vzdump
// lock; job3 (an hour and a half earlier, out of grace) reached the guest
// late, after job1's start. Every judged run made its point.
func TestRound29InGraceRunStealsJudgedRunsPoint(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "19:30", "storage": "pbs1", "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	j1Last := f.now.Add(-5 * time.Hour)
	j3Last := f.now.Add(-6*time.Hour - 30*time.Minute)
	runsFrom := func(last time.Time, from int64) []map[string]any {
		out := []map[string]any{}
		first := last.Add(-8 * 24 * time.Hour)
		for ts := first.Unix(); len(out) < 100; ts += 86400 {
			if ts >= from {
				out = append(out, map[string]any{"timestamp": ts})
			}
		}
		return out
	}
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cluster/jobs/schedule-analyze") {
			from, _ := strconv.ParseInt(r.URL.Query().Get("starttime"), 10, 64)
			last := j1Last
			if r.URL.Query().Get("schedule") == "19:30" {
				last = j3Last
			}
			writeData(w, runsFrom(last, from))
			return true
		}
		return false
	})
	for d := 1; d <= 8; d++ {
		j3 := j3Last.Add(-time.Duration(d) * 24 * time.Hour)
		j1 := j1Last.Add(-time.Duration(d) * 24 * time.Hour)
		addPointAt(f, 100, j3.Add(20*time.Minute).Sub(f.now), "ok") // job3's, before job1 starts
		addPointAt(f, 100, j1.Add(20*time.Minute).Sub(f.now), "ok") // job1's
	}
	// Today: job3 reached the guest 2h in (after job1's start); job1 is waiting.
	addPointAt(f, 100, j3Last.Add(2*time.Hour).Sub(f.now), "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven == 0 && r.Evidence == "full" {
		t.Errorf("FALSE RED: %d proven misses (%v) with evidence full; every judged run made its point, the in-grace run took it", r.MissedRuns, r.MissedRunTimes)
	}
}

// PROXMOX_PBS_HOST names client B's PBS, spelled with an '@' after a '/';
// config.toml still holds a pbs_host (client A's, saved by an older version).
func TestRound29AmbiguousPBSHostFallsBackToSavedHost(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(t.TempDir(), "config.toml")
	hostA := strings.TrimPrefix(f.pbs.URL, "https://")
	if err := os.WriteFile(p, []byte("[template_vars]\npbs_host = \""+hostA+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOX_CONFIG", p)
	t.Setenv("PROXMOX_PBS_HOST", "https://backup:Pa5/5wo@pbs-clientb.example:8007")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "clientb@pbs!cli:22222222-2222-2222-2222-222222222222")
	before := f.pbsHits.Load()
	out, errOut, err := runCLI(t, "verify-health", "--json")
	t.Logf("err=%v\nstderr=%s\nout=%.600s", err, errOut, out)
	if n := f.pbsHits.Load() - before; n > 0 {
		t.Errorf("CREDENTIAL MISROUTE: PROXMOX_PBS_HOST names pbs-clientb.example (ambiguous spelling), yet %d PBS requests carrying client B's PROXMOX_PBS_API_TOKEN reached the config.toml pbs_host %s", n, hostA)
	}
}

// The ambiguous PVE endpoint is refused, but is it printed?
func TestRound29AmbiguousBaseURLPrinted(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "https://root:Pa5/5wo#rd@pve.example:8006/api2/json")
	for _, args := range [][]string{{"doctor", "--json"}, {"doctor"}, {"auth", "status", "--json"}, {"posture", "--json"}, {"clusters", "list", "--json"}} {
		out, errOut, err := runCLI(t, args...)
		if strings.Contains(out+errOut, "wo#rd") || strings.Contains(out+errOut, "Pa5") {
			t.Errorf("%v: SECRET PRINTED (err=%v):\nout=%s\nstderr=%s", args, err, out, errOut)
		}
	}
}

// Guest 100 migrated from pve1 to pve2 three days ago; job3 has no storage
// and writes to each node's default (vzdump.conf storage: local, node-local).
func TestRound29MigratedGuestNodeLocalDefault(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "21:00", "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	everyRun(f, 100)
	guests := f.pveRoutes["/cluster/resources"].([]map[string]any)
	guests[0]["node"] = "pve2"
	f.pveRoutes["/nodes"] = []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "online"}}
	f.pveRoutes["/nodes/pve2/storage"] = f.pveRoutes["/nodes/pve1/storage"]
	f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{"storage": "local"}
	f.pveRoutes["/nodes/pve2/vzdump/defaults"] = map[string]any{"storage": "local"}
	f.pveRoutes["/nodes/pve2/storage/local/prunebackups"] = []any{}
	var l1, l2 []any
	for h := 179.5; h > 0; h -= 24 {
		ts := f.now.Add(-time.Duration(h*float64(time.Hour)) + 20*time.Minute).Unix()
		if h > 72 {
			l1 = append(l1, vzdumpEntry("local", 100, ts))
		} else {
			l2 = append(l2, vzdumpEntry("local", 100, ts))
		}
	}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = l1
	f.pveRoutes["/nodes/pve2/storage/local/content"] = l2
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven < r.MissedRuns && r.Evidence == "full" {
		t.Errorf("FALSE RED: %d proven misses %v", r.MissedRuns, r.MissedRunTimes)
	}
	if r.MetSchedule && r.ScheduledRuns != 14 {
		t.Errorf("met with %d runs", r.ScheduledRuns)
	}
	// Same, but job3 failed on pve2 every night since the migration.
	f.pveRoutes["/nodes/pve2/storage/local/content"] = []any{}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	r = decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("after-migration failures: met=%v missed=%d unproven=%d evidence=%s note=%s", r.MetSchedule, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.Note)
	if r.MetSchedule {
		t.Errorf("FALSE GREEN after migration")
	}
}

// pbs1's server field spells the PBS as "localhost"; PROXMOX_PBS_HOST uses
// 127.0.0.1. The PVE listing of pbs1 is hidden (audit-only PVE token).
func TestRound29PBSSpellingMismatchAuditOnlyPVE(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	for h := 179.5; h > 30; h -= 24 {
		addPointAt(f, 100, -time.Duration(h*float64(time.Hour)), "ok")
	} // the last run (11.5h ago) has a point only... nowhere: a real miss
	hp := strings.TrimPrefix(f.pbs.URL, "https://")
	port, _ := strconv.Atoi(hp[lastColon(hp)+1:])
	f.pveRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1}}
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": "localhost", "port": port, "datastore": "main", "namespace": "", "prune-backups": "keep-all=1"}})
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/storage/pbs1/content") {
			writeData(w, []any{}) // audit-only: PVE filters the backup volumes out
			return true
		}
		return false
	})
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven < r.MissedRuns && r.Evidence == "full" {
		t.Errorf("FALSE RED: proven misses with full evidence though pbs1 was not read: %d %v", r.MissedRuns, r.MissedRunTimes)
	}
	if r.MetSchedule {
		t.Errorf("FALSE GREEN")
	}
}

// Receipt with the Authorization header captured at the saved host.
func TestRound29AmbiguousPBSHostTokenCaptured(t *testing.T) {
	f := newFixture(t)
	var got []string
	srv := httptestNewTLS(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		f.pbs.Config.Handler.ServeHTTP(w, r)
	})
	t.Cleanup(srv.Close)
	p := filepath.Join(t.TempDir(), "config.toml")
	hostA := strings.TrimPrefix(srv.URL, "https://")
	if err := os.WriteFile(p, []byte("[template_vars]\npbs_host = \""+hostA+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOX_CONFIG", p)
	// Stand-in for a CA-trusted certificate at the saved host (no per-client pin mismatch).
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", certFP(srv))
	t.Setenv("PROXMOX_PBS_HOST", "https://backup:Pa5/5wo@pbs-clientb.example:8007")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "clientb@pbs!cli:22222222-2222-2222-2222-222222222222")
	_, errOut, err := runCLI(t, "verify-health", "--json")
	t.Logf("err=%v stderr=%q", err, errOut)
	if len(got) > 0 {
		t.Errorf("CREDENTIAL MISROUTE: %d requests to saved host %s; first Authorization: %q", len(got), hostA, got[0])
	}
}

// A dir storage "nas" over a NAS mount on both nodes, left as shared=0: each
// node lists the same files. job1 and job3 both write vm 100 to nas at 21:00;
// job3 has failed every night (one file a night).
func TestRound29NonSharedFlagOnSharedMount(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["storage"] = "nas"
	addJob3(f, "nas")
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "nas", "type": "dir", "path": "/mnt/nas", "content": "backup"})
	f.pveRoutes["/nodes"] = []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "online"}}
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	st = append(st, map[string]any{"storage": "nas", "type": "dir", "shared": 0, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	f.pveRoutes["/nodes/pve1/storage"] = st
	f.pveRoutes["/nodes/pve2/storage"] = st
	f.pveRoutes["/nodes/pve2/storage/local/content"] = []any{}
	f.pveRoutes["/nodes/pve2/vzdump/defaults"] = map[string]any{}
	var files []any
	for h := 179.5; h > 0; h -= 24 {
		files = append(files, vzdumpEntry("nas", 100, f.now.Add(-time.Duration(h*float64(time.Hour))).Unix()))
	}
	f.pveRoutes["/nodes/pve1/storage/nas/content"] = files
	f.pveRoutes["/nodes/pve2/storage/nas/content"] = files
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MetSchedule {
		t.Errorf("FALSE GREEN: one file a night for two 21:00 jobs, yet met_schedule=true scheduled=%d missed=%d evidence=%s restore_points=%d", r.ScheduledRuns, r.MissedRuns, r.Evidence, r.RestorePoints)
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--json")
	t.Logf("restore-points: %.400s", out)
}

func httptestNewTLS(h http.HandlerFunc) *httptest.Server { return httptest.NewTLSServer(h) }
func certFP(s *httptest.Server) string                   { return client.CertFingerprint(s.Certificate()) }
