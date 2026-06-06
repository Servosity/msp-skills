// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
	"time"
)

func TestScorecardRollup(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	ents := []fleetEntity{
		{CID: "A", Kind: kindHost, ID: "h1", LastSeen: now.Add(-2 * 24 * time.Hour)},  // active
		{CID: "A", Kind: kindHost, ID: "h2", LastSeen: now.Add(-20 * 24 * time.Hour)}, // stale
		{CID: "A", Kind: kindAlert, ID: "a1", Severity: "critical", Status: "new"},    // counts
		{CID: "A", Kind: kindAlert, ID: "a2", Severity: "critical", Status: "closed"}, // excluded (closed)
		{CID: "A", Kind: kindAlert, ID: "a3", Severity: "high", Status: "new"},        // excluded (not critical)
		{CID: "A", Kind: kindVuln, ID: "v1", Severity: "critical", Status: "open"},    // counts
		{CID: "A", Kind: kindPolicy, ID: "p1", Status: "true"},
		{CID: "A", Kind: kindPolicy, ID: "p2", Status: "false"},
	}
	got := scorecardRollup(ents, now)
	if len(got) != 1 {
		t.Fatalf("want 1 tenant row, got %d", len(got))
	}
	r := got[0]
	if r.CID != "A" || r.Hosts != 2 || r.ActiveSensors != 1 {
		t.Errorf("hosts/active wrong: %+v", r)
	}
	if r.CoveragePct != 50.0 {
		t.Errorf("coverage want 50.0, got %v", r.CoveragePct)
	}
	if r.OpenCritAlerts != 1 {
		t.Errorf("open critical alerts want 1, got %d", r.OpenCritAlerts)
	}
	if r.CriticalVulns != 1 {
		t.Errorf("critical vulns want 1, got %d", r.CriticalVulns)
	}
	if r.PreventionPols != 2 {
		t.Errorf("policies want 2, got %d", r.PreventionPols)
	}
}

func TestScorecardRollupEmpty(t *testing.T) {
	got := scorecardRollup(nil, time.Now())
	if got == nil || len(got) != 0 {
		t.Fatalf("empty input must yield non-nil empty slice, got %#v", got)
	}
}
