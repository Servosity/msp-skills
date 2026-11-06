// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
	"time"
)

// TestAccrueBlock pins the contract-block fields shared by contract-burn,
// reconcile, account-brief, and retainer: ContractBlockModel (spec.json)
// documents hours (purchased) and hoursApproved (used); remaining is derived.
// The undocumented hoursLeft / hoursRemaining / hoursUsed forms are not read
// (Codex r1 class wrong-field-read-from-a-documented-response).
func TestAccrueBlock(t *testing.T) {
	cases := []struct {
		name                       string
		block                      map[string]any
		purchased, used, remaining float64
	}{
		{"hoursApproved", map[string]any{"hours": 40.0, "hoursApproved": 10.0}, 40, 10, 30},
		{"over-consumed", map[string]any{"hours": 20.0, "hoursApproved": 25.0}, 20, 25, -5},
		{"undocumented forms ignored", map[string]any{"hours": 10.0, "hoursLeft": 4.0, "hoursUsed": 9.0, "hoursApproved": 2.0}, 10, 2, 8},
		{"neither side reported", map[string]any{"hours": 8.0}, 8, 0, 0},
		{"numeric strings (store decode)", map[string]any{"hours": "12", "hoursApproved": "3"}, 12, 3, 9},
		{"empty block", map[string]any{}, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, u, r := accrueBlock(tc.block)
			if p != tc.purchased || u != tc.used || r != tc.remaining {
				t.Errorf("accrueBlock(%v) = (%v, %v, %v), want (%v, %v, %v)",
					tc.block, p, u, r, tc.purchased, tc.used, tc.remaining)
			}
		})
	}
}

// TestParseNovelDuration pins the Autotask-friendly duration forms the novel
// commands accept (since, account-brief --since).
func TestParseNovelDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"24h", 24 * time.Hour, true},
		{"7d", 7 * 24 * time.Hour, true},
		{"2w", 14 * 24 * time.Hour, true},
		{"30m", 30 * time.Minute, true},
		{"14", 14 * 24 * time.Hour, true},
		{"", 0, false},
		{"xyz", 0, false},
		{"d", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseNovelDuration(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseNovelDuration(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestNormalizeEntityName pins the picklist entity-name normalization
// (kebab/lower → Pascal path segment, Pascal → kebab store key).
func TestNormalizeEntityName(t *testing.T) {
	cases := []struct {
		in, pascal, kebab string
	}{
		{"tickets", "Tickets", "tickets"},
		{"Tickets", "Tickets", "tickets"},
		{"time-entries", "TimeEntries", "time-entries"},
		{"TimeEntries", "TimeEntries", "time-entries"},
		{"", "", ""},
	}
	for _, tc := range cases {
		p, k := normalizeEntityName(tc.in)
		if p != tc.pascal || k != tc.kebab {
			t.Errorf("normalizeEntityName(%q) = (%q, %q), want (%q, %q)", tc.in, p, k, tc.pascal, tc.kebab)
		}
	}
}
