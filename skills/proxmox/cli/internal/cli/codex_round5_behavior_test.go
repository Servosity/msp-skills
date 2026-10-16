// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// Codex round 5.

// A failed PBS task names no PVE cluster: on a PBS another configured cluster
// also writes to it is no single client's, and a fleet run lists it once.
func TestSharedPBSTaskIsNotCreditedToOneClient(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	beta := pbsOtherPVE(t, f, "beta", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`)+`
[clusters.beta]
client = "Beta Law"
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
pbs_namespace = "beta"
`)
	for _, args := range [][]string{{"--cluster", "acme"}, {"--client", "Acme Dental"}, {}} {
		out, _, err := runCLI(t, append([]string{"failures", "--type", "verify,gc", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		rows := decodeJSON[failuresReport](t, out).Rows
		upids := map[string]int{}
		for _, r := range rows {
			upids[r.UPID]++
			if r.Tenant != "" || !strings.HasPrefix(r.Attribution, "shared: ") {
				t.Errorf("%v: PBS task %s on a shared PBS credited to %q (attribution %q)", args, r.UPID, r.Tenant, r.Attribution)
			}
		}
		if len(upids) == 0 {
			t.Fatalf("%v: no PBS rows:\n%s", args, out)
		}
		for u, n := range upids {
			if n != 1 {
				t.Errorf("%v: %s listed %d times", args, u, n)
			}
		}
	}
	// One cluster on its PBS alone: its tasks are its own.
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	out, _, err := runCLI(t, "failures", "--type", "verify", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.Tenant != "Acme Dental" || r.Attribution != "" {
			t.Errorf("sole cluster's task must be its own: %+v", r)
		}
	}
}

// A malformed pin is an error, never silently dropped to system trust.
func TestMalformedEnvPinIsRefused(t *testing.T) {
	t.Setenv("PROXMOX_TLS_FINGERPRINT", "not-a-fingerprint")
	if _, err := client.EnvPins("PROXMOX_TLS_FINGERPRINT"); err == nil {
		t.Fatal("malformed pin accepted")
	}
	newFixture(t)
	t.Setenv("PROXMOX_TLS_FINGERPRINT", "not-a-fingerprint")
	if _, _, err := runCLI(t, "posture", "--json"); err == nil {
		t.Fatal("posture ran with a malformed pin")
	}
}

// Below a PBS datastore a grant without the propagate flag hides every
// namespace under it, so the namespace list is incomplete.
func TestNonPropagatingNamespaceGrantHidesTheSubtree(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "acme/sub"}}
	f.pbsRoutes["/access/permissions"] = map[string]any{
		"/":                        map[string]any{"Sys.Audit": true, "Datastore.Audit": true},
		"/datastore/main/acme/sub": map[string]any{"Datastore.Audit": false, "Remote.Audit": true},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	rep, _ := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["pbs"]; !strings.Contains(e, "propagate") {
		t.Errorf("a non-propagating grant at acme/sub must flag the list below it: %+v", rep.FetchFailures)
	}
}
