// Adversarial round 37 + Codex round 49 receipts: dumpdir failures, the log cap, cost, UUID namespaces, running tasks.

package cli

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// adv38Points: VM 100 has a point before the window and one per fixture run
// except the 84h one.
func adv38Points(f *fixture) {
	addPointAt(f, 100, -170*time.Hour, "ok")
	for _, h := range []float64{155.9, 131.9, 107.9, 59.9} {
		addPointAt(f, 100, -time.Duration(h*float64(time.Hour)), "ok")
	}
}

// adv38JobTasks: one job1 task per run; the 84h one did not include VM 100
// (it was excluded from job1 then).
func adv38JobTasks(f *fixture, extra ...map[string]any) {
	var tasks []map[string]any
	for i, h := range []float64{156, 132, 108, 84, 60, 36, 12} {
		r := ago(f.now, h)
		upid := fmt.Sprintf("UPID:pve1:%08X:00000001:%08X:vzdump::root@pam:", i+10, r)
		tasks = append(tasks, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 600, "user": "root@pam"})
		if h == 84 {
			f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
				{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 100,103 --mode snapshot --storage pbs1"},
				{"n": 2, "t": "INFO: Starting Backup of VM 101 (qemu)"},
				{"n": 3, "t": "INFO: Finished Backup of VM 101 (00:00:42)"},
				{"n": 4, "t": "INFO: Backup job finished successfully"},
			}
			continue
		}
		f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
			{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 103 --mode snapshot --storage pbs1"},
			{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
			{"n": 3, "t": "INFO: Finished Backup of VM 100 (00:00:42)"},
			{"n": 4, "t": "INFO: Backup job finished successfully"},
		}
	}
	f.pveRoutes["/nodes/pve1/tasks"] = append(tasks, extra...)
}

func adv38Manual(f *fixture, cmdline string) map[string]any {
	at := ago(f.now, 80)
	upid := fmt.Sprintf("UPID:pve1:00000099:00000001:%08X:vzdump:100:root@pam:", at)
	f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
		{"n": 1, "t": "INFO: starting new backup job: " + cmdline},
		{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
		{"n": 3, "t": "ERROR: Backup of VM 100 failed - vma_queue_write: write error - No space left on device"},
		{"n": 4, "t": "INFO: Backup job finished with errors"},
	}
	return map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "100", "status": "job errors", "starttime": at, "endtime": at + 30, "user": "root@pam"}
}

// A manual backup to a USB dumpdir failed; vzdump.conf names pbs1 as the
// default storage. The dumpdir task never wrote to pbs1, yet rpo proves
// job1's 84h miss at pbs1 from it. The same task with --storage local is
// (correctly) left unproven.
func TestRound37DumpdirFailureProvesMissAtDefaultStorage(t *testing.T) {
	for _, tc := range []struct{ name, cmd string }{
		{"storage-local", "vzdump 100 --storage local --mode snapshot --compress zstd --node pve1"},
		{"dumpdir", "vzdump 100 --dumpdir /mnt/usb --mode snapshot --compress zstd --node pve1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{"storage": "pbs1"}
			adv38Points(f)
			adv38JobTasks(f, adv38Manual(f, tc.cmd))
			out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
			r := decodeJSON[rpoReport](t, out).Rows[0]
			t.Logf("err=%v stderr=%q runs=%d missed=%d unproven=%d evidence=%s met=%v times=%v note=%q", err, errOut, r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.MissedRunTimes, r.Note)
			if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
				t.Errorf("FALSE PROVEN MISS (%s): %d proven miss(es) at pbs1; the only failure on record wrote to %q, not pbs1", tc.name, c, tc.cmd)
			}
			bc, _, _ := runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
			t.Logf("by-client: %s", strings.TrimSpace(bc))
		})
	}
}

