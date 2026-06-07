// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the job wait novel feature.

package cli

import "testing"

func TestJobTerminal(t *testing.T) {
	cases := []struct {
		status    string
		terminal  bool
		succeeded bool
	}{
		{"Completed", true, true},
		{"completed", true, true},
		{"Succeeded", true, true},
		{"CompletedWithErrors", true, false},
		{"Failed", true, false},
		{"Cancelled", true, false},
		{"Canceled", true, false},
		{"Error", true, false},
		{"Running", false, false},
		{"InProgress", false, false},
		{"Pending", false, false},
		{"", false, false},
		{"  Completed  ", true, true},
	}
	for _, tc := range cases {
		terminal, succeeded := jobTerminal(tc.status)
		if terminal != tc.terminal || succeeded != tc.succeeded {
			t.Errorf("jobTerminal(%q) = (%v,%v), want (%v,%v)", tc.status, terminal, succeeded, tc.terminal, tc.succeeded)
		}
	}
}

func TestJobWaitCommandShape(t *testing.T) {
	cmd := newNovelJobWaitCmd(&rootFlags{})
	if cmd.Use != "wait <job_id>" {
		t.Errorf("Use = %q, want %q", cmd.Use, "wait <job_id>")
	}
	for _, flag := range []string{"interval", "timeout"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s flag", flag)
		}
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("job wait must be annotated mcp:read-only")
	}
}
