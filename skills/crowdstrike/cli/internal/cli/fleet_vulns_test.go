// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestRankVulns(t *testing.T) {
	ents := []fleetEntity{
		{CID: "A", Kind: kindVuln, ID: "v-med", Severity: "medium"},
		{CID: "A", Kind: kindVuln, ID: "v-crit", Severity: "critical"},
		{CID: "B", Kind: kindVuln, ID: "v-high", Severity: "high"},
		{CID: "A", Kind: kindHost, ID: "h1"}, // ignored
	}
	all := rankVulns(ents, "")
	if len(all) != 3 {
		t.Fatalf("want 3 vulns, got %d", len(all))
	}
	if all[0].Severity != "critical" || all[1].Severity != "high" || all[2].Severity != "medium" {
		t.Errorf("severity sort wrong: %+v", all)
	}
	crit := rankVulns(ents, "critical")
	if len(crit) != 1 || crit[0].ID != "v-crit" {
		t.Errorf("severity filter want [v-crit], got %+v", crit)
	}
}
