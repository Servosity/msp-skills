// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// Codex round 6.

// A namespace below pbs_namespace that another cluster writes to is skipped,
// and nothing else reads it: absence there is unproven, never "never".
func TestSkippedSharedDescendantIsUnreadEvidence(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	// Every other source is readable and empty.
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{}
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "acme/shared"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = []any{}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme/shared"] = prodSnapshots(f)
	beta := pbsOtherPVE(t, f, "acme/shared", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`)+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == 100 && (r.Verdict == "never" || r.Verdict == "fresh") {
			t.Errorf("acme/100 is %s while acme/shared was not read: %q", r.Verdict, r.Reason)
		}
	}
	out, _, _ = runCLI(t, "--cluster", "acme", "rpo", "--days", "7", "--json")
	for _, r := range decodeJSON[rpoReport](t, out).Rows {
		if r.VMID == 100 && r.Evidence == "full" {
			t.Errorf("rpo treats acme/100's misses as proven with acme/shared unread: %+v", r)
		}
	}
	out, _, _ = runCLI(t, "--cluster", "acme", "restore-points", "100", "--json")
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		if g.Evidence == "full" {
			t.Errorf("restore-points claims full evidence with acme/shared unread:\n%s", out)
		}
	}
}

// One address, two spellings: one PBS endpoint.
func TestPBSEndpointCanonicalizesAddresses(t *testing.T) {
	for _, c := range [][2]string{
		{"fd00:0:0:0:0:0:0:10", "fd00::10"},
		{"[FD00::10]", "fd00::10"},
		{"0:0:0:0:0:ffff:7f00:1", "127.0.0.1"},
		{"PBS.Example.com.", "pbs.example.com"},
	} {
		if a, b := pbsEndpoint(c[0], 0), pbsEndpoint(c[1], 0); a != b {
			t.Errorf("%q -> %s, %q -> %s: want one endpoint", c[0], a, c[1], b)
		}
	}
}

// Beta's storage names acme's PBS by another spelling of the same address:
// still the same namespace, so neither cluster is credited.
func TestSharedNamespaceDetectedAcrossAddressSpellings(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	port := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")[1]
	beta := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			p, _ := strconv.Atoi(port)
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": "0:0:0:0:0:ffff:7f00:1", "port": p, "datastore": "main", "namespace": ""}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(beta.Close)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "written by another configured cluster (acme, beta)") {
		t.Errorf("beta writes acme's namespace through another spelling of the address:\n%s", out)
	}
}

// Two entries name one PBS by different DNS aliases with no profile pin; their
// storages pin the same certificate. A PBS task there is no single client's.
func TestPBSTaskSharersMatchStorageFingerprints(t *testing.T) {
	own := &pbsOwnership{writers: map[string]map[string]string{}, svc: pbsServices{}}
	acmeRef := pbsStorageRef{server: pbsEndpoint("pbs-a.example.com", 0), datastore: "main", namespace: "acme", fingerprint: "AA:BB"}
	betaRef := pbsStorageRef{server: pbsEndpoint("pbs-b.example.com", 0), datastore: "main", namespace: "beta", fingerprint: "aabb"}
	own.svc.link(pbsEndpointFromHost("pbs-a.example.com"))
	own.svc.link(pbsEndpointFromHost("pbs-b.example.com"))
	own.svc.link(acmeRef.server, acmeRef.fingerprint)
	own.svc.link(betaRef.server, betaRef.fingerprint)
	for id, ref := range map[string]pbsStorageRef{"https://acme": acmeRef, "https://beta": betaRef} {
		for _, k := range own.keys(ref, ref.namespace) {
			if own.writers[k] == nil {
				own.writers[k] = map[string]string{}
			}
			own.writers[k][id] = strings.TrimPrefix(id, "https://")
		}
	}
	acme := fleetTarget{Name: "acme", Profile: clusterProfile{PBSHost: "pbs-a.example.com"}, own: own, pveID: "https://acme",
		C: &client.Client{Config: &config.Config{TemplateVars: map[string]string{"pbs_host": "pbs-a.example.com"}}}}
	if got := acme.pbsServiceSharers(); strings.Join(got, ",") != "acme,beta" {
		t.Errorf("want acme,beta sharing the PBS, got %v", got)
	}
}

// A declared pbs_namespace scopes a peer to the datastores it writes to: the
// same namespace name on another datastore is someone else's.
func TestDeclaredPeerIsScopedToItsDatastores(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "prod"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "prod"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#prod"] = prodSnapshots(f)
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	beta := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			p, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": p, "datastore": "other", "namespace": "prod"}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(beta.Close)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "prod"`)+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_namespace = "prod"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "written by another configured cluster") {
		t.Errorf("beta writes prod on datastore other, not main:\n%s", out)
	}
}

// Codex round 7: a datastore this cluster's storages write to that the PBS
// token's (privilege-filtered) datastore list omits is unread evidence below
// pbs_namespace, never proof of no backups.
func TestUnlistedDatastoreIsUnreadEvidence(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "acme"
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = []any{}
	f.pveRoutes["/nodes/pve1/storage/local/content"] = []any{}
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "other"}}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	rep, rows := postureByVMID(t)
	if r := rows[100]; r.Verdict == "never" {
		t.Errorf("acme/100 is never while datastore main was not listed: %q %+v", r.Reason, rep.FetchFailures)
	}
	out, _, _ := runCLI(t, "restore-points", "100", "--json")
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		if g.Evidence == "full" {
			t.Errorf("restore-points claims full evidence with datastore main unlisted:\n%s", out)
		}
	}
	out, _, _ = runCLI(t, "verify-health", "--json")
	if rep := decodeJSON[verifyReport](t, out); len(rep.FetchFailures) == 0 || len(rep.Stores) > 0 {
		t.Errorf("verify-health must say datastore main was not read:\n%s", out)
	}
}
