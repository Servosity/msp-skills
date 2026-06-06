// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
	"time"
)

func auditExclusionFixtures() []map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().Add(-400 * 24 * time.Hour).UTC().Format(time.RFC3339)
	return []map[string]any{
		// wildcard path
		{"id": "e1", "value": `C:\temp\*`, "type": "path", "createdAt": now},
		// stale (400 days old), non-wildcard, no path match
		{"id": "e2", "value": `C:\fixed\app.exe`, "type": "path", "createdAt": old},
		// hash matching a seeded threat sha1 (should NOT be never-matched)
		{"id": "e3", "value": "matchhash", "type": "white_hash", "createdAt": now},
		// hash matching nothing (should be never-matched)
		{"id": "e4", "value": "orphanhash", "type": "white_hash", "createdAt": now},
	}
}

func findingsFor(items []any, value string) []string {
	for _, it := range items {
		m := it.(map[string]any)
		if m["value"] == value {
			raw, _ := m["findings"].([]any)
			out := make([]string, 0, len(raw))
			for _, f := range raw {
				out = append(out, f.(string))
			}
			return out
		}
	}
	return nil
}

func has(findings []string, want string) bool {
	for _, f := range findings {
		if f == want {
			return true
		}
	}
	return false
}

func TestExclusionsAuditFlags(t *testing.T) {
	threats := []map[string]any{
		threatFix("t1", "Known", "matchhash", "HOST-A", "SiteA", nil),
	}
	db := newSeededStore(t, nil, threats)
	seedResource(t, db, "exclusions", auditExclusionFixtures())

	out := runJSON(t, newNovelExclusionsAuditCmd(jsonFlags()), "--db", db)

	if sc, _ := out["exclusions_scanned"].(float64); sc != 4 {
		t.Fatalf("exclusions_scanned = %v, want 4", out["exclusions_scanned"])
	}
	items := out["items"].([]any)

	if wc := findingsFor(items, `C:\temp\*`); !has(wc, "wildcard-path") {
		t.Fatalf("wildcard path not flagged wildcard-path: %v", wc)
	}
	if st := findingsFor(items, `C:\fixed\app.exe`); !has(st, "stale") {
		t.Fatalf("400-day-old exclusion not flagged stale: %v", st)
	}
	// Hash that matches a seeded threat must NOT be never-matched.
	if mh := findingsFor(items, "matchhash"); has(mh, "never-matched") {
		t.Fatalf("matching hash wrongly flagged never-matched: %v", mh)
	}
	// Hash that matches nothing must be never-matched.
	if oh := findingsFor(items, "orphanhash"); !has(oh, "never-matched") {
		t.Fatalf("orphan hash not flagged never-matched: %v", oh)
	}
}

func TestExclusionsAuditEmptyThreatStoreSkipsMatch(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	seedResource(t, db, "exclusions", auditExclusionFixtures())

	out := runJSON(t, newNovelExclusionsAuditCmd(jsonFlags()), "--db", db)

	note, _ := out["note"].(string)
	if !strings.Contains(strings.ToLower(note), "skipped") {
		t.Fatalf("expected note explaining never-matched was skipped, got %q", note)
	}
	items := out["items"].([]any)
	// With no threats, the orphan hash must NOT be flagged never-matched.
	if oh := findingsFor(items, "orphanhash"); has(oh, "never-matched") {
		t.Fatalf("never-matched should be skipped with empty threat store: %v", oh)
	}
}

func TestExclusionsAuditWildcardLeadingPathNotNeverMatched(t *testing.T) {
	// "*.tmp" has no literal prefix to match on, so never-matched is
	// unprovable and must not be asserted — even when no threat matches.
	now := time.Now().UTC().Format(time.RFC3339)
	threats := []map[string]any{
		threatFix("t1", "Known", "matchhash", "HOST-A", "SiteA", nil),
	}
	db := newSeededStore(t, nil, threats)
	seedResource(t, db, "exclusions", []map[string]any{
		{"id": "w1", "value": "*.tmp", "type": "path", "createdAt": now},
	})

	out := runJSON(t, newNovelExclusionsAuditCmd(jsonFlags()), "--db", db)
	items, _ := out["items"].([]any)
	f := findingsFor(items, "*.tmp")
	if has(f, "never-matched") {
		t.Fatalf("wildcard-leading path flagged never-matched: %v", f)
	}
	if !has(f, "wildcard-path") {
		t.Fatalf("wildcard-leading path missing wildcard-path finding: %v", f)
	}
}
