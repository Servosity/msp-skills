package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// A per-endpoint read answered from the local mirror says so on stderr in
// every output form, not only in JSON meta.
func TestMirrorAnswerWarnsInEveryForm(t *testing.T) {
	f := newFixture(t)
	if _, e, err := runCLI(t, "sync", "--resources", "backup"); err != nil {
		t.Fatalf("%v %s", err, e)
	}
	f.pve.Close()
	f.pbs.Close()
	for _, form := range []string{"--csv", "--plain", "--quiet", "--json"} {
		_, errOut, err := runCLI(t, "backup", "job", form)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if !strings.Contains(errOut, "this answer is from the local mirror") {
			t.Errorf("%s: a mirror answer with no warning; stderr:\n%s", form, errOut)
		}
	}
}

// A token pasted where clusters.toml wants an env var NAME is never echoed.
func TestTokenInTokenEnvIsNeverEchoed(t *testing.T) {
	f := newFixture(t)
	const secret = "auditor@pve!cli=5ec7e7aa-0000-4000-8000-00000000c0de"
	writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "`+secret+`"
`)
	for _, args := range [][]string{{"posture", "--json"}, {"clusters", "list", "--json"}, {"clusters", "list"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, "5ec7e7aa") {
			t.Errorf("%v echoed the token:\n%s\n%s", args, out, errOut)
		}
	}
}

// A peer whose PBS storage list was read but whose full storage list failed
// still writes to the PBS its storages name: it is a sharer, never ignored.
func TestPeerPBSRefsSurviveAFailedStorageRead(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			if r.URL.Query().Get("type") != "pbs" {
				http.Error(w, `{"data":null}`, http.StatusInternalServerError)
				return
			}
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			port, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "namespace": ""}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer other.Close()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+other.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(other.Certificate())+`"
pbs_host = "https://pbs2.invalid:8007"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if r := postureRowFor(t, out, 100); r.Verdict == "fresh" {
		t.Errorf("acme/100 credited with a snapshot beta may own: %s", r.Reason)
	}
}

// A vzdump log names the guest type: a failed backup of deleted VM 108 is
// not container 108's, which now has that VMID.
func TestVzdumpOutcomeKeepsItsGuestType(t *testing.T) {
	f := newFixture(t)
	for _, g := range f.pveRoutes["/cluster/resources"].([]map[string]any) {
		if g["vmid"] == 108 {
			g["id"], g["type"], g["name"] = "lxc/108", "lxc", "new-container"
		}
	}
	out, _, _ := runCLI(t, "failures", "--json")
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.VMID == 108 && r.GuestName == "new-container" {
			t.Errorf("container 108 named as the guest of VM 108's failed backup")
		}
	}
	out, _, _ = runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 108); r.LastAttemptStatus == "failed" {
		t.Errorf("container 108 carries VM 108's failed attempt: %s %q", r.Verdict, r.Reason)
	}
}

// One backup time on two datastores, verified ok on one and failed on the
// other: the tie goes to the failure, never to the good copy.
func TestNewestTieGoesToTheBadCopy(t *testing.T) {
	f := newFixture(t)
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	port, _ := strconv.Atoi(hp[1])
	ts := ago(f.now, 2)
	f.storageCfg = []map[string]any{{"storage": "pbs2", "type": "pbs", "server": hp[0], "port": port, "datastore": "zz", "namespace": ""}}
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "pbs2", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 10, "avail": 990})
	failed := map[string]any{"state": "failed", "upid": "UPID:pbs:verify"}
	f.pveRoutes["/nodes/pve1/storage/pbs2/content"] = []any{map[string]any{"volid": "pbs2:backup/vm/100/" + strconv.FormatInt(ts, 10), "vmid": 100, "ctime": ts, "size": 1 << 30, "subtype": "qemu", "format": "pbs-vm", "verification": failed}}
	f.pveRoutes["/nodes/pve1/storage/pbs2/prunebackups"] = []any{}
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main", "backend-type": "filesystem", "mount-status": "nonremovable"}, {"store": "zz", "backend-type": "filesystem", "mount-status": "nonremovable"}}
	f.pbsRoutes["/admin/datastore/zz/snapshots"] = []any{map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ts, "size": 1 << 30, "files": []string{"index.json.blob"}, "owner": "root@pam", "verification": failed}}
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Severity == "green" || r.VerifyState != "failed" {
		t.Errorf("a same-time failed copy was hidden by the ok one: %s/%s %q", r.Verdict, r.VerifyState, r.Reason)
	}
}

