// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 of the crowdstrike 0.1.11 branch, class completed
// refresh never removes disappeared records (handfix
// fleet-replace-only-a-proven-scope): the authenticated CID's rows ("self")
// are bound to the client that read them, in a fleet store written before the
// owner column too. The blackbox test TestR1FixSelfRefreshRemovesRemediated
// drives the same class on the built binary with two clients sharing a --db.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"crowdstrike-pp-cli/internal/store"
)

// fleetSchemaBeforeOwner is fleet_entities as 0.1.10 created it.
const fleetSchemaBeforeOwner = `
CREATE TABLE IF NOT EXISTS fleet_entities (
  cid TEXT NOT NULL, kind TEXT NOT NULL, id TEXT NOT NULL, name TEXT, severity TEXT, status TEXT,
  last_seen TEXT, synced_at TEXT, data TEXT, PRIMARY KEY (cid, kind, id)
);
INSERT INTO fleet_entities (cid, kind, id, name, severity, status, synced_at, data)
VALUES ('self', 'vuln', 'vul-legacy-1', 'CVE-2025-0001', 'critical', 'open', '2026-10-01T00:00:00Z', '{}');`

func TestR1FixSelfRefreshInAStoreBeforeTheOwnerColumn(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	st, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(fleetSchemaBeforeOwner); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	selfSync := func() {
		t.Helper()
		if out, errb, err := runFleet(t, "fleet", "sync", "--kinds", "vulns", "--db", db, "--json"); err != nil || strings.Contains(out, "store write") {
			t.Fatalf("fleet sync of the authenticated CID: %v\n%s%s", err, out, errb)
		}
	}
	f.set(refreshVulnPath, falconRecords(refreshCritVuln), false)
	selfSync()
	if out, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(out, "vul-remediated-1") {
		t.Fatalf("control: the first sync must store the open vulnerability:\n%s", out)
	}
	// Remediated: the complete, unfiltered list is empty.
	f.set(refreshVulnPath, falconRecords(), false)
	selfSync()
	out, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json")
	if strings.Contains(out, "vul-remediated-1") {
		t.Errorf("a complete refresh of the authenticated CID kept the remediated vulnerability:\n%s", out)
	}
	// A row written before the owner column is not current posture, and the
	// complete read of its kind has superseded it (handfix
	// fleet-legacy-rows-not-current).
	if strings.Contains(out, "vul-legacy-1") {
		t.Errorf("a pre-upgrade row is still reported after a complete refresh of its kind:\n%s", out)
	}
}

