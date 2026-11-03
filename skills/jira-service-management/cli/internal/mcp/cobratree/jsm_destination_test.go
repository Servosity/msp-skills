// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Handfix jsm-mcp-no-site-selection: a path-context argument sent anyway (not
// in the schema) never reaches the child CLI, on any command.
func TestPathContextNeverReachesTheChild(t *testing.T) {
	root := &cobra.Command{Use: "jira-service-management-cli"}
	for _, use := range []string{"sync", "workflow"} {
		cmd := &cobra.Command{Use: use, RunE: func(*cobra.Command, []string) error { return nil }}
		cmd.Flags().StringArray("path-context", nil, "")
		root.AddCommand(cmd)
		args := cliArgsFromMCP(map[string]any{"path-context": []any{"site=beta"}, "resources": "servicedesk"}, blockedStructuredArgsForCommand(cmd))
		if strings.Contains(strings.Join(args, " "), "path-context") {
			t.Errorf("%s: path-context reached the child: %v", use, args)
		}
	}
}
