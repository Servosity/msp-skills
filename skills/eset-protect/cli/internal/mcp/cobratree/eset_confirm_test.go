// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func TestMirroredWriteNeedsBooleanConfirm(t *testing.T) {
	root := &cobra.Command{Use: "eset-protect-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	teach := &cobra.Command{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}}
	teach.Flags().String("query", "", "")
	root.AddCommand(teach, &cobra.Command{Use: "health", Run: func(*cobra.Command, []string) {}})

	// Each call records its arguments and whether its child would run parse-only.
	var calls []map[string]any
	var parseOnly []bool
	s := server.NewMCPServer("confirm-test", "test")
	add := func(name, path string, opts ...mcplib.ToolOption) {
		tool := mcplib.NewTool(name, append(opts, mcplib.WithString("query"), mcplib.WithBoolean("dry-run"))...)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			calls = append(calls, req.GetArguments())
			parseOnly = append(parseOnly, mcpParseOnlyPreviewRequested(ctx))
			if req.GetArguments()["query"] == "unparsable" {
				return mcplib.NewToolResultError(`invalid argument "unparsable" for "--query" flag`), nil
			}
			return mcplib.NewToolResultText("ran"), nil
		})
	}
	add("teach", "teach")
	add("unknown_write", "no such command")
	add("health", "health", mcplib.WithReadOnlyHintAnnotation(true))
	RequireConfirmForMirroredWrites(s, root)

	call := func(name string, args map[string]any) string {
		t.Helper()
		req := mcplib.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := s.GetTool(name).Handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String()
	}

	// A preview runs the child parse-only, a --dry-run in the call included (it is
	// an ordinary flag there); the string "true" is not consent.
	for _, confirm := range []any{nil, "true", 1} {
		calls, parseOnly = nil, nil
		args := map[string]any{"query": "q"}
		if confirm != nil {
			args["confirm"] = confirm
		}
		out := call("teach", args)
		if !strings.Contains(out, "confirmation_required") || !strings.Contains(out, `"parse_only": "ran"`) ||
			len(calls) != 1 || !parseOnly[0] || calls[0]["confirm"] != nil || calls[0]["query"] != "q" {
			t.Errorf("confirm=%v: %s, calls %v, parse-only %v", confirm, out, calls, parseOnly)
		}
	}
	calls, parseOnly = nil, nil
	if out := call("teach", map[string]any{"query": "q", "dry-run": true}); !strings.Contains(out, "confirmation_required") || len(calls) != 1 || !parseOnly[0] {
		t.Errorf("preview with dry-run: %s, calls %v, parse-only %v", out, calls, parseOnly)
	}
	// A value the CLI cannot parse comes back as the CLI's error, before consent.
	calls, parseOnly = nil, nil
	if out := call("teach", map[string]any{"query": "unparsable"}); strings.Contains(out, "confirmation_required") || !strings.Contains(out, "invalid argument") {
		t.Errorf("unparsable preview: %s", out)
	}
	// With consent it runs for real: not parse-only, and without confirm reaching the CLI.
	calls, parseOnly = nil, nil
	if out := call("teach", map[string]any{"query": "q", "confirm": true}); out != "ran" || len(calls) != 1 || parseOnly[0] || calls[0]["confirm"] != nil {
		t.Errorf("confirmed: %s, calls %v, parse-only %v", out, calls, parseOnly)
	}
	// A command the tree cannot name still fails closed: a preview that runs nothing.
	calls = nil
	if out := call("unknown_write", nil); !strings.Contains(out, "confirmation_required") || len(calls) != 0 {
		t.Errorf("unknown write: %s, calls %v", out, calls)
	}
	// Undeclared names are refused before a preview; boolean true on a value flag too.
	calls = nil
	if out := call("teach", map[string]any{"config": "/etc/passwd"}); !strings.Contains(out, "config") || len(calls) != 0 {
		t.Errorf("undeclared: %s, calls %v", out, calls)
	}
	if out := call("teach", map[string]any{"query": true}); !strings.Contains(out, "takes a value") || len(calls) != 0 {
		t.Errorf("boolean on a value flag: %s, calls %v", out, calls)
	}
	// Read-only tools are not gated.
	if out := call("health", nil); out != "ran" {
		t.Errorf("read-only: %s", out)
	}
}

// TestMCPChildEnvironDropsInheritedPreviewMarker: only the confirm gate's
// preview sets the child marker to the preview value. A marker inherited from the
// MCP server's own environment (any case on Windows, where names fold) never
// reaches a child: a real call gets exactly one marker, "1", and a preview exactly
// one, "preview".
func TestMCPChildEnvironDropsInheritedPreviewMarker(t *testing.T) {
	t.Setenv(cliutil.MCPChildEnvVar, cliutil.MCPChildPreview)
	if runtime.GOOS != "windows" {
		t.Setenv(strings.ToLower(cliutil.MCPChildEnvVar), cliutil.MCPChildPreview)
	}
	markers := func(env []string) []string {
		var got []string
		for _, entry := range env {
			if key, _, _ := strings.Cut(entry, "="); strings.EqualFold(key, cliutil.MCPChildEnvVar) {
				got = append(got, entry)
			}
		}
		return got
	}
	if got := markers(mcpChildEnviron()); len(got) != 1 || got[0] != cliutil.MCPChildEnvVar+"=1" {
		t.Errorf("real child markers %v, want exactly %s=1", got, cliutil.MCPChildEnvVar)
	}
	if got := markers(mcpChildEnvironWithMarker(cliutil.MCPChildPreview)); len(got) != 1 || got[0] != cliutil.MCPChildEnvVar+"="+cliutil.MCPChildPreview {
		t.Errorf("preview child markers %v, want exactly %s=%s", got, cliutil.MCPChildEnvVar, cliutil.MCPChildPreview)
	}
	if mcpParseOnlyPreviewRequested(context.Background()) || !mcpParseOnlyPreviewRequested(withMCPParseOnlyPreview(context.Background())) {
		t.Error("the parse-only context value does not round-trip")
	}
	// RunCLICommand picks the value from the context, on a real child process.
	bin := writeShelloutHelper(t, "print-env")
	for _, c := range []struct {
		ctx  context.Context
		want string
	}{
		{context.Background(), "1"},
		{withMCPParseOnlyPreview(context.Background()), cliutil.MCPChildPreview},
	} {
		result, err := RunCLICommand(c.ctx, bin, nil)
		if err != nil {
			t.Fatalf("RunCLICommand: %v", err)
		}
		if got := markers(strings.Split(strings.ReplaceAll(result.Stdout, "\r\n", "\n"), "\n")); len(got) != 1 || got[0] != cliutil.MCPChildEnvVar+"="+c.want {
			t.Errorf("child process markers %v, want exactly %s=%s", got, cliutil.MCPChildEnvVar, c.want)
		}
	}
}
