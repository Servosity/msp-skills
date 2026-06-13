// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the rewst transcendence helpers. NOT generated.

package cli

import "testing"

func TestFailedExecutionStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"FAILED", true},
		{"failed", true},
		{"TIMEOUT", true},
		{"ABANDONED", true},
		{"CANCELED", true},
		{"CANCELLED", true},
		{"CANCELING", true},
		{"SUCCEEDED", false},
		{"RUNNING", false},
		{"PENDING", false},
		{"", false},
		{"  failed  ", true},
	}
	for _, c := range cases {
		if got := failedExecutionStatus(c.status); got != c.want {
			t.Errorf("failedExecutionStatus(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestParseFlexibleTime(t *testing.T) {
	if _, ok := parseFlexibleTime(""); ok {
		t.Error("empty string should not parse")
	}
	if _, ok := parseFlexibleTime("not-a-time"); ok {
		t.Error("garbage should not parse")
	}
	if _, ok := parseFlexibleTime("2026-06-12T10:00:00Z"); !ok {
		t.Error("RFC3339 should parse")
	}
	if _, ok := parseFlexibleTime("2026-06-12 10:00:00"); !ok {
		t.Error("space-separated datetime should parse")
	}
}

func TestDiffKeys(t *testing.T) {
	a := map[string]struct{}{"x": {}, "y": {}, "shared": {}}
	b := map[string]struct{}{"z": {}, "shared": {}}
	onlyA, onlyB, shared := diffKeys(a, b)
	if shared != 1 {
		t.Errorf("shared = %d, want 1", shared)
	}
	if len(onlyA) != 2 || onlyA[0] != "x" || onlyA[1] != "y" {
		t.Errorf("onlyA = %v, want sorted [x y]", onlyA)
	}
	if len(onlyB) != 1 || onlyB[0] != "z" {
		t.Errorf("onlyB = %v, want [z]", onlyB)
	}
}

func TestSinceTimestampRoundTrips(t *testing.T) {
	// A 24h cutoff must parse back as a valid RFC3339 timestamp.
	ts := sinceTimestamp(24 * 60 * 60 * 1e9)
	if _, ok := parseFlexibleTime(ts); !ok {
		t.Errorf("sinceTimestamp produced unparseable value %q", ts)
	}
}
