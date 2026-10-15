// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"strings"
	"testing"
)

func acmeClusters(f *fixture, pbsHost, extra string) string {
	return `
[clusters.acme]
client = "Acme Dental"
pve_base_url = "` + f.pve.URL + `/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "` + f.pveFingerprint() + `"
pbs_host = "` + pbsHost + `"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "` + f.pbsFingerprint() + `"
` + extra
}

func setAcmeTokens(t *testing.T) {
	t.Setenv("ACME_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("ACME_PBS", "auditor@pbs!cli:00000000-0000-0000-0000-000000000001")
}

// clusters check probes PVE and PBS per cluster, and `--cluster X doctor`
// runs it instead of checking the shell environment.
func TestClustersCheck(t *testing.T) {
	f := newFixture(t)
	out, stderr, err := runCLI(t, "clusters", "check", "--json")
	if err != nil {
		t.Fatalf("clusters check on a healthy env config: %v\n%s", err, stderr)
	}
	rows := decodeJSON[[]clusterCheckRow](t, out)
	if len(rows) != 2 || rows[0].Status != "ok" || rows[0].Version != "8.2.4" || rows[1].Service != "pbs" || rows[1].Status != "ok" {
		t.Fatalf("want pve + pbs ok: %+v", rows)
	}

	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, "https://127.0.0.1:9", ""))
	out, _, err = runCLI(t, "--cluster", "acme", "doctor", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 1 {
		t.Fatalf("--cluster acme doctor with an unreachable PBS: err = %v, want exit 1", err)
	}
	rows = decodeJSON[[]clusterCheckRow](t, out)
	if len(rows) != 2 || rows[0].Status != "ok" || rows[1].Status != "failed" || rows[1].Detail == "" {
		t.Fatalf("want pve ok, pbs failed: %+v", rows)
	}
}

// A client whose only cluster is unreachable must still be a row in the
// per-client rollups, not vanish from a forwarded CSV.
func TestByClientKeepsUnreachableClient(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	t.Setenv("BETA_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000002")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `
[clusters.beta]
client = "Beta Law"
pve_host = "127.0.0.1:9"
pve_token_env = "BETA_PVE"
`))
	out, _, err := runCLI(t, "posture", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range decodeJSON[[]postureClientRow](t, out) {
		if c.Tenant == "Beta Law" {
			found = c.ClustersUnreachable == 1
		}
	}
	if !found {
		t.Errorf("posture --by-client dropped the unreachable client:\n%s", out)
	}
	out, _, err = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"Beta Law"`) || !strings.Contains(out, `"clusters_unreachable": 1`) {
		t.Errorf("rpo --by-client dropped the unreachable client:\n%s", out)
	}
}

// verify-health applies pbs_namespace only on this cluster's own datastores.
func TestVerifyHealthNamespaceStaysOnOwnDatastores(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main"}, {"store": "other"}}
	f.pbsRoutes["/admin/datastore/other/snapshots"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "777", "backup-time": ago(f.now, 1), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "failed", "upid": "UPID:pbs:verify"}},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "prod"`))
	out, stderr, err := runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatalf("verify-health: %v\n%s", err, stderr)
	}
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Store == "other" {
			t.Errorf("another datastore's group was listed under Acme: %+v", g)
		}
	}
}

// tail follows forever by default, so it must not be an MCP tool.
func TestTailIsNotAnMCPTool(t *testing.T) {
	c, _, err := RootCmd().Find([]string{"tail"})
	if err != nil || c.Annotations["mcp:hidden"] != "true" {
		t.Fatalf("tail must carry mcp:hidden: %v %v", err, c.Annotations)
	}
}
