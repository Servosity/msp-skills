// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// cisco-meraki handfix meraki-mcp-mirror-write-confirm: the gate decides what reaches
// the child CLI. Each case records the arguments the mirrored handler got (nil
// when it never ran).
func TestMirroredWritesSendOnlyWithBooleanConfirm(t *testing.T) {
	root := &cobra.Command{Use: "cisco-meraki-cli"}
	agents := &cobra.Command{Use: "fleet"}
	bulk := &cobra.Command{Use: "push", Annotations: map[string]string{"mcp:read-only-unless-flags": "apply"}, RunE: func(*cobra.Command, []string) error { return nil }}
	bulk.Flags().Bool("apply", false, "")
	bulk.Flags().String("command", "", "")
	imp := &cobra.Command{Use: "import", RunE: func(*cobra.Command, []string) error { return nil }}
	syncCmd := &cobra.Command{Use: "sync", RunE: func(*cobra.Command, []string) error { return nil }}
	agents.AddCommand(bulk)
	root.AddCommand(agents, imp, syncCmd)

	var got map[string]any
	ran := false
	record := func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		ran, got = true, req.GetArguments()
		return mcplib.NewToolResultText("ran"), nil
	}
	s := server.NewMCPServer("t", "1")
	for name, path := range map[string]string{"fleet_push": "fleet push", "import": "import", "sync": "sync"} {
		tool := mcplib.NewTool(name)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, record)
	}
	RequireConfirmForMirroredWrites(s, root)
	tools := s.ListTools()
	call := func(name string, args map[string]any) {
		t.Helper()
		ran, got = false, nil
		req := mcplib.CallToolRequest{}
		req.Params.Name, req.Params.Arguments = name, args
		if _, err := tools[name].Handler(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}

	for _, v := range []any{true, "true", 1, false} {
		call("fleet_push", map[string]any{"apply": v, "command": "whoami"})
		if _, sent := got["apply"]; !ran || sent {
			t.Errorf("apply=%#v without confirm: ran=%v args=%v, want a preview without apply", v, ran, got)
		}
		call("fleet_push", map[string]any{"apply": v, "command": "whoami", "confirm": "true"})
		if _, sent := got["apply"]; sent {
			t.Errorf("apply=%#v with string confirm sent: %v", v, got)
		}
	}
	call("fleet_push", map[string]any{"apply": true, "command": "whoami", "confirm": true})
	if got["apply"] != true || got["confirm"] != nil {
		t.Errorf("confirm true: args=%v, want apply kept and confirm dropped", got)
	}
	call("fleet_push", map[string]any{"command": "whoami"})
	if !ran || got["command"] != "whoami" {
		t.Errorf("preview without --apply should run as mirrored: ran=%v args=%v", ran, got)
	}

	for _, args := range []map[string]any{{"batch-size": 5}, {"batch-size": 5, "confirm": 1}, {"batch-size": 5, "dry-run": false, "confirm": "true"}} {
		call("import", args)
		if !ran || got["dry-run"] != true || got["confirm"] != nil {
			t.Errorf("import %v without boolean confirm: ran=%v args=%v, want a --dry-run preview", args, ran, got)
		}
	}
	call("import", map[string]any{"batch-size": 5, "confirm": true})
	if !ran || got["confirm"] != nil || got["dry-run"] != nil {
		t.Errorf("import with confirm true: ran=%v args=%v", ran, got)
	}

	if _, gated := tools["sync"].Tool.InputSchema.Properties["confirm"]; gated {
		t.Error("sync writes only the local store and must not need confirm")
	}
}
