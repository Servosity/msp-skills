// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// Codex round 8.

// pbs_namespace that cannot be tied to a datastore (here: the storage config
// read fails) was not read: absence below it is unproven.
func TestUntiedNamespaceIsUnreadEvidence(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{}
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "acme/old"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme/old"] = prodSnapshots(f)
	acmePVE := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			http.Error(w, `{"data":null}`, http.StatusInternalServerError)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(acmePVE.Close)
	writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+acmePVE.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(acmePVE.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
pbs_namespace = "acme"
`)
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == 100 && r.Verdict == "never" {
			t.Errorf("acme/100 is never with pbs_namespace unread: %q", r.Reason)
		}
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--json")
	for _, r := range decodeJSON[rpoReport](t, out).Rows {
		if r.VMID == 100 && r.Evidence == "full" {
			t.Errorf("rpo proves acme/100's misses with pbs_namespace unread: %+v", r)
		}
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--json")
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		if g.Evidence == "full" {
			t.Errorf("restore-points claims full evidence with pbs_namespace unread:\n%s", out)
		}
	}
}

// A peer whose client could not be built has no PVE identity; neither does a
// PBS-only profile. Two empty identities are not the same cluster.
func TestUnbuildablePeerSharesWithPBSOnlyProfile(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	writeClustersFile(t, `
[clusters.acmepbs]
client = "Acme Dental"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.beta]
pve_base_url = "https://127.0.0.1:9/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "zz"
pbs_host = "`+f.pbs.URL+`"
`)
	out, _, err := runCLI(t, "--cluster", "acmepbs", "failures", "--type", "verify", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeJSON[failuresReport](t, out).Rows
	if len(rows) == 0 {
		t.Fatalf("no PBS rows:\n%s", out)
	}
	for _, r := range rows {
		if r.Tenant != "" || !strings.Contains(r.Attribution, "beta (could not be read)") {
			t.Errorf("task credited to %q with beta's use of the PBS unknown: %+v", r.Tenant, r)
		}
	}
	out, _, _ = runCLI(t, "--cluster", "acmepbs", "verify-health", "--all", "--json")
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if !strings.HasPrefix(g.Attribution, "shared") {
			t.Errorf("group %s must be marked shared: %+v", g.Group, g)
		}
	}
}

// A pin setting that holds no fingerprint fails closed, never system trust.
func TestEmptyPinSettingFailsClosed(t *testing.T) {
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", " , ")
	if _, err := client.EnvPins("PROXMOX_PBS_TLS_FINGERPRINT"); err == nil {
		t.Error("comma-only PROXMOX_PBS_TLS_FINGERPRINT accepted")
	}
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	if pins, err := client.EnvPins("PROXMOX_PBS_TLS_FINGERPRINT"); err != nil || len(pins) != 0 {
		t.Errorf("unset pin must mean no pins: %v %v", pins, err)
	}
	if err := (clusterProfile{PBSHost: "pbs.example.com", PBSFingerprint: ","}).validate("acme"); err == nil {
		t.Error("comma-only pbs_fingerprint accepted")
	}
}

// Codex round 10: with no PBS host configured, a cluster whose storage backs
// up to a PBS has its PBS tasks unchecked, and failures says so.
func TestFailuresWithoutPBSHostNamesTheGap(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	out, _, err := runCLI(t, "failures", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[failuresReport](t, out)
	if e := failureByScope(rep.FetchFailures)["pbs tasks"]; !strings.Contains(e, "were not checked") {
		t.Errorf("want the unchecked PBS tasks named: %+v", rep.FetchFailures)
	}
}
