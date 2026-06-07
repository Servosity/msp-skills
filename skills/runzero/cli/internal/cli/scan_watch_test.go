// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestIsTerminalTaskStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"processed", true},
		{"Processed", true},
		{" stopped ", true},
		{"canceled", true},
		{"cancelled", true},
		{"failed", true},
		{"error", true},
		{"new", false},
		{"active", false},
		{"running", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isTerminalTaskStatus(c.status); got != c.want {
			t.Fatalf("isTerminalTaskStatus(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestOrDash(t *testing.T) {
	if orDash("") != "-" || orDash("  ") != "-" {
		t.Fatal("empty/whitespace should render as dash")
	}
	if orDash("active") != "active" {
		t.Fatal("non-empty status should pass through")
	}
}
