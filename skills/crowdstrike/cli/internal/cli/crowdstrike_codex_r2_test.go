// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 2 (4.34.2 crowdstrike), class
// fleet-replace-only-a-proven-scope: a fleet sync read replaces stored rows
// only for the scope it proves complete: one child CID (never the shared
// "self" label of whichever client authenticated), the default vulnerability
// filter (an operator's --vuln-filter narrows the read), and a page carrying
// CrowdStrike's resources envelope (a 200 null or {} proves nothing).

import (
	"strings"
	"testing"
)

func TestR2FilteredVulnReadReplacesNothing(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	a := `{"id":"vul-host-a","aid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"open","cve":{"id":"CVE-2026-2001","severity":"CRITICAL"}}`
	b := `{"id":"vul-host-b","aid":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","status":"open","cve":{"id":"CVE-2026-2002","severity":"CRITICAL"}}`
	f.set(refreshVulnPath, falconRecords(a, b), false)
	refreshSync(t, db, "vulns")
	f.set(refreshVulnPath, falconRecords(a), false)
	out, errb, err := runFleet(t, "fleet", "sync", "--member-cid", refreshChild, "--kinds", "vulns", "--vuln-filter", "status:'open'+aid:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s%s", err, out, errb)
	}
	if got, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(got, "vul-host-b") {
		t.Errorf("a --vuln-filter read deleted host B's still-open vulnerability:\n%s", got)
	}
	// Control: the default filter's complete read still replaces.
	f.set(refreshVulnPath, falconRecords(a), false)
	refreshSync(t, db, "vulns")
	if got, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); strings.Contains(got, "vul-host-b") {
		t.Errorf("control: a complete default-filter read must replace the CID's vulnerabilities:\n%s", got)
	}
}

func TestR2SecondParentFabricKeepsFirstParentsChildren(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set("/mssp/queries/children/v1", `{"meta":{"pagination":{"offset":0,"limit":500,"total":1}},"resources":["cidparentachild00000000000000001"],"errors":[]}`, false)
	f.set("/mssp/entities/children/GET/v2", falconRecords(`{"child_cid":"cidparentachild00000000000000001","name":"Parent A Child"}`), false)
	if _, _, err := runFleet(t, "fleet", "sync", "--kinds", "fabric", "--db", db, "--json"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FALCON_CLIENT_ID", "parentBClientId-77")
	t.Setenv("FALCON_CLIENT_SECRET", "parentBClientSecret-88")
	f.set("/mssp/queries/children/v1", `{"meta":{"pagination":{"offset":0,"limit":500,"total":1}},"resources":["cidparentbchild00000000000000002"],"errors":[]}`, false)
	f.set("/mssp/entities/children/GET/v2", falconRecords(`{"child_cid":"cidparentbchild00000000000000002","name":"Parent B Child"}`), false)
	if _, _, err := runFleet(t, "fleet", "sync", "--kinds", "fabric", "--db", db, "--json"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := runFleet(t, "fleet", "tenants", "--db", db, "--json")
	if !strings.Contains(got, "cidparentachild00000000000000001") {
		t.Errorf("parent B's fabric sync deleted parent A's child roster:\n%s", got)
	}
	if !strings.Contains(got, "cidparentbchild00000000000000002") {
		t.Errorf("control: parent B's child must be listed:\n%s", got)
	}
}

func TestR2PageWithoutAnEnvelopeReplacesNothing(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set(refreshVulnPath, falconRecords(refreshCritVuln), false)
	refreshSync(t, db, "vulns")
	for _, body := range []string{"{}", "null"} {
		f.set(refreshVulnPath, body, false)
		refreshSync(t, db, "vulns")
		if got, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(got, "vul-remediated-1") {
			t.Errorf("a 200 %s answer deleted the stored vulnerability:\n%s", body, got)
		}
	}
}
