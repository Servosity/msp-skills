// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestAgingBucketLabelBoundaries pins the bucket cutoffs AND the contract that
// every produced label is a member of the ordered display list — the render
// loop iterates agingBucketLabels, so a produced-but-unlisted label silently
// vanishes from output (the exact regression this test was written to catch).
func TestAgingBucketLabelBoundaries(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, "0-7d"},
		{7, "0-7d"},
		{8, "8-30d"},
		{30, "8-30d"},
		{31, "31-90d"},
		{90, "31-90d"},
		{91, "91d+"},
		{365, "91d+"},
	}
	listed := map[string]bool{}
	for _, lbl := range agingBucketLabels {
		listed[lbl] = true
	}
	for _, tt := range tests {
		got := agingBucketLabel(tt.days)
		if got != tt.want {
			t.Errorf("agingBucketLabel(%d) = %q, want %q", tt.days, got, tt.want)
		}
		if !listed[got] {
			t.Errorf("agingBucketLabel(%d) = %q is not in agingBucketLabels %v — it would be dropped from rendered output", tt.days, got, agingBucketLabels)
		}
	}
}
