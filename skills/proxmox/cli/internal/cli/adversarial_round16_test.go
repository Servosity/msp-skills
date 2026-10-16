package cli

import (
	"strings"
	"testing"
	"time"
)

// A PBS-only cluster whose token sees no datastore (no Datastore.Audit: PBS
// filters every list to empty) measured nothing: it exits 5 like verify-health.
func TestCapacityPBSOnlyNoVisibleDatastoreExits5(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "")
	f.pbsRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": true}}
	f.pbsRoutes["/status/datastore-usage"] = []map[string]any{}
	f.pbsRoutes["/admin/datastore"] = []map[string]any{}
	f.pbsRoutes["/admin/gc"] = []map[string]any{}
	for _, mode := range []string{"--json", "--human-friendly"} {
		out, _, err := runCLI(t, "capacity", "--timeout", "2s", mode)
		if ExitCode(err) != 5 || strings.Contains(out, "No backup storage at risk") {
			t.Errorf("%s: exit %d, want 5; stdout %q", mode, ExitCode(err), out)
		}
	}
}

// rpo: a guest whose only point is future-dated is unproven (partial), not
// "no restore points retained"; a point inside the tolerance counts.
func TestRPOFutureDatedIsPartialNotNone(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 105, 200*24*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.Evidence != "partial" || r.MetSchedule || !strings.Contains(r.Note, "dated in the future") {
		t.Errorf("future-only guest: evidence %s met %v note %q", r.Evidence, r.MetSchedule, r.Note)
	}
	f = newFixture(t)
	addPointAt(f, 105, 30*time.Minute, "ok")
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 1 {
		t.Errorf("a point 30 min ahead (inside the tolerance) was not counted: %+v", r)
	}
}

// Only the newest future-dated point's verify state decides: a newer one that
// verified ok supersedes an older failed one.
func TestFutureFailedSupersededByNewerFutureOK(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 100, 3*time.Hour, "failed")
	addPointAt(f, 100, 5*time.Hour, "ok")
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict == "verify-failed" {
		t.Errorf("a superseded failed point kept the guest red: %q", r.Reason)
	}
	out, _, _ = runCLI(t, "verify-health", "--all", "--json")
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Group == "vm/100" && g.NewestIsBad {
			t.Errorf("verify-health: newest_is_bad for a superseded failed point")
		}
	}
}

// A PBS-only entry's client is compared the way --client compares it: the
// cluster-name fallback and case-insensitive.
func TestPBSOnlySameClientFallbackAndCase(t *testing.T) {
	for _, tc := range []struct{ acme, pbs string }{{"", "ACME"}, {"Acme Dental", "acme dental"}} {
		f := newFixture(t)
		setAcmeTokens(t)
		acme := acmeClusters(f, f.pbs.URL, "")
		if tc.acme == "" {
			acme = strings.Replace(acme, `client = "Acme Dental"`, "", 1)
		}
		writeClustersFile(t, acme+`
[clusters.acmepbs]
client = "`+tc.pbs+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`)
		out, _, _ := runCLI(t, "--cluster", "acmepbs", "capacity", "--json")
		for _, r := range decodeJSON[capacityReport](t, out).Rows {
			if r.Kind == "pbs-datastore" && r.Attribution != "" {
				t.Errorf("acme=%q pbs=%q: the same client's PBS-only entry was marked %q", tc.acme, tc.pbs, r.Attribution)
			}
		}
	}
}

// A client whose reachable cluster lists no included guest still appears in
// posture --by-client, with that cluster counted.
func TestByClientCountsClusterWithoutGuests(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	empty := newFixture(t)
	empty.pveRoutes["/cluster/resources"] = []map[string]any{}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `
[clusters.acme2]
client = "Acme Dental"
pve_base_url = "`+empty.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+empty.pveFingerprint()+`"
`))
	out, _, _ := runCLI(t, "posture", "--by-client", "--json")
	seen := false
	for _, c := range decodeJSON[[]postureClientRow](t, out) {
		seen = seen || c.Tenant == "Acme Dental"
		if c.Tenant == "Acme Dental" && c.Clusters != 2 {
			t.Errorf("Acme Dental clusters %d, want 2", c.Clusters)
		}
	}
	if !seen {
		t.Errorf("Acme Dental missing from --by-client:\n%s", out)
	}
}
