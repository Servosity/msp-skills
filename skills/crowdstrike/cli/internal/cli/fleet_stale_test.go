// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
	"time"
)

func TestStaleHosts(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	ents := []fleetEntity{
		{CID: "A", Kind: kindHost, ID: "fresh", Name: "fresh-host", LastSeen: now.Add(-2 * 24 * time.Hour)},
		{CID: "A", Kind: kindHost, ID: "old", Name: "old-host", LastSeen: now.Add(-20 * 24 * time.Hour)},
		{CID: "B", Kind: kindHost, ID: "never", Name: "never-host"}, // zero last_seen
		{CID: "A", Kind: kindAlert, ID: "a1"},                       // ignored (not a host)
	}
	got := staleHosts(ents, 14, now)
	if len(got) != 2 {
		t.Fatalf("want 2 stale hosts (old + never), got %d: %+v", len(got), got)
	}
	if got[0].ID != "never" || got[0].DaysAgo != -1 {
		t.Errorf("never-seen host must sort first with DaysAgo -1, got %+v", got[0])
	}
	if got[1].ID != "old" || got[1].DaysAgo != 20 {
		t.Errorf("old host want DaysAgo 20, got %+v", got[1])
	}
}

func TestStaleHostsNoneStale(t *testing.T) {
	now := time.Now()
	ents := []fleetEntity{{CID: "A", Kind: kindHost, ID: "h1", LastSeen: now.Add(-1 * time.Hour)}}
	got := staleHosts(ents, 14, now)
	if len(got) != 0 {
		t.Fatalf("want 0 stale, got %d", len(got))
	}
}
