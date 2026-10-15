// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// netStorageID: one identity per share, however it is spelled.
func TestNetStorageID(t *testing.T) {
	same := func(a, b string) {
		t.Helper()
		if a == "" || a != b {
			t.Errorf("%q and %q should be one share", a, b)
		}
	}
	same(netStorageID("nfs", "NAS1.example.", "/backup/", "", "", "", "", ""), netStorageID("nfs", "nas1.example", "//backup", "", "", "", "", ""))
	same(netStorageID("nfs", "[FD00::10]", "/b", "", "", "", "", ""), netStorageID("nfs", "fd00:0::10", "/b", "", "", "", "", ""))
	same(netStorageID("cifs", "nas", "", "Backups", "ClientA/", "", "", ""), netStorageID("cifs", "NAS", "", "backups", "/clienta", "", "", ""))
	same(netStorageID("cephfs", "", "", "", "", "10.0.0.2 10.0.0.1", "fs", "/"), netStorageID("cephfs", "", "", "", "", "10.0.0.1,10.0.0.2", "fs", ""))
	if netStorageID("nfs", "nas", "/Backup", "", "", "", "", "") == netStorageID("nfs", "nas", "/backup", "", "", "", "", "") {
		t.Error("NFS export paths are case-sensitive")
	}
	if netStorageID("cifs", "nas", "", "backups", "a", "", "", "") == netStorageID("cifs", "nas", "", "backups", "", "", "", "") {
		t.Error("a subdir is its own dump directory")
	}
	if netStorageID("cephfs", "", "", "", "", "", "cephfs", "") != "" || netStorageID("dir", "", "", "", "", "", "", "") != "" {
		t.Error("a cluster's own CephFS and a dir storage have no cross-cluster identity")
	}
}

// A cluster-local network storage (CephFS on the cluster's own Ceph) is never
// shared; an unread peer makes every listed share possibly shared.
func TestNetSharers(t *testing.T) {
	own := &pbsOwnership{net: map[string]map[string]string{"nfs://nas/b": {"pveA": "a", "pveB": "b"}},
		netOf: map[string]map[string]string{"pveA": {"nas": "nfs://nas/b", "ceph": ""}, "pveB": {"nas": "nfs://nas/b"}}}
	a := fleetTarget{Name: "a", pveID: "pveA", own: own}
	if got := a.netSharers(storageInfo{Storage: "nas", Type: "nfs"}); len(got) != 1 || got[0] != "b" {
		t.Errorf("nas sharers = %v, want [b]", got)
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
	if got := a.netSharers(storageInfo{Storage: "nas", Type: "nfs"}); len(got) != 2 {
		t.Errorf("an unread peer may mount it too: %v", got)
	}
	if got := (fleetTarget{Name: "z", pveID: "pveZ", own: own}).netSharers(storageInfo{Storage: "nas", Type: "nfs"}); len(got) == 0 {
		t.Error("a cluster whose own config was not read cannot rule sharing out")
	}
}
