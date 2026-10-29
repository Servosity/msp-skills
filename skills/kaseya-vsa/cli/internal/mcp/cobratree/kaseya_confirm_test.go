// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func confirmTestServer(t *testing.T, calls *[]map[string]any) (*server.MCPServer, *cobra.Command) {
	t.Helper()
	root := &cobra.Command{Use: "kaseya-vsa-cli"}
	scripts := &cobra.Command{Use: "scripts"}
	scripts.AddCommand(&cobra.Command{Use: "run-wait", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(scripts)
	root.AddCommand(&cobra.Command{Use: "sync", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "health", Run: func(*cobra.Command, []string) {}})

	s := server.NewMCPServer("confirm-test", "test")
	add := func(name, path string, readOnly bool) {
		opts := []mcplib.ToolOption{mcplib.WithString("device")}
		if readOnly {
			opts = append(opts, mcplib.WithReadOnlyHintAnnotation(true))
		}
		tool := mcplib.NewTool(name, opts...)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			*calls = append(*calls, req.GetArguments())
			return mcplib.NewToolResultText("ran"), nil
		})
	}
	add("scripts_run_wait", "scripts run-wait", false)
	add("sync", "sync", false)
	add("teach", "teach", false)
	add("health", "health", true)
	add("unknown_write", "no such command", false)
	return s, root
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) string {
	t.Helper()
	entry := s.GetTool(name)
	if entry == nil {
		t.Fatalf("tool %s not registered", name)
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := entry.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMirroredWriteNeedsBooleanConfirm(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root)

	for _, name := range []string{"scripts_run_wait", "unknown_write"} {
		props := s.GetTool(name).Tool.InputSchema.Properties
		if _, ok := props["confirm"]; !ok {
			t.Fatalf("%s: no confirm parameter", name)
		}
		for _, confirm := range []any{nil, false, "true", 1.0} {
			args := map[string]any{"device": "PC-1"}
			if confirm != nil {
				args["confirm"] = confirm
			}
			out := callTool(t, s, name, args)
			if !strings.Contains(out, `"confirmation_required": true`) {
				t.Fatalf("%s confirm=%v: want preview, got %s", name, confirm, out)
			}
		}
	}
	if len(calls) != 0 {
		t.Fatalf("a write ran without confirm: %v", calls)
	}

	if out := callTool(t, s, "scripts_run_wait", map[string]any{"device": "PC-1", "confirm": true}); out != "ran" {
		t.Fatalf("confirm=true: want run, got %s", out)
	}
	if len(calls) != 1 || calls[0]["device"] != "PC-1" {
		t.Fatalf("confirmed call args: %v", calls)
	}
	if _, leaked := calls[0]["confirm"]; leaked {
		t.Fatal("confirm was forwarded to the CLI child")
	}
}

func TestReadOnlyAndLocalOnlyMirrorsStayUngated(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root)
	for _, name := range []string{"sync", "teach", "health"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; ok {
			t.Fatalf("%s: unexpected confirm parameter", name)
		}
		if out := callTool(t, s, name, map[string]any{}); out != "ran" {
			t.Fatalf("%s: want run, got %s", name, out)
		}
	}
}

// An undeclared argument (a host-file flag the schema dropped) is refused by
// name before any preview or run; the preview runs the command with the root
// --dry-run so ordinary flags reach the CLI; a command that shadows --dry-run
// gets a preview that runs nothing.
func TestMirroredWritePreviewDryRunsAndRefusesUndeclared(t *testing.T) {
	root := &cobra.Command{Use: "kaseya-vsa-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("client-profile", "", "")
	scripts := &cobra.Command{Use: "scripts"}
	scripts.AddCommand(&cobra.Command{Use: "run-wait", Run: func(*cobra.Command, []string) {}})
	shadow := &cobra.Command{Use: "shadow", Run: func(*cobra.Command, []string) {}}
	shadow.Flags().Bool("dry-run", false, "")
	root.AddCommand(scripts, shadow)

	var calls []map[string]any
	s := server.NewMCPServer("confirm-dry-run-test", "test")
	for name, path := range map[string]string{"scripts_run_wait": "scripts run-wait", "shadow": "shadow"} {
		tool := mcplib.NewTool(name, mcplib.WithString("device"), mcplib.WithString("select"), mcplib.WithBoolean("dry-run"), mcplib.WithString("client-profile"))
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			calls = append(calls, req.GetArguments())
			return mcplib.NewToolResultText("dry-run: would POST"), nil
		})
	}
	RequireConfirmForMirroredWrites(s, root)

	for _, confirm := range []any{nil, true} {
		for _, flag := range []string{"config", "receipt-file", "audit-dir"} {
			args := map[string]any{"device": "PC-1", flag: "/tmp/x"}
			if confirm != nil {
				args["confirm"] = confirm
			}
			if out := callTool(t, s, "scripts_run_wait", args); !strings.Contains(out, `unknown MCP parameter "`+flag+`"`) {
				t.Fatalf("confirm=%v --%s: want a refusal naming it, got %s", confirm, flag, out)
			}
		}
	}
	// A bare --client-profile would read the forced --dry-run as its value.
	for _, confirm := range []any{nil, true} {
		args := map[string]any{"device": "PC-1", "client-profile": true}
		if confirm != nil {
			args["confirm"] = confirm
		}
		if out := callTool(t, s, "scripts_run_wait", args); !strings.Contains(out, `"client-profile" takes a value`) {
			t.Fatalf("confirm=%v: boolean true for a value flag must be refused, got %s", confirm, out)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("an undeclared or mistyped argument reached the CLI: %v", calls)
	}

	out := callTool(t, s, "scripts_run_wait", map[string]any{"device": "PC-1", "select": "id", "dry-run": false})
	if !strings.Contains(out, `"confirmation_required": true`) || !strings.Contains(out, "would POST") {
		t.Fatalf("want a dry-run preview, got %s", out)
	}
	if len(calls) != 1 || calls[0]["select"] != "id" || calls[0]["dry-run"] != true {
		t.Fatalf("preview must run the CLI with --dry-run and the ordinary flags: %v", calls)
	}

	if out := callTool(t, s, "shadow", map[string]any{"device": "PC-1"}); !strings.Contains(out, `"confirmation_required": true`) || len(calls) != 1 {
		t.Fatalf("a shadowed --dry-run must preview without running: calls %v, out %s", calls, out)
	}
}
