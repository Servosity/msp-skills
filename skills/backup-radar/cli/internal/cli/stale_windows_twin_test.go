// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"

	"backup-radar-pp-cli/internal/radar"
)

// HAND-FIX(stale-hint-windows-twin) and HAND-FIX(stale-bodies-ascii-dedup).
func TestPSQuoteAndASCIIBodies(t *testing.T) {
	if got, want := psQuote("O’Brien's ‘x‛"), "'O’’Brien''s ‘‘x‛‛'"; got != want {
		t.Errorf("psQuote = %q, want %q", got, want)
	}
	in := map[string]string{"client_name": "Müller GmbH \U0001F600", "job_name": "<Daily> & co"}
	raw, _ := json.Marshal(in)
	out := asciiJSON(raw)
	for _, b := range out {
		if b >= 0x80 {
			t.Fatalf("non-ASCII byte in %q", out)
		}
	}
	var back map[string]string
	if err := json.Unmarshal(out, &back); err != nil || back["client_name"] != in["client_name"] || back["job_name"] != in["job_name"] {
		t.Fatalf("round trip: %v %q", err, back)
	}
}

// Two Backup Radar companies mapped to same-named ScalePad clients, seen only
// in v2 data (no client_id), stay in separate by_client groups.
func TestGroupStaleByClientV2OnlyKeepsCompaniesApart(t *testing.T) {
	g := groupStaleByClient([]radar.StaleRow{
		{Client: "Acme", BRCompany: "Acme Dental"},
		{Client: "Acme", BRCompany: "Acme Corp"},
		{Client: "Acme", BRCompany: "Acme Corp"},
		{Client: "Acme", ClientID: "c-1", BRCompany: "Acme Dental"},
		{Client: "Acme", ClientID: "c-1", BRCompany: "Acme Dental 2"},
	})
	if len(g) != 3 || g[0].Count != 1 || g[1].Count != 2 || g[1].BRCompany != "Acme Corp" || g[2].ClientID != "c-1" || g[2].Count != 2 {
		t.Fatalf("by_client = %+v", g)
	}
}