// A password in a clusters.toml endpoint is never printed back.
func TestEndpointPasswordIsNeverEchoed(t *testing.T) {
	f := newFixture(t)
	const pw = "Pa5/5?wo#rdSECRET"
	writeClustersFile(t, `
[clusters.beta]
pve_host = "root:`+pw+`@pve-beta.invalid"
pve_token_env = "ACME_PVE"

[clusters.gamma]
pve_host = "`+strings.TrimPrefix(f.pve.URL, "https://")+`"
pbs_host = "https://backup:`+pw+`@pbs-gamma.invalid:8007"
pve_token_env = "ACME_PVE"
`)
	setAcmeTokens(t)
	for _, args := range [][]string{{"clusters", "list", "--json"}, {"clusters", "list"}, {"clusters", "check", "--json"}, {"posture", "--json"}, {"failures", "--json"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, pw) {
			t.Errorf("%v echoed the password", args)
		}
	}
}

// Only backups of this guest's type need its creation time: a VM whose
// config cannot be read, with only a container's backups under its VMID, has
// no restore point and full evidence.
func TestOtherTypeBackupsNeedNoCreationTime(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{map[string]any{"volid": "local:backup/vzdump-lxc-105-2026_10_01-00_00_00.tar.zst", "vmid": 105, "ctime": ago(f.now, 3), "size": 1 << 20, "subtype": "lxc", "format": "tar.zst"}}
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) {
			http.Error(w, `{"data":null}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	out, _, err := runCLI(t, "restore-points", "105", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; err != nil || g.Count != 0 || g.Evidence != "full" {
		t.Errorf("restore-points 105: err %v count %d evidence %s %v", err, g.Count, g.Evidence, g.NotRead)
	}
}

// A VM created inside the window, with a point after every run since: met.
func TestRPOCreatedInWindowMet(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 40) // own points at 26h and 2h; runs since creation at 36h and 12h
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; !r.MetSchedule || r.Evidence != "full" || r.ScheduledRuns != 2 || r.LargestGapHours < 0 {
		t.Errorf("rpo: met %v evidence %s runs %d gap %.1f note %q", r.MetSchedule, r.Evidence, r.ScheduledRuns, r.LargestGapHours, r.Note)
	}
}

// PROXMOX_HOST naming another endpoint than a base_url saved in config.toml:
// no request reaches the saved host; a --cluster run is not affected.
func TestHostDisagreementSendsNothing(t *testing.T) {
	f := newFixture(t)
	var hits atomic.Int64
	saved := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, `{"data":null}`, http.StatusUnauthorized)
	}))
	defer saved.Close()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("base_url = \""+saved.URL+"/api2/json\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOX_CONFIG", cfgPath)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", strings.TrimPrefix(f.pve.URL, "https://"))
	_, errOut, err := runCLI(t, "nodes", "list", "--json")
	if err == nil || hits.Load() != 0 || !strings.Contains(errOut+err.Error(), "wrong host") {
		t.Errorf("err %v, %d request(s) reached the saved host; stderr %s", err, hits.Load(), errOut)
	}
	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	if _, errOut, err := runCLI(t, "--cluster", "acme", "posture", "--json"); err != nil || hits.Load() != 0 {
		t.Errorf("--cluster acme: %v (%d saved-host hits) %s", err, hits.Load(), errOut)
	}
}
