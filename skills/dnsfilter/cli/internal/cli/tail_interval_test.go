// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// TestTailRejectsNonPositiveInterval guards handfixes.json:
// tail-interval-positive: a zero or negative --interval is a usage error
// before any request, not a time.NewTicker panic after the first fetch.
func TestTailRejectsNonPositiveInterval(t *testing.T) {
	for _, iv := range []string{"0s", "-5s"} {
		cmd := RootCmd()
		cmd.SetArgs([]string{"tail", "networks", "--interval=" + iv})
		var out, errOut strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--interval must be greater than 0") {
			t.Fatalf("--interval=%s: want a usage error, got %v (stderr %q)", iv, err, errOut.String())
		}
	}
}
