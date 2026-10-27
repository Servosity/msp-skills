// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
)

// TestCABaselineSetThreshold asserts the baseline is a share of tenants, not a
// strict intersection, so one misconfigured tenant cannot empty it.
func TestCABaselineSetThreshold(t *testing.T) {
	byTenant := map[string]map[string]bool{
		"a.example.com": {"Require MFA": true, "Block Legacy Auth": true},
		"b.example.com": {"Require MFA": true, "Block Legacy Auth": true},
		"c.example.com": {"Require MFA": true, "Block Legacy Auth": true},
		"d.example.com": {"Require MFA": true}, // missing Block Legacy Auth
	}
	baseline := caBaselineSet(byTenant, caDefaultThreshold)
	if !baseline["Require MFA"] {
		t.Error("Require MFA (4/4 tenants) must be in the baseline")
	}
	if !baseline["Block Legacy Auth"] {
		t.Error("Block Legacy Auth (3/4, a majority) must be in the default baseline")
	}

	// A near-intersection threshold drops Block Legacy Auth, hiding exactly
	// the deviation this command exists to surface.
	strict := caBaselineSet(byTenant, 0.9)
	if strict["Block Legacy Auth"] {
		t.Error("at threshold 0.9 Block Legacy Auth must fall out of the baseline")
	}
	// Three tenants: a policy two of them hold is baseline (the old 0.8 made
	// it a strict intersection and flagged the hardened tenants as "extra").
	three := map[string]map[string]bool{
		"a": {"P1": true, "P2": true}, "b": {"P1": true, "P2": true}, "c": {"P1": true},
	}
	if !caBaselineSet(three, caDefaultThreshold)["P2"] {
		t.Error("P2 (2/3) must be baseline on a three-tenant fleet")
	}
}

// TestCABaselineSetEmpty asserts no panic and an empty baseline for no tenants.
func TestCABaselineSetEmpty(t *testing.T) {
	if got := caBaselineSet(map[string]map[string]bool{}, 0.8); len(got) != 0 {
		t.Errorf("empty input should yield an empty baseline, got %v", got)
	}
}

// TestCAPolicyNameFallbacks asserts the tolerant display-name lookup, since CIPP
// and Graph disagree on casing across entrypoints.
func TestCAPolicyNameFallbacks(t *testing.T) {
	cases := []struct {
		obj  map[string]any
		want string
	}{
		{map[string]any{"displayName": "Require MFA"}, "Require MFA"},
		{map[string]any{"DisplayName": "Block Legacy"}, "Block Legacy"},
		{map[string]any{"name": "Trusted Locations"}, "Trusted Locations"},
		{map[string]any{"displayName": "  Padded  "}, "Padded"},
		{map[string]any{"irrelevant": "x"}, ""},
		{map[string]any{"displayName": ""}, ""},
	}
	for _, c := range cases {
		if got := caPolicyName(c.obj); got != c.want {
			t.Errorf("caPolicyName(%v) = %q, want %q", c.obj, got, c.want)
		}
	}
}

// TestCADiffMissingAndExtra walks the real deviation computation.
func TestCADiffMissingAndExtra(t *testing.T) {
	byTenant := map[string]map[string]bool{
		"a.example.com": {"Require MFA": true, "Block Legacy Auth": true},
		"b.example.com": {"Require MFA": true, "Block Legacy Auth": true},
		"c.example.com": {"Require MFA": true, "Shadow Policy": true},
	}
	baseline := caBaselineSet(byTenant, 0.6)

	// c is missing Block Legacy Auth (2/3 = 0.67 >= 0.6) and carries an extra.
	missing := []string{}
	for name := range baseline {
		if !byTenant["c.example.com"][name] {
			missing = append(missing, name)
		}
	}
	if len(missing) != 1 || missing[0] != "Block Legacy Auth" {
		t.Errorf("expected c to be missing [Block Legacy Auth], got %v", missing)
	}
	if baseline["Shadow Policy"] {
		t.Error("Shadow Policy (1/3) must not be in the baseline, so it counts as extra")
	}
}
