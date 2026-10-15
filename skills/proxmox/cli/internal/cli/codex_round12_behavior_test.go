// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http/httptest"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// Codex round 12.

// Two PBS servers can mint the same UPID (a clone keeps the node name): a
// task is deduplicated only within one PBS service.
func TestSameUPIDOnTwoPBSServersIsListedTwice(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	pbs2 := httptest.NewTLSServer(f.pbs.Config.Handler)
	t.Cleanup(pbs2.Close)
	writeClustersFile(t, `
[clusters.acme]
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.beta]
pbs_host = "`+pbs2.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+client.CertFingerprint(pbs2.Certificate())+`"
`)
	out, _, err := runCLI(t, "failures", "--type", "verify", "--json")
	if err != nil {
		t.Fatal(err)
	}
	clusters := map[string]bool{}
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.UPID == "UPID:pbs:verify" {
			clusters[r.Cluster] = true
		}
	}
	if !clusters["acme"] || !clusters["beta"] {
		t.Errorf("the same UPID on two PBS servers (here: one certificate, like a clone) must be listed for each: %v\n%s", clusters, out)
	}
	if strings.Contains(out, "storage config could not be read") {
		t.Errorf("PBS-only profiles have no PVE storages to check:\n%s", out)
	}
}

// A job with no storage of its own writes to its node's vzdump default; its
// retention prunes that storage.
func TestInheritedJobTargetRetentionCounts(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	for _, j := range jobs {
		delete(j, "storage")
		j["prune-backups"] = "keep-last=7"
	}
	f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{"storage": "pbs1"}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && strings.Contains(strings.Join(r.Flags, ","), "unpruned") {
			t.Errorf("a job pruning its inherited pbs1 target prunes the datastore: %+v", r)
		}
	}
}

// A read peer's datastores are scoped per PBS service: beta writing other on
// PBS1 and main on PBS2 does not claim main on PBS1.
func TestPeerDatastoresArePerService(t *testing.T) {
	own := &pbsOwnership{writers: map[string]map[string]string{}, svc: pbsServices{}}
	pbs1, pbs2 := pbsEndpoint("pbs1.example.com", 0), pbsEndpoint("pbs2.example.com", 0)
	own.svc.link(pbs1)
	own.svc.link(pbs2)
	p := peerScope{name: "beta", pve: "https://beta", ns: "prod",
		svcs:   append(own.svc.of(pbs1), own.svc.of(pbs2)...),
		stores: map[string][]string{own.svc.of(pbs1)[0]: {"other"}, own.svc.of(pbs2)[0]: {"main"}}}
	if own.covers(p, pbsStorageRef{server: pbs1, datastore: "main"}, "prod") {
		t.Error("beta claims main on PBS1, where it writes only other")
	}
	if !own.covers(p, pbsStorageRef{server: pbs2, datastore: "main"}, "prod") {
		t.Error("beta writes main on PBS2")
	}
}

// Adversarial round 8: a PBS datastore another configured cluster also uses
// is no single client's capacity row.
func TestSharedPBSDatastoreCapacityCreditsNoTenant(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	beta := pbsOtherPVE(t, f, "beta", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`)+`
[clusters.beta]
client = "Beta Law"
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
pbs_namespace = "beta"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && (r.Tenant != "" || !strings.HasPrefix(r.Attribution, "shared: ")) {
			t.Errorf("shared datastore credited to %q (attribution %q)", r.Tenant, r.Attribution)
		}
	}
}

// Codex round 13: a storage on another endpoint that pins the same
// certificate (a clone) is not the PBS whose tasks were read.
func TestSamePBSIsTheEndpointRead(t *testing.T) {
	own := &pbsOwnership{writers: map[string]map[string]string{}, svc: pbsServices{}}
	pin := "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
	own.svc.link(pbsEndpointFromHost("pbs1.example.com"), pin)
	own.svc.link(pbsEndpoint("pbs2.example.com", 0), pin)
	acme := fleetTarget{Name: "acme", Profile: clusterProfile{PBSHost: "pbs1.example.com", PBSFingerprint: pin}, own: own,
		C: &client.Client{Config: &config.Config{TemplateVars: map[string]string{"pbs_host": "pbs1.example.com"}}}}
	if acme.samePBS(pbsStorageRef{server: pbsEndpoint("pbs2.example.com", 0), fingerprint: pin}) {
		t.Error("a clone sharing the certificate counted as the PBS whose tasks were read")
	}
	if !acme.samePBS(pbsStorageRef{server: pbsEndpoint("pbs1.example.com", 0)}) {
		t.Error("the configured endpoint itself must match")
	}
}
