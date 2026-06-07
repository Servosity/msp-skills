// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet billing-rollup novel feature.

package cli

import "testing"

func TestParsePeriod(t *testing.T) {
	cases := []struct {
		in        string
		start     string
		end       string
		expectErr bool
	}{
		{"2026-05-01:2026-05-31", "2026-05-01", "2026-05-31", false},
		{" 2026-01-01:2026-01-31", "2026-01-01", "2026-01-31", false},
		{"2026-05-01", "", "", true},
		{"2026-05-01:", "", "", true},
		{":2026-05-31", "", "", true},
		{"May 1:May 31", "", "", true},
		{"2026-13-01:2026-13-31", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range cases {
		start, end, err := parsePeriod(tc.in)
		if tc.expectErr {
			if err == nil {
				t.Errorf("parsePeriod(%q) expected error, got (%q,%q)", tc.in, start, end)
			}
			continue
		}
		if err != nil || start != tc.start || end != tc.end {
			t.Errorf("parsePeriod(%q) = (%q,%q,%v), want (%q,%q,nil)", tc.in, start, end, err, tc.start, tc.end)
		}
	}
}

func TestBillingRollupCommandShape(t *testing.T) {
	cmd := newNovelFleetBillingRollupCmd(&rootFlags{})
	for _, flag := range []string{"period", "unpaid-only"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s flag", flag)
		}
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("billing-rollup is a read aggregate and must be mcp:read-only")
	}
}
