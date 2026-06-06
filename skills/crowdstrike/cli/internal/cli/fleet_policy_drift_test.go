// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestPolicyDrift(t *testing.T) {
	ents := []fleetEntity{
		{CID: "A", Kind: kindPolicy, ID: "1", Name: "Windows:default", Status: "true"},
		{CID: "A", Kind: kindPolicy, ID: "2", Name: "Mac:default", Status: "true"},
		{CID: "B", Kind: kindPolicy, ID: "3", Name: "Windows:default", Status: "true"}, // missing Mac:default
		{CID: "B", Kind: kindPolicy, ID: "4", Name: "Mac:default", Status: "false"},    // disabled -> not in signature
	}
	got := policyDrift(ents, "A")
	byCID := map[string]driftRow{}
	for _, r := range got {
		byCID[r.CID] = r
	}
	if !byCID["A"].Matches {
		t.Errorf("baseline A must match itself: %+v", byCID["A"])
	}
	if byCID["B"].Matches {
		t.Errorf("B must drift (missing Mac:default): %+v", byCID["B"])
	}
	if len(byCID["B"].Missing) != 1 || byCID["B"].Missing[0] != "Mac:default" {
		t.Errorf("B missing want [Mac:default], got %+v", byCID["B"].Missing)
	}
	// drifted tenants sort first
	if got[0].CID != "B" {
		t.Errorf("drifted tenant B should sort first, got %s", got[0].CID)
	}
}

func TestPolicySignatureEnabledOnly(t *testing.T) {
	ents := []fleetEntity{
		{CID: "A", Kind: kindPolicy, ID: "1", Name: "Windows:default", Status: "true"},
		{CID: "A", Kind: kindPolicy, ID: "2", Name: "Linux:default", Status: "false"},
	}
	sig := policySignature(ents, "A")
	if len(sig) != 1 || sig[0] != "Windows:default" {
		t.Errorf("signature must include only enabled policies, got %+v", sig)
	}
}
