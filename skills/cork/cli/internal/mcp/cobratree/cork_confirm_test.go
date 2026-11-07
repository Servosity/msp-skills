// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// cork handfix mcp-mirror-write-confirm: a mirrored write previews as its own
// --dry-run until confirm is boolean true; a local-only command stays ungated.
func TestCorkMirroredWritesPreviewWithDryRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	fake := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"ARGS: $*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "cork-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	imp := &cobra.Command{Use: "import", Run: func(*cobra.Command, []string) {}}
	imp.Flags().Bool("upsert", false, "")
	root.AddCommand(imp, &cobra.Command{Use: "sync", Run: func(*cobra.Command, []string) {}})
	calls := []map[string]any{}
	s := server.NewMCPServer("cork-confirm-test", "test")
	for _, path := range []string{"import", "sync"} {
		path := path
		tool := mcplib.NewTool(path, mcplib.WithBoolean("upsert"))
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			calls = append(calls, req.GetArguments())
			return mcplib.NewToolResultText("ran " + path), nil
		})
	}
	RequireConfirmForMirroredWrites(s, root, func() (string, error) { return fake, nil })
	call := func(name string, args map[string]any) string {
		req := mcplib.CallToolRequest{}
		req.Params.Name, req.Params.Arguments = name, args
		res, err := s.GetTool(name).Handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				b.WriteString(tc.Text + "\n")
			}
		}
		return b.String()
	}
	for _, confirm := range []any{nil, false, "true", 1.0} {
		args := map[string]any{"dry-run": false}
		if confirm != nil {
			args["confirm"] = confirm
		}
		if out := call("import", args); !strings.Contains(out, "ARGS: import --dry-run") || !strings.Contains(out, "Preview only") {
			t.Fatalf("confirm=%v: import must run as its own --dry-run preview, got %s", confirm, out)
		}
	}
	if out := call("import", map[string]any{"confirm": true}); !strings.HasPrefix(out, "ran import") {
		t.Fatalf("confirm=true: want the mirrored send, got %s", out)
	}
	if out := call("sync", map[string]any{}); !strings.HasPrefix(out, "ran sync") {
		t.Fatalf("sync is local-only and stays ungated, got %s", out)
	}
	if len(calls) != 2 {
		t.Fatalf("want exactly the confirmed import and sync to reach the handler: %v", calls)
	}
}
