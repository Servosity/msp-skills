// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet trend snapshot diffing.

package cli

import (
	"testing"
	"time"
)

func TestTrendFromSnapshotsComputesDeltas(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	prior := now.Add(-7 * 24 * time.Hour)
	// loadFleetSnapshots orders newest-first per CID.
	snaps := []fleetSnapshot{
		{CID: "cid-worse", TakenAt: now, Hosts: 100, CriticalAlerts: 9, CriticalVulns: 12},
		{CID: "cid-worse", TakenAt: prior, Hosts: 100, CriticalAlerts: 4, CriticalVulns: 10},
		{CID: "cid-better", TakenAt: now, Hosts: 50, CriticalAlerts: 1, CriticalVulns: 2},
		{CID: "cid-better", TakenAt: prior, Hosts: 50, CriticalAlerts: 5, CriticalVulns: 6},
		{CID: "cid-flat", TakenAt: now, Hosts: 10, CriticalAlerts: 0, CriticalVulns: 0},
		{CID: "cid-flat", TakenAt: prior, Hosts: 10, CriticalAlerts: 0, CriticalVulns: 0},
	}
	view := trendFromSnapshots(snaps)
	if len(view.Tenants) != 3 {
		t.Fatalf("tenants = %d, want 3", len(view.Tenants))
	}
	// Worst sorts first.
	if view.Tenants[0].CID != "cid-worse" || view.Tenants[0].Direction != "worse" {
		t.Fatalf("first row = %s/%s, want cid-worse/worse", view.Tenants[0].CID, view.Tenants[0].Direction)
	}
	if view.Tenants[0].CriticalAlertsDelta != 5 || view.Tenants[0].CriticalVulnsDelta != 2 {
		t.Errorf("worse deltas = %d/%d, want 5/2",
			view.Tenants[0].CriticalAlertsDelta, view.Tenants[0].CriticalVulnsDelta)
	}
	if view.Tenants[1].CID != "cid-flat" || view.Tenants[1].Direction != "flat" {
		t.Errorf("second row = %s/%s, want cid-flat/flat", view.Tenants[1].CID, view.Tenants[1].Direction)
	}
	if view.Tenants[2].CID != "cid-better" || view.Tenants[2].Direction != "better" {
		t.Errorf("third row = %s/%s, want cid-better/better", view.Tenants[2].CID, view.Tenants[2].Direction)
	}
	if view.Note != "" {
		t.Errorf("unexpected note with full delta data: %q", view.Note)
	}
}

func TestTrendFromSnapshotsBaselineOnly(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	view := trendFromSnapshots([]fleetSnapshot{
		{CID: "cid-a", TakenAt: now, Hosts: 10, CriticalAlerts: 1, CriticalVulns: 0},
	})
	if len(view.Tenants) != 1 {
		t.Fatalf("tenants = %d, want 1", len(view.Tenants))
	}
	row := view.Tenants[0]
	if row.Direction != "baseline" || row.Prior != nil {
		t.Errorf("single-generation row = %s prior=%v, want baseline/nil", row.Direction, row.Prior)
	}
	if view.Note == "" {
		t.Error("expected single-generation note, got none")
	}
}

func TestTrendFromSnapshotsEmpty(t *testing.T) {
	view := trendFromSnapshots(nil)
	if len(view.Tenants) != 0 {
		t.Fatalf("expected no tenants, got %d", len(view.Tenants))
	}
	if view.Note == "" {
		t.Error("expected empty-store note, got none")
	}
}

func TestSnapshotFromEntitiesCountsPerCID(t *testing.T) {
	ents := []fleetEntity{
		{CID: "cid-a", Kind: kindHost, ID: "h1"},
		{CID: "cid-a", Kind: kindHost, ID: "h2"},
		{CID: "cid-a", Kind: kindAlert, ID: "a1", Severity: "critical", Status: "new"},
		{CID: "cid-a", Kind: kindAlert, ID: "a2", Severity: "low", Status: "new"},
		{CID: "cid-a", Kind: kindVuln, ID: "v1", Severity: "critical", Status: "open"},
		{CID: "cid-b", Kind: kindHost, ID: "h3"},
	}
	snaps := snapshotFromEntities(ents)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	a := snaps[0]
	if a.CID != "cid-a" || a.Hosts != 2 || a.CriticalAlerts != 1 || a.CriticalVulns != 1 {
		t.Errorf("cid-a snapshot = %+v, want hosts=2 critical_alerts=1 critical_vulns=1", a)
	}
	b := snaps[1]
	if b.CID != "cid-b" || b.Hosts != 1 || b.CriticalAlerts != 0 {
		t.Errorf("cid-b snapshot = %+v, want hosts=1", b)
	}
}

func TestSnapshotFromEntitiesExcludesClosedCriticals(t *testing.T) {
	// Closed/resolved criticals must not count, or fleet trend diverges from
	// fleet scorecard's open_critical_* numbers and masks real improvement.
	ents := []fleetEntity{
		{CID: "cid-a", Kind: kindAlert, ID: "a1", Severity: "critical", Status: "closed"},
		{CID: "cid-a", Kind: kindAlert, ID: "a2", Severity: "critical", Status: "resolved"},
		{CID: "cid-a", Kind: kindAlert, ID: "a3", Severity: "critical", Status: "new"},
		{CID: "cid-a", Kind: kindVuln, ID: "v1", Severity: "critical", Status: "closed"},
		{CID: "cid-a", Kind: kindVuln, ID: "v2", Severity: "critical", Status: "open"},
	}
	snaps := snapshotFromEntities(ents)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if snaps[0].CriticalAlerts != 1 || snaps[0].CriticalVulns != 1 {
		t.Errorf("snapshot = %+v, want critical_alerts=1 critical_vulns=1 (closed excluded)", snaps[0])
	}
}

func TestWriteFleetSnapshotsDistinctSubSecondGenerations(t *testing.T) {
	// Two syncs in the same wall-clock second must produce two snapshot
	// generations (RFC3339Nano PK), or fleet trend silently loses one.
	dir := t.TempDir()
	ctx := t.Context()
	st, err := openFleetStore(ctx, dir+"/fleet.db")
	if err != nil {
		t.Fatalf("openFleetStore: %v", err)
	}
	defer st.Close()
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	snap := []fleetSnapshot{{CID: "cid-a", Hosts: 1}}
	if err := writeFleetSnapshots(ctx, st.DB(), snap, base); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeFleetSnapshots(ctx, st.DB(), snap, base.Add(500*time.Millisecond)); err != nil {
		t.Fatalf("second write: %v", err)
	}
	snaps, err := loadFleetSnapshots(ctx, st)
	if err != nil {
		t.Fatalf("loadFleetSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("snapshot generations = %d, want 2 (sub-second collision)", len(snaps))
	}
}

func TestSplitFabricKind(t *testing.T) {
	kinds, fabric := splitFabricKind([]string{"hosts", "fabric", "vulns"})
	if !fabric {
		t.Error("fabric kind not detected")
	}
	if len(kinds) != 2 || kinds[0] != "hosts" || kinds[1] != "vulns" {
		t.Errorf("kinds = %v, want [hosts vulns]", kinds)
	}
	kinds, fabric = splitFabricKind([]string{"hosts"})
	if fabric || len(kinds) != 1 {
		t.Errorf("unexpected fabric=%v kinds=%v for [hosts]", fabric, kinds)
	}
}
