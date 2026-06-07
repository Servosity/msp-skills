// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the agents stale novel command.

package cli

import (
	"testing"
	"time"
)

func TestClassifyStaleAgents(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)
	items := rawItems(
		`{"hostname": "FRESH", "customerId": 1, "connectivity": "online", "lastConnected": "2026-06-05T00:00:00.000Z"}`,
		`{"hostname": "STALE-10D", "customerId": 2, "connectivity": "offline", "lastConnected": "2026-05-27T00:00:00.000Z"}`,
		`{"hostname": "STALE-30D", "customerId": 2, "connectivity": "offline", "lastConnected": "2026-05-07T00:00:00.000Z"}`,
		`{"hostname": "NO-TIMESTAMP", "customerId": 3, "connectivity": "offline"}`,
	)
	view := classifyStaleAgents(items, cutoff, now, 50)
	if view.StaleCount != 2 {
		t.Fatalf("StaleCount = %d, want 2 (fresh and timestamp-less agents excluded)", view.StaleCount)
	}
	if view.Agents[0].Hostname != "STALE-30D" {
		t.Errorf("not sorted longest-silent first: got %q", view.Agents[0].Hostname)
	}
	if view.Agents[0].DaysSilent != 30 {
		t.Errorf("DaysSilent = %d, want 30", view.Agents[0].DaysSilent)
	}
	if view.ByCustomer["2"] != 2 {
		t.Errorf("ByCustomer[2] = %d, want 2", view.ByCustomer["2"])
	}
}

func TestClassifyStaleAgentsLimit(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	cutoff := now
	items := rawItems(
		`{"hostname": "A", "lastConnected": "2026-01-01T00:00:00Z"}`,
		`{"hostname": "B", "lastConnected": "2026-02-01T00:00:00Z"}`,
		`{"hostname": "C", "lastConnected": "2026-03-01T00:00:00Z"}`,
	)
	view := classifyStaleAgents(items, cutoff, now, 2)
	if view.StaleCount != 3 {
		t.Errorf("StaleCount = %d, want 3 (count reflects all matches)", view.StaleCount)
	}
	if len(view.Agents) != 2 {
		t.Errorf("Agents = %d, want 2 (limit applied to rows, not count)", len(view.Agents))
	}
}

func TestClassifyStaleAgentsEmpty(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	view := classifyStaleAgents(nil, now.Add(-7*24*time.Hour), now, 50)
	if view.StaleCount != 0 || len(view.Agents) != 0 {
		t.Errorf("empty input should produce zero stale agents, got %+v", view)
	}
}
