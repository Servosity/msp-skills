// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the job wait novel feature.

package cli

import (
	"errors"
	"fmt"
	"testing"

	"nerdio-pp-cli/internal/client"
)

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

// A job poll error no retry can change ends `job wait` at once: before this a
// host-pin refusal (no NERDIO_BASE_URL) or a 404 for a wrong job id polled
// three times with two full intervals between (20 s at the default 10 s).
func TestJobWaitStopsOnAFinalError(t *testing.T) {
	for _, c := range []struct {
		err   error
		final bool
	}{
		{fmt.Errorf("GET x: %w", client.ErrHostPinRefused), true},
		{&client.APIError{StatusCode: 404}, true},
		{&client.APIError{StatusCode: 403}, true},
		{&client.APIError{StatusCode: 429}, false},
		{&client.APIError{StatusCode: 503}, false},
		{errors.New("connection reset by peer"), false},
	} {
		if got := jobPollErrorIsFinal(c.err); got != c.final {
			t.Errorf("jobPollErrorIsFinal(%v) = %v, want %v", c.err, got, c.final)
		}
	}
}
