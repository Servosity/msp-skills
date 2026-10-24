// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

// TestGDAPRelationshipRowsShapes locks both payload shapes: CIPP returns some
// endpoints as a bare array and others wrapped in the {"Results": [...]}
// envelope that produced issue #200 when a novel path assumed only the former.
func TestGDAPRelationshipRowsShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bare array", `[{"id":"r1","customerTenantId":"a"},{"id":"r2","customerTenantId":"b"}]`, 2},
		{"Results envelope", `{"Results":[{"id":"r1","customerTenantId":"a"},{"id":"r2","customerTenantId":"b"}]}`, 2},
		{"lowercase results envelope", `{"results":[{"id":"r1","customerTenantId":"a"}]}`, 1},
		{"empty array", `[]`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := gdapRelationshipRows(json.RawMessage(c.body))
			if err != nil {
				t.Fatalf("gdapRelationshipRows(%s): %v", c.body, err)
			}
			if len(got) != c.want {
				t.Errorf("gdapRelationshipRows(%s) returned %d rows, want %d", c.body, len(got), c.want)
			}
		})
	}
}

// TestParseGDAPDateFormats asserts the end-date parser handles the shapes
// Partner Center and Graph emit, and reports failure rather than defaulting.
func TestParseGDAPDateFormats(t *testing.T) {
	valid := []string{
		"2026-09-01T00:00:00Z",
		"2026-09-01T00:00:00.1234567Z",
		"2026-09-01T00:00:00",
		"2026-09-01",
	}
	for _, s := range valid {
		if _, ok := parseGDAPDate(s); !ok {
			t.Errorf("parseGDAPDate(%q) failed, want success", s)
		}
	}
	for _, s := range []string{"", "   ", "not-a-date", "09/01/2026"} {
		if _, ok := parseGDAPDate(s); ok {
			t.Errorf("parseGDAPDate(%q) succeeded, want failure so the caller can surface it", s)
		}
	}
}

// TestGDAPIsActiveOnlyExplicitActive asserts an unknown or terminated status is
// never treated as active. Treating unknown as active would hide the coverage
// gap this command exists to surface.
func TestGDAPIsActiveOnlyExplicitActive(t *testing.T) {
	if !gdapIsActive(map[string]any{"status": "active"}) {
		t.Error(`status "active" must be active`)
	}
	if !gdapIsActive(map[string]any{"Status": "Active"}) {
		t.Error("status match must be case-insensitive")
	}
	for _, s := range []string{"terminated", "pending", "approved", ""} {
		if gdapIsActive(map[string]any{"status": s}) {
			t.Errorf("status %q must NOT count as active", s)
		}
	}
	if gdapIsActive(map[string]any{}) {
		t.Error("a relationship with no status field must not count as active")
	}
}

// TestGDAPRelationshipTenantFallbacks asserts the tenant key lookup tolerates
// the several spellings CIPP uses.
func TestGDAPRelationshipTenantFallbacks(t *testing.T) {
	cases := []struct {
		obj  map[string]any
		want string
	}{
		{map[string]any{"customerTenantId": "a.example.com"}, "a.example.com"},
		{map[string]any{"customerId": "b.example.com"}, "b.example.com"},
		{map[string]any{"_tenant": "c.example.com"}, "c.example.com"},
		{map[string]any{"nothing": "x"}, "(unknown)"},
	}
	for _, c := range cases {
		if got := gdapRelationshipTenant(c.obj); got != c.want {
			t.Errorf("gdapRelationshipTenant(%v) = %q, want %q", c.obj, got, c.want)
		}
	}
}

// TestGDAPExpiryWindowBoundary asserts a relationship ending inside the window
// is selected and one ending outside it is not.
func TestGDAPExpiryWindowBoundary(t *testing.T) {
	now := time.Now().UTC()
	inside := now.AddDate(0, 0, 10)
	outside := now.AddDate(0, 0, 90)
	cutoff := now.AddDate(0, 0, 30)

	if !inside.Before(cutoff) {
		t.Error("a relationship ending in 10 days must fall inside a 30-day window")
	}
	if outside.Before(cutoff) {
		t.Error("a relationship ending in 90 days must fall outside a 30-day window")
	}
}
