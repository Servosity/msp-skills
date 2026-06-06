// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestSearchFleetEntities(t *testing.T) {
	ents := []fleetEntity{
		{CID: "A", Kind: kindHost, ID: "h1", Name: "ransomware-lab01"},
		{CID: "B", Kind: kindAlert, ID: "a1", Name: "Ransomware behavior", Severity: "critical"},
		{CID: "A", Kind: kindVuln, ID: "v1", Name: "CVE-2026-1", Severity: "high"},
		{CID: "A", Kind: kindHost, ID: "h2", Name: "web01"},
	}
	// case-insensitive substring across name; should match host + alert
	hits := searchFleetEntities(ents, "RANSOMWARE")
	if len(hits) != 2 {
		t.Fatalf("want 2 hits for ransomware, got %d: %+v", len(hits), hits)
	}
	// sorted by kind: alert before host
	if hits[0].Kind != kindAlert || hits[1].Kind != kindHost {
		t.Errorf("hits not kind-sorted: %+v", hits)
	}
	// match by severity term
	if got := searchFleetEntities(ents, "critical"); len(got) != 1 || got[0].ID != "a1" {
		t.Errorf("severity search want [a1], got %+v", got)
	}
	// match by CVE id
	if got := searchFleetEntities(ents, "cve-2026"); len(got) != 1 || got[0].ID != "v1" {
		t.Errorf("cve search want [v1], got %+v", got)
	}
	// empty term -> non-nil empty slice
	if got := searchFleetEntities(ents, "  "); got == nil || len(got) != 0 {
		t.Errorf("empty term must yield non-nil empty slice, got %#v", got)
	}
	// no match -> empty
	if got := searchFleetEntities(ents, "zzz-nomatch"); len(got) != 0 {
		t.Errorf("no-match want 0, got %+v", got)
	}
}
