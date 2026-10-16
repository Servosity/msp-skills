// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
)

// Codex round 23: an archive on a network share carries only a VMID. When a
// second configured cluster mounts the same NFS export, its VM 100 archives
// sit next to this cluster's, so none is credited to either.
func TestSharedNFSExportCreditsNoCluster(t *testing.T) {
	run := func(t *testing.T, betaExport string) restorePointsReport {
		f := newFixture(t)
		st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
		f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "nas", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 10, "avail": 990})
		f.pveRoutes["/nodes/pve1/storage/nas/content"] = []map[string]any{{"volid": "nas:backup/vzdump-qemu-100.vma.zst", "vmid": 100, "ctime": ago(f.now, 1), "size": 1 << 30, "subtype": "qemu", "format": "vma.zst"}}
		f.storageCfg = []map[string]any{{"storage": "nas", "type": "nfs", "server": "NAS1.example.", "export": "/backup/", "content": "backup"}}
		// beta: another PVE cluster (its own origin) on the same fixture data,
		// with its nas export rewritten.
		beta := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := httptest.NewRecorder()
			f.pve.Config.Handler.ServeHTTP(rec, r)
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write([]byte(strings.ReplaceAll(rec.Body.String(), `"export":"/backup/"`, `"export":"`+betaExport+`"`)))
		}))
		t.Cleanup(beta.Close)
		setAcmeTokens(t)
		writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
		out, _, _ := runCLI(t, "--cluster", "acme", "restore-points", "100", "--json")
		return decodeJSON[restorePointsReport](t, out)
	}
	// The same export spelled the way beta spells it: shared.
	rep := run(t, "//backup")
	g := rep.Guests[0]
	for _, rp := range g.RestorePoints {
		if strings.HasPrefix(rp.Source, "pve-storage:nas") {
			t.Errorf("shared export credited to acme: %+v", rp)
		}
	}
	if g.Evidence != "partial" || !strings.Contains(strings.Join(g.NotRead, ";"), "storage nas") {
		t.Errorf("want storage nas in not_read: %+v", g)
	}
	if e := failureByScope(rep.FetchFailures)["storage nas"]; !strings.Contains(e, "beta") {
		t.Errorf("want the failure to name beta: %+v", rep.FetchFailures)
	}
	// A different export is this cluster's alone.
	rep = run(t, "/backup/beta")
	nas := 0
	for _, rp := range rep.Guests[0].RestorePoints {
		if strings.HasPrefix(rp.Source, "pve-storage:nas") {
			nas++
		}
	}
	if nas != 1 || failureByScope(rep.FetchFailures)["storage nas"] != "" {
		t.Errorf("an export only acme mounts must count: %d nas points, failures %+v", nas, rep.FetchFailures)
	}
}

// netStorageID: two spellings of one share have a token in common.
func TestNetStorageID(t *testing.T) {
	one := func(a, b []string) bool {
		for _, x := range a {
			if slices.Contains(b, x) {
				return true
			}
		}
		return false
	}
	id := func(typ, server, server2, export, share, subdir, monhost, fs, volume string) []string {
		return netStorageID(typ, server, server2, export, share, subdir, monhost, fs, volume)
	}
	for _, c := range [][2][]string{
		{id("nfs", "NAS1.example.", "", "/backup/", "", "", "", "", ""), id("nfs", "nas1.example", "", "//backup", "", "", "", "", "")},
		{id("nfs", "[FD00::10]", "", "/b", "", "", "", "", ""), id("nfs", "fd00:0::10", "", "/b", "", "", "", "", "")},
		{id("cifs", "nas", "", "", "Backups", "ClientA/", "", "", ""), id("cifs", "NAS", "", "", "backups", "/clienta", "", "", "")},
		{id("cephfs", "", "", "", "", "", "10.0.0.2 10.0.0.1", "fs", "/"), id("cephfs", "", "", "", "", "", "10.0.0.1:6789", "fs", "")},
		{id("glusterfs", "g1", "g2", "", "", "", "", "", "vol"), id("glusterfs", "g2", "g1", "", "", "", "", "", "vol")},
		{id("glusterfs", "g1", "g2", "", "", "", "", "", "vol"), id("glusterfs", "g2", "", "", "", "", "", "", "vol")},
	} {
		if !one(c[0], c[1]) {
			t.Errorf("%v and %v should be one share", c[0], c[1])
		}
	}
	for _, c := range [][2][]string{
		{id("nfs", "nas", "", "/Backup", "", "", "", "", ""), id("nfs", "nas", "", "/backup", "", "", "", "", "")},
		{id("cifs", "nas", "", "", "backups", "a", "", "", ""), id("cifs", "nas", "", "", "backups", "", "", "", "")},
		{id("glusterfs", "g1", "", "", "", "", "", "", "vol"), id("glusterfs", "g1", "", "", "", "", "", "", "other")},
	} {
		if one(c[0], c[1]) {
			t.Errorf("%v and %v are two directories", c[0], c[1])
		}
	}
	if len(id("cephfs", "", "", "", "", "", "", "cephfs", "")) != 0 || len(id("dir", "", "", "", "", "", "", "", "")) != 0 {
		t.Error("a cluster's own CephFS and a dir storage have no cross-cluster token")
	}
}

