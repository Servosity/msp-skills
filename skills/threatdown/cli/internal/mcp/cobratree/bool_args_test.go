// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// TestMirroredBoolFlagsTakeOnlyJSONBooleans: a mirrored tool's boolean flag
// (confirm, all-matches, ...) is set only by a JSON boolean. The string "true"
// or a number used to become --confirm=true on the child, so
// {"confirm":"true"} sent a respond job that previews until boolean true.
// Hand-fix (handfixes.json mcp-bool-args-json-boolean-only).
func TestMirroredBoolFlagsTakeOnlyJSONBooleans(t *testing.T) {
	root := &cobra.Command{Use: "threatdown-cli"}
	root.PersistentFlags().Bool("all-matches", false, "")
	cmd := &cobra.Command{Use: "isolate", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().StringSlice("host", nil, "")
	root.AddCommand(cmd)
	calls := 0
	h := guardMirroredMCPCall(cmd, func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		calls++
		return mcplib.NewToolResultText("sent"), nil
	})
	call := func(args map[string]any) *mcplib.CallToolResult {
		var req mcplib.CallToolRequest
		req.Params.Arguments = args
		res, err := h(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, bad := range []map[string]any{
		{"confirm": "true"}, {"confirm": "TRUE"}, {"confirm": float64(1)}, {"confirm": []any{true}},
		{"all-matches": "true", "confirm": true}, {"confirm": map[string]any{"v": true}},
	} {
		if res := call(bad); res == nil || !res.IsError {
			t.Fatalf("%v: a non-boolean value reached the CLI as a boolean flag", bad)
		}
	}
	if calls != 0 {
		t.Fatalf("handler ran %d time(s) for non-boolean flag values", calls)
	}
	if res := call(map[string]any{"confirm": true, "host": []any{"HOST1"}, "all-matches": false}); res.IsError || calls != 1 {
		t.Fatalf("boolean true was refused: %#v (calls=%d)", res, calls)
	}
}
