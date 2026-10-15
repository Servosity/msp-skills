// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// Adversarial round 9.

func datastoreRow(t *testing.T, out, kind, name string) capacityRow {
	t.Helper()
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == kind && r.Name == name {
			return r
		}
	}
	t.Fatalf("no %s %s row:\n%s", kind, name, out)
	return capacityRow{}
}

// A job with remove=0 prunes nothing after its run.
func TestRemoveZeroJobDoesNotPrune(t *testing.T) {
	f := newFixture(t)
	for _, j := range f.pveRoutes["/cluster/backup"].([]map[string]any) {
		j["prune-backups"] = "keep-last=3"
		j["remove"] = 0
	}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if r := datastoreRow(t, out, "pbs-datastore", "main"); !strings.Contains(strings.Join(r.Flags, ","), "unpruned") {
		t.Errorf("remove=0 prunes nothing: want unpruned, got %v", r.Flags)
	}
}

// Attribution is per datastore: beta writing datastore other on the same PBS
// leaves acme's main its own.
func TestCapacityAttributionIsPerDatastore(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	beta := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			port, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "other", "namespace": ""}})
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
	out, _, err := runCLI(t, "--cluster", "acme", "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if r := datastoreRow(t, out, "pbs-datastore", "main"); r.Tenant != "Acme Dental" || r.Attribution != "" {
		t.Errorf("only acme writes main: %+v", r)
	}
	// beta now writes main too: shared, on the PBS row and the PVE storage row.
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+pbsOtherPVE(t, f, "", "").URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, _ = runCLI(t, "--cluster", "acme", "capacity", "--json")
	if r := datastoreRow(t, out, "pbs-datastore", "main"); r.Tenant != "" || !strings.HasPrefix(r.Attribution, "shared: ") {
		t.Errorf("beta also writes main: %+v", r)
	}
}

// verify-health names a PBS a storage backs up to that is not the
// configured one: its verify state was not read.
func TestVerifyHealthNamesUnreadPBS(t *testing.T) {
	f := newFixture(t)
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			port, _ := strconv.Atoi(hp[1])
			list := []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main"}}
			if r.URL.Query().Get("type") == "pbs" || r.URL.Query().Get("type") == "" {
				list = append(list, map[string]any{"storage": "offsite", "type": "pbs", "server": "pbs2.example.net", "datastore": "dr"})
			}
			writeData(w, list)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(pve.Close)
	t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
	out, _, err := runCLI(t, "verify-health", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[verifyReport](t, out).FetchFailures)["pbs verify"]; !strings.Contains(e, "offsite -> pbs2.example.net") {
		t.Errorf("want the unread offsite PBS named:\n%s", out)
	}
}

// An env setup with only a PBS host has no PVE storages to check.
func TestEnvPBSOnlyHasNoSpuriousStorageFailure(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "")
	out, _, err := runCLI(t, "failures", "--type", "verify", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "storage config could not be read") {
		t.Errorf("a PBS-only env setup has no PVE storage config:\n%s", out)
	}
}

// Codex round 15: a job inherits remove and prune-backups from its node's
// /etc/vzdump.conf when it sets neither.
func TestJobsInheritNodeVzdumpDefaults(t *testing.T) {
	unpruned := func(t *testing.T) bool {
		out, _, err := runCLI(t, "capacity", "--json")
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(strings.Join(datastoreRow(t, out, "pbs-datastore", "main").Flags, ","), "unpruned")
	}
	f := newFixture(t)
	for _, j := range f.pveRoutes["/cluster/backup"].([]map[string]any) {
		j["prune-backups"] = "keep-last=7"
	}
	f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{"remove": 0}
	if !unpruned(t) {
		t.Error("node default remove=0: the job prunes nothing, want unpruned")
	}
	f = newFixture(t)
	f.pveRoutes["/nodes/pve1/vzdump/defaults"] = map[string]any{"prune-backups": "keep-last=5"}
	if unpruned(t) {
		t.Error("node default prune-backups keep-last=5 prunes after every job run")
	}
}
