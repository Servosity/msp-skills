package cli

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
)

func deadAddr(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens: connection refused
	return addr
}

// A PBS-only setup whose PBS does not answer is a cluster that did not
// answer: capacity exits 5 and never prints the all-clear.
func TestPBSOnlyCapacityDownExits5(t *testing.T) {
	_ = newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "")
	t.Setenv("PROXMOX_PBS_HOST", "https://"+deadAddr(t))
	for _, mode := range []string{"--json", "--human-friendly"} {
		out, _, err := runCLI(t, "capacity", "--timeout", "2s", mode)
		if code := ExitCode(err); code != 5 {
			t.Errorf("%s: exit %d, want 5 (%v)", mode, code, err)
		}
		if strings.Contains(out, "No backup storage at risk") {
			t.Errorf("%s: all-clear printed for a cluster that did not answer:\n%s", mode, out)
		}
	}
}

func addFuturePoint(f *fixture, vmid int) {
	future := f.now.Add(200 * 24 * time.Hour).Unix()
	f.pbsRoutes["/admin/datastore/main/snapshots"] = append(f.pbsRoutes["/admin/datastore/main/snapshots"].([]any),
		map[string]any{"backup-type": "vm", "backup-id": strconv.Itoa(vmid), "backup-time": future, "size": 1 << 30, "files": []string{"index.json.blob"}, "owner": "root@pam",
			"verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}})
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = append(f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any),
		map[string]any{"volid": "pbs1:backup/vm/" + strconv.Itoa(vmid) + "/future", "vmid": vmid, "ctime": future, "size": 1 << 30, "subtype": "qemu", "format": "pbs-vm",
			"verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}})
}

// A restore point dated in the future (a node clock that ran ahead) never
// counts toward age: guest 101's real newest point is 50h old, so it stays
// red, and the fresh guest 100 turns amber clock-skew.
func TestFutureDatedPointNeverCountsTowardAge(t *testing.T) {
	f := newFixture(t)
	addFuturePoint(f, 101)
	addFuturePoint(f, 100)
	out, errOut, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatalf("%v %s", err, errOut)
	}
	seen := 0
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.AgeHours < 0 {
			t.Errorf("%d: negative age %v", r.VMID, r.AgeHours)
		}
		switch r.VMID {
		case 101:
			seen++
			if r.Severity == "green" || !strings.Contains(r.Reason, "dated in the future") {
				t.Errorf("101: a future-dated point masked a stale guest: %s/%s %q", r.Verdict, r.Severity, r.Reason)
			}
		case 100:
			seen++
			if r.Verdict != "clock-skew" || r.Severity != "amber" {
				t.Errorf("100: want clock-skew/amber, got %s/%s %q", r.Verdict, r.Severity, r.Reason)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("rows for 100 and 101 missing:\n%s", out)
	}
}

// --verified-only that filters out read points says so; "could not be read"
// is for points nobody read.
func TestVerifiedOnlyReadButUnverifiedLine(t *testing.T) {
	f := newFixture(t)
	delete(f.pveRoutes, "/nodes/pve1/storage/local/content") // evidence partial
	out, _, _ := runCLI(t, "restore-points", "107", "--verified-only", "--human-friendly")
	if strings.Contains(out, "no restore point could be read") || !strings.Contains(out, "none verified") {
		t.Errorf("read-but-unverified points reported as unread:\n%s", out)
	}
}

// The --only empty-result line counts only the clusters that answered.
func TestPostureOnlyCountsAnsweringClusters(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	guests, _ := f.pveRoutes["/cluster/resources"].([]map[string]any)
	var fresh []map[string]any
	for _, g := range guests {
		if g["vmid"] == 100 {
			fresh = append(fresh, g)
		}
	}
	f.pveRoutes["/cluster/resources"] = fresh
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `
[clusters.beta]
client = "Beta Law"
pve_base_url = "https://`+deadAddr(t)+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`))
	out, _, _ := runCLI(t, "posture", "--only", "green", "--human-friendly", "--timeout", "2s")
	if !strings.Contains(out, "across 1 cluster(s) that answered") {
		t.Errorf("an unreachable cluster counted as seen:\n%s", out)
	}
	out, _, _ = runCLI(t, "posture", "--human-friendly", "--timeout", "2s")
	if !strings.Contains(out, "across 1 cluster(s) that answered") {
		t.Errorf("an unreachable cluster counted as seen:\n%s", out)
	}
}

// capacity attributes a network share another configured cluster mounts to
// no single client, like a shared PBS datastore.
func TestCapacitySharedNFSExportIsShared(t *testing.T) {
	run := func(t *testing.T, betaExport string) capacityRow {
		f := newFixture(t)
		st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
		f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "nas", "type": "nfs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 10, "avail": 990})
		f.pveRoutes["/nodes/pve1/storage/nas/content"] = []map[string]any{}
		f.storageCfg = []map[string]any{{"storage": "nas", "type": "nfs", "server": "NAS1.example.", "export": "/backup/", "content": "backup"}}
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
client = "Acme"
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.beta]
client = "Beta"
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
		out, _, _ := runCLI(t, "--cluster", "acme", "capacity", "--json")
		for _, r := range decodeJSON[capacityReport](t, out).Rows {
			if r.Kind == "pve-storage" && r.Name == "nas" {
				return r
			}
		}
		t.Fatalf("no nas row:\n%s", out)
		return capacityRow{}
	}
	if r := run(t, "//backup"); r.Tenant != "" || r.Attribution != "shared: beta" {
		t.Errorf("one export two clients mount was credited to one: tenant %q attribution %q", r.Tenant, r.Attribution)
	}
	if r := run(t, "/backup/beta"); r.Tenant == "" || r.Attribution != "" {
		t.Errorf("an export only acme mounts must stay acme's: tenant %q attribution %q", r.Tenant, r.Attribution)
	}
}
