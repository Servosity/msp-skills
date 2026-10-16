package cli

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
