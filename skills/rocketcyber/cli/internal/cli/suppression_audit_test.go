// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the suppression audit novel command.

package cli

import (
	"testing"
	"time"
)

func TestClassifySuppressionRules(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	staleAfter := 90 * 24 * time.Hour
	items := rawItems(
		`{"ruleId": 1, "ruleName": "stale-active", "status": "active", "updatedAt": "2025-12-01T00:00:00Z"}`,
		`{"ruleId": 2, "ruleName": "fresh-active", "status": "active", "updatedAt": "2026-06-04T00:00:00Z"}`,
		`{"ruleId": 3, "ruleName": "old-inactive", "status": "expired", "updatedAt": "2025-01-01T00:00:00Z"}`,
		`{"ruleId": 4, "ruleName": "created-only", "status": "active", "createdAt": "2026-06-05T00:00:00Z"}`,
	)
	view := classifySuppressionRules(items, now, staleAfter)
	if view.TotalRules != 4 {
		t.Fatalf("TotalRules = %d, want 4", view.TotalRules)
	}
	if view.ActiveCount != 3 || view.InactiveCount != 1 {
		t.Errorf("Active/Inactive = %d/%d, want 3/1", view.ActiveCount, view.InactiveCount)
	}
	if len(view.StaleActiveRules) != 1 || view.StaleActiveRules[0].RuleID != 1 {
		t.Errorf("StaleActiveRules = %+v, want only rule 1 (inactive old rule excluded)", view.StaleActiveRules)
	}
	if len(view.RecentlyChanged) != 2 {
		t.Errorf("RecentlyChanged = %d, want 2 (fresh-active + created-only fallback)", len(view.RecentlyChanged))
	}
}

func TestClassifySuppressionRulesEmpty(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	view := classifySuppressionRules(nil, now, 90*24*time.Hour)
	if view.TotalRules != 0 || len(view.StaleActiveRules) != 0 || len(view.RecentlyChanged) != 0 {
		t.Errorf("empty input should classify nothing, got %+v", view)
	}
}

func TestClassifySuppressionRulesCreatedAtFallback(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	items := rawItems(`{"ruleId": 9, "ruleName": "no-updated", "status": "active", "createdAt": "2025-01-01T00:00:00Z"}`)
	view := classifySuppressionRules(items, now, 90*24*time.Hour)
	if len(view.StaleActiveRules) != 1 {
		t.Fatalf("createdAt fallback not applied: %+v", view)
	}
	if view.StaleActiveRules[0].AgeDays < 500 {
		t.Errorf("AgeDays = %d, want >500", view.StaleActiveRules[0].AgeDays)
	}
}
