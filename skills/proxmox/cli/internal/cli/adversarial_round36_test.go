// Adversarial round 36 + Codex round 48 receipts: a failure proves a miss only at its destination and without a success; tokens in namespace/host fields.

package cli

import (
	"fmt"
	"net/http/httptest"
	"proxmox-pp-cli/internal/client"
	"strings"
	"testing"
	"time"
)

// adv37Tasks: one OK job1 task per fixture run (156..12h ago) that backed VM
// 100 up to pbs1, plus extra tasks.
func adv37Tasks(f *fixture, extra ...map[string]any) {
	var tasks []map[string]any
	for i, h := range []float64{156, 132, 108, 84, 60, 36, 12} {
		r := ago(f.now, h)
		upid := fmt.Sprintf("UPID:pve1:%08X:00000001:%08X:vzdump::root@pam:", i+10, r)
		tasks = append(tasks, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "OK", "starttime": r, "endtime": r + 600, "user": "root@pam"})
		f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
			{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 103 --mode snapshot --storage pbs1"},
			{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
			{"n": 3, "t": "INFO: Finished Backup of VM 100 (00:00:42)"},
			{"n": 4, "t": "INFO: prune older backups with retention: keep-daily=2, keep-weekly=4"},
			{"n": 5, "t": "INFO: Backup job finished successfully"},
		}
	}
	f.pveRoutes["/nodes/pve1/tasks"] = append(tasks, extra...)
}

func adv37Row(t *testing.T) rpoRow {
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	t.Logf("err=%v stderr=%q runs=%d missed=%d unproven=%d evidence=%s met=%v times=%v note=%q", err, errOut, r.ScheduledRuns, r.MissedRuns, r.MissedRunsUnproven, r.Evidence, r.MetSchedule, r.MissedRunTimes, r.Note)
	return r
}

// A: job1 -> pbs1 (GFS keep-daily=2,keep-weekly=4) backed VM 100 up every
// night (every run's task is OK and logs "Finished Backup of VM 100"); GFS
// pruned the middle dailies. A manual "Backup now" of VM 100 to local
// storage 80h ago failed (local disk full). rpo turns job1's 84h run, whose
// own task succeeded, into a PROVEN miss.
func TestRound36ManualFailureToOtherStorageProvesMiss(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-daily=2,keep-weekly=4"
	addPointAt(f, 100, -170*time.Hour, "ok") // the weekly kept before the window
	at := ago(f.now, 80)
	upid := fmt.Sprintf("UPID:pve1:00000099:00000001:%08X:vzdump:100:root@pam:", at)
	adv37Tasks(f, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "100", "status": "job errors", "starttime": at, "endtime": at + 30, "user": "root@pam"})
	f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
		{"n": 1, "t": "INFO: starting new backup job: vzdump 100 --storage local --mode snapshot --compress zstd --node pve1 --remove 0"},
		{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
		{"n": 3, "t": "ERROR: Backup of VM 100 failed - vma_queue_write: write error - No space left on device"},
		{"n": 4, "t": "INFO: Failed at 2026-10-13 05:00:30"},
		{"n": 5, "t": "INFO: Backup job finished with errors"},
	}
	r := adv37Row(t)
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE PROVEN MISS: %d proven missed run(s) at pbs1 although job1's own task for every run is OK and logs VM 100 finished; the only failure on record is a manual backup to 'local' (--storage local)", c)
	}
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--by-client", "--csv")
	t.Logf("by-client csv:\n%s", out)
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--agent")
	t.Logf("agent: %.600s", out)
}