// A cluster-local network storage (CephFS on the cluster's own Ceph) is never
// shared; an unread peer makes every listed share possibly shared.
func TestNetSharers(t *testing.T) {
	own := &pbsOwnership{net: map[string]map[string]string{"glusterfs://g2/vol": {"pveA": "a", "pveB": "b"}, "glusterfs://g1/vol": {"pveA": "a"}},
		netOf: map[string]map[string][]string{"pveA": {"gv": {"glusterfs://g1/vol", "glusterfs://g2/vol"}, "ceph": nil}, "pveB": {"gv": {"glusterfs://g2/vol"}}}}
	a := fleetTarget{Name: "a", pveID: "pveA", own: own}
	if got := a.netSharers(storageInfo{Storage: "gv", Type: "glusterfs"}); len(got) != 1 || got[0] != "b" {
		t.Errorf("gv sharers = %v, want [b]", got)
	}
	if got := a.netSharers(storageInfo{Storage: "ceph", Type: "cephfs"}); got != nil {
		t.Errorf("local CephFS shared with %v", got)
	}
	if got := a.netSharers(storageInfo{Storage: "local", Type: "dir"}); got != nil {
		t.Errorf("dir storage shared with %v", got)
	}
	if got := a.netSharers(storageInfo{Storage: "hidden", Type: "nfs"}); len(got) == 0 {
		t.Error("a share missing from the config this token sees must not count as unshared")
	}
	own.netUnread = []string{"c"}
	if got := a.netSharers(storageInfo{Storage: "gv", Type: "glusterfs"}); len(got) != 2 {
		t.Errorf("an unread peer may mount it too: %v", got)
	}
	if got := (fleetTarget{Name: "z", pveID: "pveZ", own: own}).netSharers(storageInfo{Storage: "gv", Type: "glusterfs"}); len(got) == 0 {
		t.Error("a cluster whose own config was not read cannot rule sharing out")
	}
}

// Codex round 24: PBS lists a verify job only to a token with Audit or Verify
// on the job's own path. With Audit only on the client's namespace, a
// datastore-root verify job is hidden, so no verification age is judged ok.
func TestHiddenRootVerifyJobLeavesAgeUnjudged(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = prodSnapshots(f)
	f.pbsRoutes["/access/permissions"] = map[string]any{
		"/":                    map[string]any{"Sys.Audit": true},
		"/datastore":           map[string]any{"Datastore.Audit": false},
		"/datastore/main/acme": map[string]any{"Datastore.Audit": true},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	out, stderr, _ := runCLI(t, "verify-health", "--all", "--json")
	rep := decodeJSON[verifyReport](t, out)
	groups := 0
	for _, g := range rep.Groups {
		if g.Namespace == "acme" {
			groups++
			if g.Status == "ok" {
				t.Errorf("group %s judged ok while the verify jobs above acme are hidden", g.Group)
			}
		}
	}
	if groups == 0 {
		t.Fatalf("no acme groups read:\n%s%s", out, stderr)
	}
	if !strings.Contains(out, "verify jobs") || !strings.Contains(out, "/datastore/main") {
		t.Errorf("want the hidden verify jobs reported:\n%s", out)
	}
}

// Adversarial round 13: met_schedule needs a judged run. With the window's
// only run still within the in-progress grace, a guest whose newest point is
// 50h old has met nothing.
func TestRPONoJudgedRunIsNotMet(t *testing.T) {
	now := time.Now()
	old := restorePoint{Time: now.Add(-50 * time.Hour).Unix()}
	row := evaluateRPO([]restorePoint{old}, []int64{now.Add(-2 * time.Hour).Unix()}, now.Add(-24*time.Hour), now)
	if row.MetSchedule || row.ScheduledRuns != 0 {
		t.Errorf("no judged run, newest point 50h old: %+v", row)
	}
	// One judged run with its point, and the in-grace newest run with its point too.
	row = evaluateRPO([]restorePoint{old, {Time: now.Add(-20 * time.Hour).Unix()}, {Time: now.Add(-time.Hour).Unix()}}, []int64{now.Add(-22 * time.Hour).Unix(), now.Add(-2 * time.Hour).Unix()}, now.Add(-24*time.Hour), now)
	if !row.MetSchedule || row.ScheduledRuns != 1 {
		t.Errorf("one judged run with its point: want met: %+v", row)
	}
}

// Adversarial round 13 P2s: restore-points reports an unread job target as a
// fetch failure (as posture does), prints [] not null for no points, and
// --verified-only on a read-but-unverified guest does not claim "could not
// be read"; verify-health --agent keeps its incompleteness markers.
func TestRestorePointsGapAndEmptyCSV(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	delete(jobs[0], "storage")
	jobs[0]["dumpdir"] = "/mnt/backup"
	out, stderr, _ := runCLI(t, "restore-points", "100", "--json")
	if e := failureByScope(decodeJSON[restorePointsReport](t, out).FetchFailures)["job job1"]; !strings.Contains(e, "dumpdir") {
		t.Errorf("want the unread dumpdir target as a fetch failure:\n%s%s", out, stderr)
	}
	newFixture(t)
	out, _, _ = runCLI(t, "restore-points", "107", "--verified-only", "--csv")
	if strings.TrimSpace(out) == "null" {
		t.Error("--csv with no points printed null")
	}
	if !slices.Contains(verifyKeep, "verify_jobs_unknown") || !slices.Contains(verifyKeep, "snapshots_incomplete") {
		t.Error("verify-health --agent drops its incompleteness markers")
	}
}
