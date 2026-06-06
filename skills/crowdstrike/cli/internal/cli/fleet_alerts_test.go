// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestAlertQueue(t *testing.T) {
	ents := []fleetEntity{
		{CID: "A", Kind: kindAlert, ID: "a-new-crit", Severity: "critical", Status: "new"},
		{CID: "A", Kind: kindAlert, ID: "a-new-low", Severity: "low", Status: "new"},
		{CID: "A", Kind: kindAlert, ID: "a-closed", Severity: "critical", Status: "closed"},
		{CID: "B", Kind: kindAlert, ID: "a-prog-high", Severity: "high", Status: "in_progress"},
		{CID: "A", Kind: kindVuln, ID: "v1"}, // ignored
	}
	// status filter: only "new", severity-sorted
	q := alertQueue(ents, "new")
	if len(q) != 2 || q[0].ID != "a-new-crit" || q[1].ID != "a-new-low" {
		t.Fatalf("status=new queue wrong: %+v", q)
	}
	// no filter: all open (closed excluded), severity-sorted crit>high>low
	open := alertQueue(ents, "")
	if len(open) != 3 {
		t.Fatalf("open queue want 3, got %d: %+v", len(open), open)
	}
	if open[0].Severity != "critical" || open[1].Severity != "high" || open[2].Severity != "low" {
		t.Errorf("open queue severity sort wrong: %+v", open)
	}
}
