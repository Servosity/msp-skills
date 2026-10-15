// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json mcp-bool-false-default-true.

package cobratree

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestKeepExplicitFalseReachesDefaultTrueFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "tail"}
	cmd.Flags().Bool("follow", true, "")
	cmd.Flags().Bool("json", false, "")
	args := map[string]any{"follow": false, "json": false, "resource": "companies"}
	keepExplicitFalse(cmd, args)
	got := cliArgsFromMCP(args, nil)
	want := map[string]bool{"--follow=false": true, "--resource=companies": true}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want exactly %v", got, want)
	}
	for _, a := range got {
		if !want[a] {
			t.Fatalf("argv = %v, want exactly %v", got, want)
		}
	}
}
