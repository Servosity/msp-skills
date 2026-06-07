// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the triage novel command.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func rawItems(s ...string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(s))
	for _, v := range s {
		out = append(out, json.RawMessage(v))
	}
	return out
}

func TestParseEnvelope(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantItems int
		wantTotal int64
	}{
		{"envelope with data and totalCount", `{"totalCount": 7, "currentPage": 1, "data": [{"a":1},{"a":2}]}`, 2, 7},
		{"bare array", `[{"a":1},{"a":2},{"a":3}]`, 3, 3},
		{"envelope without totalCount", `{"data": [{"a":1}]}`, 1, 1},
		{"empty object", `{}`, 0, 0},
		{"invalid json", `not-json`, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, total := parseEnvelope(json.RawMessage(tt.input))
			if len(items) != tt.wantItems {
				t.Errorf("items = %d, want %d", len(items), tt.wantItems)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestParseTriageIncidents(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)
	items := rawItems(
		`{"id": 11, "title": "Webroot Detection", "status": "open", "createdAt": "2026-06-06T03:00:00.000Z"}`,
		`{"id": "abc", "title": "Old incident", "status": "open", "createdAt": "2026-05-01T03:00:00.000Z"}`,
		`{"title": "No timestamps", "status": "open"}`,
	)
	rows, recent := parseTriageIncidents(items, cutoff, now)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if recent != 1 {
		t.Errorf("recent = %d, want 1 (only the 2026-06-06 incident is within 24h)", recent)
	}
	if rows[0].Title != "Webroot Detection" {
		t.Errorf("rows not sorted newest-first: got %q first", rows[0].Title)
	}
	if rows[0].ID != "11" {
		t.Errorf("numeric id not normalized: got %q", rows[0].ID)
	}
	if rows[0].AgeDays != 0 {
		t.Errorf("AgeDays = %d, want 0", rows[0].AgeDays)
	}
}

func TestParseTriageIncidentsMixedOffsets(t *testing.T) {
	// +00:00 vs Z wire formats must order chronologically, and rows without
	// a parseable timestamp must sort last, not first.
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)
	items := rawItems(
		`{"id": 1, "title": "older-offset-format", "status": "open", "createdAt": "2026-06-05T00:00:00+00:00"}`,
		`{"id": 2, "title": "newer-z-format", "status": "open", "createdAt": "2026-06-06T03:00:00Z"}`,
		`{"id": 3, "title": "no-timestamp", "status": "open"}`,
	)
	rows, _ := parseTriageIncidents(items, cutoff, now)
	if rows[0].Title != "newer-z-format" {
		t.Errorf("mixed-offset sort broken: got %q first", rows[0].Title)
	}
	if rows[2].Title != "no-timestamp" {
		t.Errorf("timestamp-less row must sort last, got %q last", rows[2].Title)
	}
}

func TestParseTriageAgentsMissingTimestampSortsLast(t *testing.T) {
	items := rawItems(
		`{"hostname": "NO-TS", "connectivity": "offline"}`,
		`{"hostname": "SILENT", "connectivity": "offline", "lastConnected": "2026-01-01T00:00:00+00:00"}`,
		`{"hostname": "RECENT", "connectivity": "offline", "lastConnected": "2026-06-01T00:00:00Z"}`,
	)
	rows := parseTriageAgents(items)
	if rows[0].Hostname != "SILENT" {
		t.Errorf("longest-silent must lead: got %q", rows[0].Hostname)
	}
	if rows[2].Hostname != "NO-TS" {
		t.Errorf("timestamp-less agent must sort last, got %q last", rows[2].Hostname)
	}
}

func TestParseTriageAgents(t *testing.T) {
	items := rawItems(
		`{"hostname": "HOST-B", "customerId": 22, "connectivity": "offline", "lastConnected": "2026-06-01T00:00:00.000Z"}`,
		`{"hostname": "HOST-A", "customerId": "11", "connectivity": "offline", "lastConnected": "2026-04-01T00:00:00.000Z"}`,
	)
	rows := parseTriageAgents(items)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Hostname != "HOST-A" {
		t.Errorf("rows not sorted longest-silent first: got %q first", rows[0].Hostname)
	}
	if rows[0].CustomerID != 11 {
		t.Errorf("string-encoded customerId not extracted: got %d", rows[0].CustomerID)
	}
}
