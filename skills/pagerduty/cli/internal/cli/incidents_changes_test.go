// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestBuildIncidentChangesRanksClosestFirst(t *testing.T) {
	trigger := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	incidents := []map[string]any{
		{"id": "PT4KHLK", "created_at": rfc(trigger), "service": ref("PSVC1", "Checkout API")},
	}
	changes := []map[string]any{
		{"timestamp": rfc(trigger.Add(-10 * time.Minute)), "summary": "Deploy v2.3.1", "source": "GitHub", "services": []any{ref("PSVC1", "Checkout API")}},
		{"timestamp": rfc(trigger.Add(-90 * time.Minute)), "summary": "Config rollout", "source": "LaunchDarkly", "services": []any{ref("PSVC1", "Checkout API")}},
		{"timestamp": rfc(trigger.Add(-5 * time.Minute)), "summary": "Other service deploy", "source": "GitHub", "services": []any{ref("PSVC9", "Other")}},
		{"timestamp": rfc(trigger.Add(-2 * time.Minute)), "summary": "Account-wide infra change", "source": "Terraform", "services": []any{}},
		{"timestamp": rfc(trigger.Add(30 * time.Minute)), "summary": "After the trigger", "source": "GitHub", "services": []any{ref("PSVC1", "Checkout API")}},
	}

	res := buildIncidentChanges(incidents, changes, "PT4KHLK", 2*time.Hour)

	if res.TriggeredAt != rfc(trigger) {
		t.Fatalf("triggered_at = %q", res.TriggeredAt)
	}
	if res.ServiceID != "PSVC1" || res.Service != "Checkout API" {
		t.Fatalf("service = %q/%q", res.ServiceID, res.Service)
	}
	if len(res.Changes) != 3 {
		t.Fatalf("expected 3 correlated changes (same-service x2 + account-wide), got %d: %+v", len(res.Changes), res.Changes)
	}
	// Closest to trigger first.
	if res.Changes[0].Summary != "Account-wide infra change" || !res.Changes[0].AccountWide {
		t.Errorf("closest change = %+v, want account-wide infra change", res.Changes[0])
	}
	if res.Changes[1].Summary != "Deploy v2.3.1" {
		t.Errorf("second change = %q, want Deploy v2.3.1", res.Changes[1].Summary)
	}
	if res.Changes[1].Source != "GitHub" {
		t.Errorf("source = %q, want GitHub (source must survive into the result)", res.Changes[1].Source)
	}
	if res.Changes[2].Summary != "Config rollout" {
		t.Errorf("third change = %q, want Config rollout", res.Changes[2].Summary)
	}
	for _, c := range res.Changes {
		if c.Summary == "Other service deploy" {
			t.Errorf("change on unrelated service leaked into results")
		}
		if c.Summary == "After the trigger" {
			t.Errorf("post-trigger change leaked into results")
		}
	}
}

func TestBuildIncidentChangesUnknownIncident(t *testing.T) {
	res := buildIncidentChanges(nil, nil, "PNOPE", time.Hour)
	if res.TriggeredAt != "" {
		t.Fatalf("expected empty triggered_at for unknown incident, got %q", res.TriggeredAt)
	}
	if len(res.Changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(res.Changes))
	}
}

func TestBuildIncidentChangesEmptyWindowNote(t *testing.T) {
	trigger := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	incidents := []map[string]any{
		{"id": "PT4KHLK", "created_at": rfc(trigger), "service": ref("PSVC1", "Checkout API")},
	}
	changes := []map[string]any{
		{"timestamp": rfc(trigger.Add(-11 * time.Hour)), "summary": "Way too early", "services": []any{ref("PSVC1", "Checkout API")}},
	}
	res := buildIncidentChanges(incidents, changes, "PT4KHLK", 30*time.Minute)
	if len(res.Changes) != 0 {
		t.Fatalf("expected no changes inside 30m window, got %d", len(res.Changes))
	}
	if res.Note == "" {
		t.Errorf("expected a widen-the-window note on empty result")
	}
}

func TestBuildIncidentChangesCreatedAtFallback(t *testing.T) {
	trigger := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	incidents := []map[string]any{
		{"id": "PT4KHLK", "created_at": rfc(trigger), "service": ref("PSVC1", "Checkout API")},
	}
	changes := []map[string]any{
		// Payload variant carrying created_at instead of timestamp.
		{"created_at": rfc(trigger.Add(-15 * time.Minute)), "summary": "Deploy via created_at", "services": []any{ref("PSVC1", "Checkout API")}},
	}
	res := buildIncidentChanges(incidents, changes, "PT4KHLK", time.Hour)
	if len(res.Changes) != 1 || res.Changes[0].Summary != "Deploy via created_at" {
		t.Fatalf("created_at fallback failed: %+v", res.Changes)
	}
}
