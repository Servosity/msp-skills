// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestAuditPagesLocal(t *testing.T) {
	tests := []struct {
		name      string
		pages     []statusPageRow
		incidents []incidentRow
		wantLen   int
	}{
		{name: "no pages", wantLen: 0},
		{
			name:    "operational page with no incidents is clean",
			pages:   []statusPageRow{{ID: "1", CompanyName: "Acme", AggregateState: "operational"}},
			wantLen: 0,
		},
		{
			name:    "non-operational page with zero open incidents is flagged",
			pages:   []statusPageRow{{ID: "1", CompanyName: "Acme", AggregateState: "downtime"}},
			wantLen: 1,
		},
		{
			name:  "non-operational page with a matching open incident is consistent",
			pages: []statusPageRow{{ID: "1", CompanyName: "Acme", AggregateState: "downtime"}},
			incidents: []incidentRow{
				{ID: "i1", Source: "m1", StartedAt: "2026-06-06T00:00:00Z"},
			},
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auditPagesLocal(tt.pages, tt.incidents)
			if len(got) != tt.wantLen {
				t.Fatalf("findings = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestAuditPageResources(t *testing.T) {
	page := statusPageRow{ID: "p1", CompanyName: "Acme", AggregateState: "operational"}
	monitors := map[string]monitorRow{
		"m1": {ID: "m1", Name: "api"},
		"m2": {ID: "m2", Name: "web", Paused: true},
	}
	open := map[string][]incidentRow{
		"m1": {{ID: "i1", Source: "m1"}},
	}
	tests := []struct {
		name     string
		entries  []pageResourceEntry
		wantLen  int
		wantKind string
	}{
		{
			name:     "operational page with open incident on backing monitor is drift",
			entries:  []pageResourceEntry{{ID: "r1", PublicName: "API", Type: "Monitor", ResourceID: "m1"}},
			wantLen:  1,
			wantKind: "drift",
		},
		{
			name:     "resource pointing at a missing monitor is a warning",
			entries:  []pageResourceEntry{{ID: "r1", PublicName: "Ghost", Type: "Monitor", ResourceID: "deleted"}},
			wantLen:  1,
			wantKind: "warning",
		},
		{
			name:     "paused monitor on a page is a warning",
			entries:  []pageResourceEntry{{ID: "r1", PublicName: "Web", Type: "Monitor", ResourceID: "m2"}},
			wantLen:  1,
			wantKind: "warning",
		},
		{
			name:    "non-monitor resources are skipped",
			entries: []pageResourceEntry{{ID: "r1", PublicName: "HB", Type: "Heartbeat", ResourceID: "hb1"}},
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auditPageResources(page, tt.entries, monitors, open)
			if len(got) != tt.wantLen {
				t.Fatalf("findings = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Severity != tt.wantKind {
				t.Errorf("severity = %s, want %s", got[0].Severity, tt.wantKind)
			}
		})
	}
}

func TestParsePageResources(t *testing.T) {
	raw := json.RawMessage(`{
		"data": [
			{"id": "r1", "attributes": {"public_name": "API", "resource_type": "Monitor", "resource_id": 12345}},
			{"id": "r2", "attributes": {"public_name": "Web", "resource_type": "Monitor", "resource_id": "67890"}}
		],
		"pagination": {"next": "https://uptime.betterstack.com/api/v2/status-pages/1/resources?page=2"}
	}`)
	entries, truncated, err := parsePageResources(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	// resource_id arrives as a JSON number or string depending on endpoint; both normalize to text
	if entries[0].ResourceID != "12345" {
		t.Errorf("numeric resource_id = %q, want 12345", entries[0].ResourceID)
	}
	if entries[1].ResourceID != "67890" {
		t.Errorf("string resource_id = %q, want 67890", entries[1].ResourceID)
	}
	if !truncated {
		t.Error("pagination.next present should report truncated=true")
	}

	big, _, err := parsePageResources(json.RawMessage(`{"data": [{"id": "r3", "attributes": {"public_name": "Big", "resource_type": "Monitor", "resource_id": 12345678}}], "pagination": {"next": null}}`))
	if err != nil {
		t.Fatalf("parse big id: %v", err)
	}
	if big[0].ResourceID != "12345678" {
		t.Errorf("7-digit resource_id = %q, want exact 12345678 (scientific notation breaks the mirror join)", big[0].ResourceID)
	}

	nullID, _, err := parsePageResources(json.RawMessage(`{"data": [{"id": "r4", "attributes": {"public_name": "NullID", "resource_type": "Monitor", "resource_id": null}}], "pagination": {"next": null}}`))
	if err != nil {
		t.Fatalf("parse null id: %v", err)
	}
	if nullID[0].ResourceID != "" {
		t.Errorf("null resource_id = %q, want empty string", nullID[0].ResourceID)
	}

	empty, truncated, err := parsePageResources(json.RawMessage(`{"data": [], "pagination": {"next": null}}`))
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(empty) != 0 || truncated {
		t.Errorf("empty page = %d entries truncated=%v, want 0/false", len(empty), truncated)
	}
}