// B: a second job (job3 -> local, same nightly time) failed for VM 100 on
// one night; job1 -> pbs1 succeeded that night (its point later thinned by
// GFS). The miss rpo proves at pbs1 is job3's, at local.
func TestRound36OtherJobFailureProvesMissAtFirstDestination(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-daily=2,keep-weekly=4"
	addPointAt(f, 100, -170*time.Hour, "ok")
	addJob3(f, "local")
	// local holds job3's archives for every night except 84h ago.
	var local []any
	for _, h := range []float64{156, 132, 108, 60, 36, 12} {
		local = append(local, vzdumpEntry("local", 100, ago(f.now, h)+300))
	}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = local
	at := ago(f.now, 84) + 1
	upid := fmt.Sprintf("UPID:pve1:00000098:00000001:%08X:vzdump::root@pam:", at)
	adv37Tasks(f, map[string]any{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "job errors", "starttime": at, "endtime": at + 30, "user": "root@pam"})
	f.pveRoutes["/nodes/pve1/tasks/"+upid+"/log"] = []map[string]any{
		{"n": 1, "t": "INFO: starting new backup job: vzdump 100 --mode snapshot --storage local"},
		{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
		{"n": 3, "t": "ERROR: Backup of VM 100 failed - vma_queue_write: write error - No space left on device"},
		{"n": 5, "t": "INFO: Backup job finished with errors"},
	}
	r := adv37Row(t)
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 1 {
		t.Errorf("FALSE PROVEN MISS: %d proven misses; only job3's one run (to local) failed, job1's run to pbs1 that night succeeded", c)
	}
}

// C: job1's 84h run failed for VM 100 (PBS rebooting); the admin re-ran job1
// ("Run now") 6h later, which backed VM 100 up to pbs1. That retry's point
// paid the run, then GFS thinned it. rpo proves the miss from the first
// task alone, ignoring the OK retry in the same window and destination.
func TestRound36RetrySucceededInWindow(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-daily=2,keep-weekly=4"
	addPointAt(f, 100, -170*time.Hour, "ok")
	fail := ago(f.now, 84) + 1
	retry := ago(f.now, 78)
	upF := fmt.Sprintf("UPID:pve1:00000097:00000001:%08X:vzdump::root@pam:", fail)
	upR := fmt.Sprintf("UPID:pve1:00000096:00000001:%08X:vzdump::root@pam:", retry)
	adv37Tasks(f,
		map[string]any{"upid": upF, "node": "pve1", "type": "vzdump", "id": "", "status": "job errors", "starttime": fail, "endtime": fail + 30, "user": "root@pam"},
		map[string]any{"upid": upR, "node": "pve1", "type": "vzdump", "id": "", "status": "OK", "starttime": retry, "endtime": retry + 600, "user": "root@pam"})
	f.pveRoutes["/nodes/pve1/tasks/"+upF+"/log"] = []map[string]any{
		{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 103 --mode snapshot --storage pbs1"},
		{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
		{"n": 3, "t": "ERROR: Backup of VM 100 failed - could not activate storage 'pbs1': pbs1: error fetching datastores - 500 Can't connect to 10.0.0.5:8007"},
		{"n": 4, "t": "INFO: Backup job finished with errors"},
	}
	f.pveRoutes["/nodes/pve1/tasks/"+upR+"/log"] = []map[string]any{
		{"n": 1, "t": "INFO: starting new backup job: vzdump --all 1 --exclude 103 --mode snapshot --storage pbs1"},
		{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
		{"n": 3, "t": "INFO: Finished Backup of VM 100 (00:00:42)"},
		{"n": 4, "t": "INFO: Backup job finished successfully"},
	}
	// job1's own 84h task is the failed one, not an OK one.
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	for i, tk := range tasks {
		if tk["starttime"] == ago(f.now, 84) {
			tasks = append(tasks[:i], tasks[i+1:]...)
			break
		}
	}
	f.pveRoutes["/nodes/pve1/tasks"] = tasks
	r := adv37Row(t)
	if c := r.MissedRuns - r.MissedRunsUnproven; c > 0 {
		t.Errorf("FALSE PROVEN MISS: %d proven; the run's window holds an OK vzdump task that logs VM 100 finished to pbs1 (its point later pruned)", c)
	}
}

// The round-35 rule rejects a token-shaped pbs_namespace and says "it is not
// shown", but clusters list prints it in the same row.
func TestRound36NamespaceTokenShownByClustersList(t *testing.T) {
	newFixture(t)
	const secret = "5ec7e7aa-0000-4000-8000-00000000c0de"
	writeClustersFile(t, `
[clusters.acme]
pve_host = "pve1.acme.lan"
pve_token_env = "ACME_PVE"
pbs_namespace = "pbs!cli=`+secret+`"
`)
	setAcmeTokens(t)
	out, errOut, _ := runCLI(t, "clusters", "list", "--json")
	if strings.Contains(out+errOut, secret) {
		t.Errorf("clusters list printed the token from pbs_namespace:\n%s", out)
	}
}

// A token's secret alone (what PVE's token dialog shows in its own "Secret"
// box) in pve_host: neither '!' nor '=', so it is used as a host, looked up
// in DNS and printed in every fetch failure.
func TestRound36BareSecretInHost(t *testing.T) {
	newFixture(t)
	const secret = "5ec7e7aa-0000-4000-8000-00000000beef"
	writeClustersFile(t, `
[clusters.acme]
pve_host = "`+secret+`"
pve_token_env = "ACME_PVE"
`)
	setAcmeTokens(t)
	for _, args := range [][]string{{"clusters", "list", "--json"}, {"posture", "--json", "--timeout", "3s"}, {"clusters", "check", "--json", "--timeout", "3s"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, secret) {
			i := strings.Index(out+errOut, secret)
			s := (out + errOut)[max(0, i-80):min(len(out+errOut), i+60)]
			t.Errorf("%v printed the bare secret: ...%s...", args, s)
		}
	}
}

// Codex round 48: --type other on a shared PBS still marks the shared tasks.
func TestFailuresTypeOtherSeesSharedPBS(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsRoutes["/nodes/localhost/tasks"] = []map[string]any{{"upid": "UPID:pbs:hostbackup", "node": "localhost", "worker_type": "backup", "worker_id": "main:host/fs1", "status": "TASK ERROR: connection lost", "starttime": ago(f.now, 3), "endtime": ago(f.now, 2.9), "user": "root@pam"}}
	other := httptest.NewTLSServer(f.pve.Config.Handler) // a second cluster writing the same PBS namespace
	defer other.Close()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+other.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(other.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`)
	out, _, _ := runCLI(t, "--cluster", "acme", "failures", "--type", "other", "--json")
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.UPID == "UPID:pbs:hostbackup" && r.Attribution == "" {
			t.Errorf("a shared PBS task credited to acme alone: %+v", r)
		}
	}
}
