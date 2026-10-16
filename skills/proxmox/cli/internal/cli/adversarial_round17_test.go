package cli

import (
	"strings"
	"testing"
	"time"
)

// Misses a future-dated point may explain are unproven in the row AND the
// client rollup, never proven misses of a guest without restore points.
func TestRPOFutureDatedMissesAreUnprovenInRollup(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 105, 200*24*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	rep := decodeJSON[rpoReport](t, out)
	if r := rep.Rows[0]; !r.PointsUnread || r.MetSchedule {
		t.Errorf("row: points_unread %v met %v", r.PointsUnread, r.MetSchedule)
	}
	for _, c := range rep.Clients {
		if c.GuestsUnproven != 1 || c.MissedRuns != 0 || c.MissedRunsUnproven == 0 || c.GuestsWithNoData != 0 {
			t.Errorf("rollup counts future-dated misses as proven: %+v", c)
		}
	}
}

// One stray future-dated point next to a restore point for every run leaves
// the schedule met: it can only add cover.
func TestRPOMetStaysMetWithStrayFuturePoint(t *testing.T) {
	f := newFixture(t)
	for d := 0; d < 8; d++ { // the fixture's daily runs: one point 1h after each
		addPointAt(f, 105, -time.Duration(180-24*d-1)*time.Hour, "ok")
	}
	addPointAt(f, 105, 200*24*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns != 0 || !r.MetSchedule || r.Evidence != "full" {
		t.Errorf("a stray future point unmet a covered schedule: met %v evidence %s missed %d note %q", r.MetSchedule, r.Evidence, r.MissedRuns, r.Note)
	}
}

// rpo --by-client lists a client whose reachable cluster has no backup job,
// as posture --by-client does.
func TestRPOByClientKeepsClientWithoutRows(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	other := newFixture(t)
	other.pveRoutes["/cluster/backup"] = []map[string]any{}
	other.pveRoutes["/nodes/pve1/storage"] = []map[string]any{{"storage": "local", "type": "dir", "shared": 0, "active": 1, "enabled": 1, "content": "backup,iso", "total": 1000, "used": 100, "avail": 900}}
	other.pveRoutes["/cluster/resources"] = other.pveRoutes["/cluster/resources"].([]map[string]any)[:2]
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `
[clusters.beta]
client = "Beta"
pve_base_url = "`+other.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+other.pveFingerprint()+`"
`))
	out, _, _ := runCLI(t, "--client", "Beta", "rpo", "--by-client", "--json")
	if !strings.Contains(out, `"Beta"`) {
		t.Errorf("client Beta missing from rpo --by-client:\n%s", out)
	}
}

// A same-client PBS-only entry next to a cluster that declares its
// pbs_namespace is credited with that namespace, as governance.md says.
func TestPBSOnlySameClientDeclaredNamespaceCredited(t *testing.T) {
	f := newFixture(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = f.pbsRoutes["/admin/datastore/main/snapshots"]
	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"
[clusters.acmepbs]
client = "Acme Dental"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`))
	out, _, _ := runCLI(t, "--cluster", "acmepbs", "capacity", "--json")
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && r.Attribution != "" {
			t.Errorf("same-client PBS-only entry marked %q", r.Attribution)
		}
	}
}

// restore-points --csv has no evidence column: partial evidence goes to stderr.
func TestRestorePointsCSVWarnsPartial(t *testing.T) {
	f := newFixture(t)
	delete(f.pveRoutes, "/nodes/pve1/storage/local/content")
	_, errOut, _ := runCLI(t, "restore-points", "107", "--csv")
	if !strings.Contains(errOut, "partial evidence, not read: storage local") {
		t.Errorf("CSV dropped the partial-evidence signal; stderr %q", errOut)
	}
}
