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

// The PBS API stands in for a PBS storage the PVE token cannot list; when that
// PBS read fails, the job's target is unread: unproven, never "never".
func TestFailedPBSStandInIsUnproven(t *testing.T) {
	f := newFixture(t)
	auditOnly(f)
	delete(f.pbsRoutes, "/admin/datastore/main/snapshots")
	rep, rows := postureByVMID(t)
	if _, ok := failureByScope(rep.FetchFailures)["pbs:main"]; !ok {
		t.Fatalf("want the failed PBS read reported: %+v", rep.FetchFailures)
	}
	for _, id := range []int64{100, 101, 104, 105, 108} {
		if r := rows[id]; r.Verdict != "unproven" {
			t.Errorf("%d = %s %q, want unproven", id, r.Verdict, r.Reason)
		}
	}
	out, _, err := runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range decodeJSON[[]rpoClientRow](t, out) {
		if c.GuestsWithNoData != 0 || c.MissedRuns != 0 || c.GuestsUnproven == 0 {
			t.Errorf("rpo must count the unread target as unproven: %+v", c)
		}
	}
}

// A datastore this cluster does not write to but that carries its namespace is
// named, not dropped; a PVE outage lists every datastore with a note.
func TestVerifyHealthNamespaceNotesAndPVEOutage(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsRoutes["/admin/datastore"] = []map[string]any{{"store": "main"}, {"store": "archive"}}
	f.pbsRoutes["/admin/datastore/archive/namespace"] = []map[string]any{{"ns": ""}, {"ns": "prod"}}
	f.pbsRoutes["/admin/datastore/archive/snapshots"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ago(f.now, 1), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "failed", "upid": "UPID:pbs:verify"}},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "prod"`))
	out, stderr, err := runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatalf("verify-health: %v\n%s", err, stderr)
	}
	if e := failureByScope(decodeJSON[verifyReport](t, out).FetchFailures)["pbs archive"]; !strings.Contains(e, "also has namespace") {
		t.Errorf("the untied datastore carrying the namespace must be named:\n%s", out)
	}

	writeClustersFile(t, `
[clusters.acme]
pve_host = "127.0.0.1:9"
pve_token_env = "ACME_PVE"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
pbs_namespace = "prod"
`)
	out, stderr, err = runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatalf("verify-health with PVE down must still read PBS: %v\n%s", err, stderr)
	}
	rep := decodeJSON[verifyReport](t, out)
	if len(rep.Groups) == 0 || !strings.Contains(failureByScope(rep.FetchFailures)["pbs namespace"], "could not tie") {
		t.Errorf("want PBS groups plus a could-not-tie note:\n%s", out)
	}
}

// PBS gives no estimate for a datastore with under a week of samples: unknown,
// so --within keeps it.
func TestCapacityUnforecastPBSDatastoreIsKept(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/status/datastore-usage"] = []map[string]any{{"store": "main", "total": 1000 << 30, "used": 850 << 30, "avail": 150 << 30}}
	out, _, err := runCLI(t, "capacity", "--within", "30d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && r.Name == "main" {
			if !strings.Contains(strings.Join(r.Flags, ","), "no-forecast") {
				t.Errorf("main must carry no-forecast: %+v", r)
			}
			return
		}
	}
	t.Errorf("--within dropped the unforecast datastore:\n%s", out)
}

// --cluster X doctor --dry-run sends nothing and must not report ok rows; a
// 200 without a version is not Proxmox.
func TestClustersCheckDryRunAndVersionlessAnswer(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	out, _, err := runCLI(t, "--cluster", "acme", "doctor", "--dry-run", "--json")
	if err != nil || strings.Contains(out, `"status": "ok"`) {
		t.Errorf("dry-run doctor reported checks: err %v\n%s", err, out)
	}
	f.pveRoutes["/version"] = nil
	out, _, err = runCLI(t, "--cluster", "acme", "clusters", "check", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 1 || !strings.Contains(out, "without a PVE version") {
		t.Errorf("a versionless 200 must fail the check: err %v\n%s", err, out)
	}
}