// Cost: a 30-node cluster with an hourly job, --days 7. Count the GETs and
// whether anything can be proven.
func TestRound37CostThirtyNodesHourly(t *testing.T) {
	f := newFixture(t)
	nodes := []map[string]any{}
	logs := 0
	for n := 1; n <= 30; n++ {
		node := fmt.Sprintf("pve%d", n)
		nodes = append(nodes, map[string]any{"node": node, "status": "online"})
		if n > 1 {
			f.pveRoutes["/nodes/"+node+"/storage"] = f.pveRoutes["/nodes/pve1/storage"]
			f.pveRoutes["/nodes/"+node+"/storage/pbs1/content"] = []any{}
			f.pveRoutes["/nodes/"+node+"/storage/local/content"] = []any{}
			f.pveRoutes["/nodes/"+node+"/vzdump/defaults"] = map[string]any{}
		}
		var tasks []map[string]any
		for h := 1; h <= 168; h++ {
			r := ago(f.now, float64(h))
			upid := fmt.Sprintf("UPID:%s:%08X:00000001:%08X:vzdump::root@pam:", node, h, r)
			tasks = append(tasks, map[string]any{"upid": upid, "node": node, "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 300, "user": "root@pam"})
			var lines []map[string]any
			lines = append(lines, map[string]any{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --mode snapshot --storage pbs1 --node " + node})
			for g := 0; g < 10; g++ {
				id := 1000 + n*10 + g
				lines = append(lines, map[string]any{"n": 2, "t": fmt.Sprintf("INFO: Starting Backup of VM %d (qemu)", id)})
				for p := 0; p < 20; p++ {
					lines = append(lines, map[string]any{"n": 3, "t": fmt.Sprintf("INFO: %3d%% (1.0 GiB of 10.0 GiB) in 5s, read: 200 MiB/s, write: 100 MiB/s", p*5)})
				}
				lines = append(lines, map[string]any{"n": 4, "t": fmt.Sprintf("INFO: Finished Backup of VM %d (00:01:00)", id)})
			}
			lines = append(lines, map[string]any{"n": 5, "t": "INFO: Backup job finished successfully"})
			f.pveRoutes["/nodes/"+node+"/tasks/"+upid+"/log"] = lines
			logs++
		}
		f.pveRoutes["/nodes/"+node+"/tasks"] = tasks
	}
	f.pveRoutes["/nodes"] = nodes
	before := f.pveHits.Load()
	start := time.Now()
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--json")
	el := time.Since(start)
	rep := decodeJSON[rpoReport](t, out)
	t.Logf("tasks=%d pve GETs=%d elapsed=%s err=%v stderr=%.300q rows=%d", logs, f.pveHits.Load()-before, el, err, errOut, len(rep.Rows))
	for _, r := range rep.Rows {
		t.Logf("vm %d missed=%d unproven=%d evidence=%s", r.VMID, r.MissedRuns, r.MissedRunsUnproven, r.Evidence)
	}
	_ = http.MethodGet
}

// A PBS namespace named by the client's tenant GUID (e.g. its M365 tenant
// id) is not a token, yet the UUID rule refuses the whole cluster with an
// error that says it "holds '!' or '='".
func TestRound37UUIDNamespaceRefusesCluster(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "tenant-3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "tenant-3f2504e0-4f89-11d3-9a0c-0305e82c3301"
`))
	out, errOut, err := runCLI(t, "clusters", "list", "--json")
	t.Logf("list: %s %s %v", out, errOut, err)
	out, errOut, err = runCLI(t, "--cluster", "acme", "rpo", "--days", "7", "--json")
	t.Logf("rpo: err=%v stderr=%q out=%.200q", err, errOut, out)
	if err != nil {
		t.Errorf("a GUID-named PBS namespace refuses the cluster: %v", err)
	}
}

// One GUID-named namespace in clusters.toml: does it take the other clients down?
func TestRound37UUIDNamespaceFleetBlast(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
client = "Beta Law"
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
pbs_namespace = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
`)
	for _, args := range [][]string{{"--cluster", "acme", "posture", "--json"}, {"posture", "--json"}, {"rpo", "--json"}} {
		out, errOut, err := runCLI(t, args...)
		t.Logf("%v: err=%v stderr=%.300q out=%.120q", args, err, errOut, out)
	}
}

// Same 30-node hourly cluster with 50 ms per PVE request (a WAN link to the
// client site): wall time of one rpo run, old-style (only failed logs) vs now.
func TestRound37CostLatency(t *testing.T) {
	f := newFixture(t)
	nodes := []map[string]any{}
	for n := 1; n <= 30; n++ {
		node := fmt.Sprintf("pve%d", n)
		nodes = append(nodes, map[string]any{"node": node, "status": "online"})
		if n > 1 {
			f.pveRoutes["/nodes/"+node+"/storage"] = f.pveRoutes["/nodes/pve1/storage"]
			f.pveRoutes["/nodes/"+node+"/storage/pbs1/content"] = []any{}
			f.pveRoutes["/nodes/"+node+"/storage/local/content"] = []any{}
			f.pveRoutes["/nodes/"+node+"/vzdump/defaults"] = map[string]any{}
		}
		var tasks []map[string]any
		for h := 1; h <= 168; h++ {
			r := ago(f.now, float64(h))
			upid := fmt.Sprintf("UPID:%s:%08X:00000001:%08X:vzdump::root@pam:", node, h, r)
			tasks = append(tasks, map[string]any{"upid": upid, "node": node, "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 300, "user": "root@pam"})
			f.pveRoutes["/nodes/"+node+"/tasks/"+upid+"/log"] = []map[string]any{
				{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --mode snapshot --storage pbs1 --node " + node},
				{"n": 5, "t": "INFO: Backup job finished successfully"},
			}
		}
		f.pveRoutes["/nodes/"+node+"/tasks"] = tasks
	}
	f.pveRoutes["/nodes"] = nodes
	inner := f.pve.Config.Handler
	var logGets int
	f.pve.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			logGets++
		}
		time.Sleep(50 * time.Millisecond)
		inner.ServeHTTP(w, r)
	})
	start := time.Now()
	_, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("rpo --guest 100 on 30 nodes x hourly at 50ms/request: %s, task-log GETs=%d, total GETs=%d err=%v stderr=%.200q", time.Since(start), logGets, f.pveHits.Load(), err, errOut)
}

// Logs of tasks that started before the window (the 48h slack) count against
// the 400-log cap, though no judged run can use them.
func TestRound37PreWindowLogsConsumeCap(t *testing.T) {
	for _, filler := range []int{0, 395} {
		t.Run(fmt.Sprint(filler), func(t *testing.T) {
			f := newFixture(t)
			adv38Points(f)
			var tasks []map[string]any
			for i, h := range []float64{156, 132, 108, 84, 60, 36, 12} {
				r := ago(f.now, h)
				upid := fmt.Sprintf("UPID:pve1:%08X:00000001:%08X:vzdump::root@pam:", i+10, r)
				st, line := "OK", "INFO: Finished Backup of VM 100 (00:00:42)"
				if h == 84 {
					st, line = "job errors", "ERROR: Backup of VM 100 failed - could not activate storage 'pbs1'"
				}
				tasks = append(tasks, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": st, "starttime": r, "endtime": r + 600, "user": "root@pam"})
				f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
					{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 103 --mode snapshot --storage pbs1"},
					{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
					{"n": 3, "t": line},
					{"n": 4, "t": "INFO: Backup job finished with errors"},
				}
			}
			for i := 0; i < filler; i++ { // hourly-ish tasks 7-9 days ago: before the window
				r := ago(f.now, 169+float64(i)*0.1)
				upid := fmt.Sprintf("UPID:pve1:%08X:00000002:%08X:vzdump::root@pam:", 1000+i, r)
				tasks = append(tasks, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 60, "user": "root@pam"})
				f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{{"n": 1, "t": "INFO: starting new backup job: vzdump 101 --storage pbs1"}, {"n": 2, "t": "INFO: Backup job finished successfully"}}
			}
			f.pveRoutes["/nodes/pve1/tasks"] = tasks
			out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
			r := decodeJSON[rpoReport](t, out).Rows[0]
			t.Logf("pre-window tasks=%d in-window tasks=7 missed=%d proven=%d evidence=%s", filler, r.MissedRuns, r.MissedRuns-r.MissedRunsUnproven, r.Evidence)
			if r.MissedRuns-r.MissedRunsUnproven == 0 {
				t.Errorf("pre-window tasks (%d) cost the in-window proof", filler)
			}
		})
	}
}

// Codex round 49: a still-running multi-guest task may hold the guest's
// successful retry: while it runs, no miss in its window is proven.
func TestRunningTaskMakesWindowUnprovable(t *testing.T) {
	f := newFixture(t)
	snaps := f.pbsRoutes["/admin/datastore/main/snapshots"].([]any)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = slices.DeleteFunc(snaps, func(s any) bool {
		m := s.(map[string]any)
		return m["backup-id"] == "108" && m["backup-time"] == ago(f.now, 4)
	})
	vols := f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any)
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = slices.DeleteFunc(vols, func(v any) bool {
		m := v.(map[string]any)
		return m["vmid"] == 108 && m["ctime"] == ago(f.now, 4)
	})
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/tasks"] = append(tasks, map[string]any{"upid": "UPID:pve1:9:9:9:vzdump::root@pam:", "node": "pve1", "type": "vzdump", "id": "", "status": "", "starttime": ago(f.now, 0.5), "user": "root@pam"})
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "108", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns != 1 || r.MissedRunsUnproven != 1 {
		t.Errorf("a running task overlaps the window: missed %d unproven %d", r.MissedRuns, r.MissedRunsUnproven)
	}
}

// Codex round 50: with PBS read only through PVE, a fresh guest is not green
// (a newer snapshot that failed verification may be hidden from the saved
// credential).
func TestPVEOnlyPBSFreshIsNotGreen(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Severity == "green" {
		t.Errorf("VM 100 green with PBS read only through PVE: %s %q", r.Verdict, r.Reason)
	}
}

// Round 38 P2: a token's secret as the host of a base URL is never printed.
func TestUUIDBaseURLNeverShown(t *testing.T) {
	newFixture(t)
	const secret = "5ec7e7aa-0000-4000-8000-00000000c0de"
	t.Setenv("PROXMOX_BASE_URL", "https://"+secret+":8006/api2/json")
	for _, args := range [][]string{{"doctor", "--json"}, {"posture", "--json"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, secret) {
			t.Errorf("%v printed the secret", args)
		}
	}
}

// Round 39 P2: a GUID-named PBS namespace (or one below it) is a real
// namespace, not a token.
func TestGUIDNamespaceIsValid(t *testing.T) {
	f := newFixture(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "5ec7e7aa-0000-4000-8000-00000000c0de/pve1"`))
	setAcmeTokens(t)
	out, _, _ := runCLI(t, "clusters", "list", "--json")
	if strings.Contains(out, "looks like an API token") {
		t.Errorf("a GUID-named namespace refused:\n%s", out)
	}
}

// Codex round 52: a job that started before the attempt window and failed
// inside it is the guest's last attempt.
func TestPostureReadsLongJobEndingInWindow(t *testing.T) {
	f := newFixture(t)
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	tasks[0]["starttime"], tasks[0]["endtime"] = ago(f.now, 49), ago(f.now, 1) // failed 1h ago after 48h
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/nodes/pve1/tasks") {
			return false
		}
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		var out []map[string]any
		for _, tk := range tasks {
			if tk["starttime"].(int64) >= since {
				out = append(out, tk)
			}
		}
		writeExtra(w, out, "total", len(out))
		return true
	})
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 108); r.LastAttemptStatus != "failed" {
		t.Errorf("VM 108's long job failed 1h ago, last attempt %q (%s)", r.LastAttemptStatus, r.Verdict)
	}
}
