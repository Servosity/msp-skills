// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: an MCP true is only a value for a boolean flag.

package cobratree

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/spf13/cobra"
)

func TestBoolForNonBoolFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "set"}
	cmd.Flags().Int("seats", 0, "")
	cmd.Flags().String("friendly-name", "", "")
	cmd.Flags().Bool("confirm", false, "")
	for _, bad := range []map[string]any{{"seats": true}, {"friendly-name": true, "confirm": true}} {
		if boolForNonBoolFlag(cmd, bad) == "" {
			t.Errorf("must refuse %v", bad)
		}
	}
	for _, ok := range []map[string]any{{"confirm": true}, {"seats": 7.0, "confirm": true}, {"seats": false}, {"unknown": true}} {
		if msg := boolForNonBoolFlag(cmd, ok); msg != "" {
			t.Errorf("must allow %v: %s", ok, msg)
		}
	}
}

// The guard is wired into every mirrored tool: true for a non-boolean flag
// never reaches the shell-out (the next handler).
func TestGuardMirroredMCPCallRefusesTrueForNonBoolFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "set"}
	cmd.Flags().Int("seats", 0, "")
	cmd.Flags().Bool("confirm", false, "")
	called := 0
	handler := guardMirroredMCPCall(cmd, func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		called++
		return mcplib.NewToolResultText("ran"), nil
	})
	call := func(args map[string]any) *mcplib.CallToolResult {
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = args
		res, err := handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := call(map[string]any{"seats": true, "confirm": true}); !res.IsError || called != 0 {
		t.Fatalf("seats:true must be refused before the shell-out: isError=%v called=%d", res.IsError, called)
	}
	if res := call(map[string]any{"seats": 7.0, "confirm": true}); res.IsError || called != 1 {
		t.Fatalf("a real count must reach the shell-out: isError=%v called=%d", res.IsError, called)
	}
}
