// Adversarial round 28 receipts (rpo destinations and run windows).

package cli

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func dumpRPO(t *testing.T, out string) {
	var m map[string]any
	_ = json.Unmarshal([]byte(out), &m)
	b, _ := json.MarshalIndent(m["rows"], "", " ")
	t.Logf("rows: %s", b)
	c, _ := json.MarshalIndent(m["clients"], "", " ")
	t.Logf("clients: %s", c)
}

func everyRun(f *fixture, vmid int) {
	for h := 179.5; h > 0; h -= 24 {
		addPointAt(f, vmid, -time.Duration(h*float64(time.Hour)), "ok")
	}
}

// Second PBS storage on a DIFFERENT server, same datastore name "main".
func TestRPOSameDatastoreNameOnTwoServers(t *testing.T) {
	f := newFixture(t)
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "pbs2", "type": "pbs", "server": "offsite.example.net", "port": 8007, "datastore": "main", "namespace": ""})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbs2", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	f.pveRoutes["/nodes/pve1/storage/pbs2/content"] = []any{}
	addJob3(f, "pbs2")
	everyRun(f, 100)
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MetSchedule {
		t.Errorf("FALSE GREEN: job3 -> pbs2 (offsite.example.net) has no point, yet met_schedule=true missed=%d evidence=%s", r.MissedRuns, r.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	t.Logf("by-client: %s", out)
}

// Two PVE storages naming the same PBS datastore+namespace (an alias).
func TestRPOTwoStoragesOneDatastore(t *testing.T) {
	f := newFixture(t)
	hp := f.pbs.URL[len("https://"):]
	host, port := hp[:len(hp)-len(hp[lastColon(hp):])], hp[lastColon(hp)+1:]
	f.storageCfg = append(f.storageCfg, map[string]any{"storage": "pbs1b", "type": "pbs", "server": host, "port": port, "datastore": "main", "namespace": ""})
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbs1b", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 100, "avail": 900})
	addJob3(f, "pbs1b")
	everyRun(f, 100)
	f.pveRoutes["/nodes/pve1/storage/pbs1b/content"] = f.pveRoutes["/nodes/pve1/storage/pbs1/content"]
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MetSchedule {
		t.Errorf("FALSE GREEN: job1 (pbs1) and job3 (pbs1b) both write vm/100 to main at 21:00, one point per run exists, yet met_schedule=true missed=%d evidence=%s", r.MissedRuns, r.Evidence)
	}
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

// job1 was re-targeted from local to pbs1 three days ago; local still holds
// the guest's backup for each of the earlier runs; pbs1 holds one old manual
// backup from before the window and keeps everything (keep-all, no prune job).
func TestRPORetargetedJobMissesUnproven(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	addPointAt(f, 100, -200*time.Hour, "ok") // old manual backup on pbs1
	var local []any
	for h := 179.5; h > 0; h -= 24 {
		ts := f.now.Add(-time.Duration(h * float64(time.Hour))).Unix()
		if h > 72 {
			local = append(local, map[string]any{"volid": "local:backup/vzdump-qemu-100-" + time.Unix(ts, 0).UTC().Format("2006_01_02-15_04_05") + ".vma.zst", "vmid": 100, "ctime": ts, "size": 1 << 30, "subtype": "qemu", "format": "vma.zst"})
		} else {
			addPointAt(f, 100, -time.Duration(h*float64(time.Hour)), "ok")
		}
	}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = local
	out, errOut, err := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	t.Logf("err=%v stderr=%s", err, errOut)
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven == 0 && r.Evidence == "full" && !r.PointsUnread {
		t.Errorf("FALSE RED: %d proven missed runs with evidence full, yet every one of those windows holds a restore point on local", r.MissedRuns)
	}
}

// job1 (21:00) and job3 (22:00) both write vm/100 to pbs1; job1 reaches the
// guest late in its queue (22:30) and job3 at 22:40. Every run made its point.
func TestRPOLateQueuePointsOnSharedTarget(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-all=1"
	f.pbsRoutes["/admin/datastore/main/snapshots"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	f.pveRoutes["/cluster/backup"] = append(jobs, map[string]any{"id": "job3", "enabled": 1, "schedule": "22:00", "storage": "pbs1", "vmid": "100"})
	f.pveRoutes["/cluster/backup/job3/included_volumes"] = map[string]any{"children": []map[string]any{
		{"id": 100, "name": "web", "type": "qemu", "children": []any{map[string]any{"id": "scsi0", "name": "scsi0", "included": 1, "reason": "default"}}},
	}}
	first := f.now.Add(-180 * time.Hour)
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cluster/jobs/schedule-analyze") && r.URL.Query().Get("schedule") == "22:00" {
			from, _ := strconv.ParseInt(r.URL.Query().Get("starttime"), 10, 64)
			out := []map[string]any{}
			for ts := first.Add(time.Hour).Unix(); len(out) < 100; ts += 86400 {
				if ts >= from {
					out = append(out, map[string]any{"timestamp": ts})
				}
			}
			writeData(w, out)
			return true
		}
		return false
	})
	for d := 0; d < 8; d++ {
		base := first.Add(time.Duration(d) * 24 * time.Hour)
		for _, off := range []time.Duration{90 * time.Minute, 100 * time.Minute} {
			if at := base.Add(off); at.Before(f.now.Add(-time.Hour)) {
				addPointAt(f, 100, at.Sub(f.now), "ok")
			}
		}
	}
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	dumpRPO(t, out)
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns > 0 && r.MissedRunsUnproven == 0 && r.Evidence == "full" {
		t.Errorf("FALSE RED: %d proven misses with evidence full; every run of both jobs made its point", r.MissedRuns)
	}
}