// Codex round 2 of the crowdstrike 0.1.11 branch, class historical rows remain
// counted as current after a complete refresh (handfix
// fleet-legacy-rows-not-current): a "self" row written before the owner column
// is not current posture for any reader, every reader says how many are
// excluded, and a complete unfiltered read of a kind deletes that kind's. The
// fabric, child tenants' rows and other clients' rows are not touched.
func TestR3LegacySelfRowsAreNotCurrentPosture(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	ctx := context.Background()
	st, err := store.OpenWithContext(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(fleetSchemaBeforeOwner + `
INSERT INTO fleet_entities (cid, kind, id, name, severity, status, last_seen, synced_at, data) VALUES
 ('self', 'alert', 'ldt:legacy', 'LegacyAlert', 'critical', 'new', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z', '{}'),
 ('self', 'host', 'dev-legacy', 'LegacyHost', '', 'normal', '2026-01-01T00:00:00Z', '2026-10-01T00:00:00Z', '{}'),
 ('self', 'policy', 'pol-legacy', 'Windows:LegacyPolicy', '', 'true', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z', '{}'),
 ('self', 'child_cid', 'cid-fabric', 'FabricChild', '', '', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z', '{}'),
 ('cid-tenant', 'vuln', 'vul-tenant', 'TenantVuln', 'high', 'open', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z', '{}');`); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	st, err = openFleetStore(ctx, db) // migrates: the rows above get owner ''
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeFleetEntities(ctx, st.DB(), []fleetEntity{{CID: "self", Kind: kindVuln, ID: "vul-other-owner", Name: "OtherOwner", Severity: "high", Status: "open"}}, nil, "another-clients-digest"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSyncState(fleetSyncStateKey, "", 1); err != nil {
		t.Fatal(err)
	}
	stored := func(id string) bool {
		t.Helper()
		var n int
		if err := st.DB().QueryRow(`SELECT COUNT(*) FROM fleet_entities WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	defer st.Close()

	legacy := []string{"vul-legacy-1", "CVE-2025-0001", "LegacyAlert", "LegacyHost", "LegacyPolicy"}
	for _, args := range [][]string{
		{"fleet", "scorecard"}, {"fleet", "vulns"}, {"fleet", "alerts"}, {"fleet", "stale"},
		{"fleet", "remediate"}, {"fleet", "policy-drift"}, {"fleet", "search", "Legacy"}, {"fleet", "tenants"},
	} {
		out, errb, err := runFleet(t, append(args, "--db", db, "--json")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out, errb)
		}
		for _, l := range legacy {
			if strings.Contains(out, l) {
				t.Errorf("%v reports the pre-upgrade row %s as current:\n%s", args, l, out)
			}
		}
		if strings.Count(errb, "4 row(s) written before this version") != 1 || !strings.Contains(errb, "next complete 'fleet sync' of that kind replaces them") {
			t.Errorf("%v must say once how many rows it excludes and what replaces them, stderr:\n%s", args, errb)
		}
	}
	if out, _, _ := runFleet(t, "fleet", "tenants", "--db", db, "--json"); !strings.Contains(out, "cid-fabric") {
		t.Errorf("control: the Flight Control fabric has no owner and stays:\n%s", out)
	}
	if out, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(out, "OtherOwner") || !strings.Contains(out, "TenantVuln") {
		t.Errorf("control: other clients' and child tenants' rows stay current:\n%s", out)
	}

	// A filtered read proves nothing about the kind: the pre-upgrade row stays
	// stored and excluded.
	if out, errb, err := runFleet(t, "fleet", "sync", "--kinds", "vulns", "--vuln-filter", "status:'open'+aid:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'", "--db", db, "--json"); err != nil {
		t.Fatalf("filtered fleet sync: %v\n%s%s", err, out, errb)
	}
	if !stored("vul-legacy-1") {
		t.Errorf("a filtered read deleted a pre-upgrade row")
	}

	// The complete, unfiltered, empty read of vulns: critical_vulns 0, that
	// kind's pre-upgrade row is gone, everything else is untouched.
	f.set(refreshVulnPath, falconRecords(), false)
	out, errb, err := runFleet(t, "fleet", "sync", "--kinds", "vulns", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
	}
	if !strings.Contains(out, "3 row(s) written before this version") {
		t.Errorf("fleet sync must name the rows still excluded:\n%s", out)
	}
	sc, _, err := runFleet(t, "fleet", "scorecard", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var scores []tenantScore
	if err := json.Unmarshal([]byte(sc), &scores); err != nil {
		t.Fatalf("scorecard JSON: %v\n%s", err, sc)
	}
	for _, s := range scores {
		if s.CriticalVulns != 0 || s.OpenCritAlerts != 0 {
			t.Errorf("a complete empty refresh left posture counted: %+v", s)
		}
	}
	if stored("vul-legacy-1") {
		t.Errorf("the complete read of vulns kept the pre-upgrade vuln row it superseded")
	}
	for _, id := range []string{"vul-other-owner", "vul-tenant", "cid-fabric", "ldt:legacy", "dev-legacy", "pol-legacy"} {
		if !stored(id) {
			t.Errorf("a complete read of vulns deleted %s, which it did not supersede", id)
		}
	}

	// The rest of the kinds: no pre-upgrade row left, no line printed, and the
	// posture snapshot counts only what the read returned.
	out, errb, err = runFleet(t, "fleet", "sync", "--kinds", "hosts,alerts,vulns,policies", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
	}
	if strings.Contains(out, "written before this version") {
		t.Errorf("no pre-upgrade row is left, no line is due:\n%s", out)
	}
	if _, errb, _ := runFleet(t, "fleet", "scorecard", "--db", db, "--json"); strings.Contains(errb, "written before this version") {
		t.Errorf("no pre-upgrade row is left, no line is due:\n%s", errb)
	}
	for _, id := range []string{"ldt:legacy", "dev-legacy", "pol-legacy"} {
		if stored(id) {
			t.Errorf("the complete read of its kind kept the pre-upgrade row %s", id)
		}
	}
	if !stored("cid-fabric") || !stored("vul-other-owner") {
		t.Errorf("a complete read removed the fabric or another client's row")
	}
	tr, _, err := runFleet(t, "fleet", "trend", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tr, `"self"`) || strings.Contains(tr, `"critical_vulns":1`) || strings.Contains(tr, `"critical_vulns": 1`) {
		t.Errorf("the trend snapshot must record self with no pre-upgrade row counted:\n%s", tr)
	}
}
