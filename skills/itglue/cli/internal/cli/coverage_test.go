// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the coverage novel feature.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelCoverageCommand(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "coverage", "--json")

	var rows []coverageRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 org rows, got %d: %s", len(rows), out)
	}

	byID := map[string]coverageRow{}
	for _, r := range rows {
		byID[r.OrganizationID] = r
	}

	acme, ok := byID["1"]
	if !ok {
		t.Fatal("Acme (org 1) missing from coverage")
	}
	if acme.Configurations != 1 || acme.Contacts != 2 || acme.Passwords != 2 || acme.Documents != 0 {
		t.Errorf("Acme counts wrong: %#v", acme)
	}
	if acme.Total != 5 {
		t.Errorf("Acme total = %d, want 5", acme.Total)
	}
	if len(acme.Missing) != 1 || acme.Missing[0] != "documents" {
		t.Errorf("Acme missing = %v, want [documents]", acme.Missing)
	}

	initech, ok := byID["3"]
	if !ok {
		t.Fatal("Initech (org 3) missing from coverage")
	}
	if initech.Total != 0 || len(initech.Missing) != 4 {
		t.Errorf("Initech should be empty with 4 missing: %#v", initech)
	}

	// Thinnest first.
	if rows[0].Total != 0 {
		t.Errorf("rows not sorted thinnest-first: first total = %d", rows[0].Total)
	}

	// --below 1 keeps orgs missing a whole category (Initech qualifies).
	out2 := runITGCmd(t, "coverage", "--below", "1", "--json")
	var rows2 []coverageRow
	if err := json.Unmarshal([]byte(out2), &rows2); err != nil {
		t.Fatalf("unmarshal --below: %v\n%s", err, out2)
	}
	foundInitech := false
	for _, r := range rows2 {
		if r.OrganizationID == "3" {
			foundInitech = true
		}
	}
	if !foundInitech {
		t.Errorf("--below 1 should include empty Initech; got %s", out2)
	}
}
