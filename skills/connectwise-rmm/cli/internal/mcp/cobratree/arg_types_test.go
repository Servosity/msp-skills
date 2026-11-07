// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/connectwise-rmm/handfixes.json
// under the "mcp-argument-types-enforced" entry.

package cobratree

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// TestMirroredToolRefusesMistypedArguments: the class behind Codex r1 "write
// without boolean confirmation". A JSON string must never set a bool flag (run
// --confirm, --yes, --dry-run=false), and a number flag takes only a number.
func TestMirroredToolRefusesMistypedArguments(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().String("template-id", "", "")
	cmd.Flags().StringSlice("endpoint", nil, "")
	root := &cobra.Command{Use: "cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(cmd)

	for _, tc := range []struct {
		args map[string]any
		ran  bool
	}{
		{map[string]any{"confirm": "true"}, false},
		{map[string]any{"confirm": "1"}, false},
		{map[string]any{"confirm": float64(1)}, false},
		{map[string]any{"dry-run": "false"}, false},
		{map[string]any{"limit": "5"}, false},
		{map[string]any{"endpoint": []any{true}}, false},
		{map[string]any{"confirm": true, "limit": float64(5), "template-id": "x", "endpoint": []any{"a"}}, true},
		{map[string]any{"confirm": false, "dry-run": true}, true},
		{map[string]any{"endpoint": "a"}, true},
	} {
		ran := false
		h := guardMirroredMCPCall(cmd, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			ran = true
			return mcplib.NewToolResultText("ok"), nil
		})
		res, err := h(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: tc.args}})
		if err != nil {
			t.Fatalf("%v: transport error %v", tc.args, err)
		}
		if ran != tc.ran || res.IsError == tc.ran {
			t.Fatalf("%v: ran=%v isError=%v, want ran=%v", tc.args, ran, res.IsError, tc.ran)
		}
	}
}
