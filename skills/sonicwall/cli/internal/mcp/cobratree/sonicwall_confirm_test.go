// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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

// A mirrored tool that is neither read-only nor local-only previews with the
// root --dry-run and runs only with boolean confirm (handfix mcp-mirror-write-confirm).
func TestSonicwallWritesPreviewWithDryRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	fake := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"ARGS: $*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "sonicwall-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(&cobra.Command{Use: "nsm-login", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "sync", Run: func(*cobra.Command, []string) {}})
	s := server.NewMCPServer("sonicwall-confirm-test", "test")
	var calls []map[string]any
	for name, path := range map[string]string{"nsm_login": "nsm-login", "sync": "sync"} {
		path := path
		tool := mcplib.NewTool(name)
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
			t.Fatalf("%s: %v", name, err)
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
		if out := call("nsm_login", args); !strings.Contains(out, "ARGS: nsm-login --dry-run") || !strings.Contains(out, "Preview only: nothing was sent") {
			t.Fatalf("confirm=%v: nsm_login must preview as its own --dry-run, got %s", confirm, out)
		}
	}
	if out := call("nsm_login", map[string]any{"confirm": true}); !strings.HasPrefix(out, "ran nsm-login") {
		t.Fatalf("confirm=true: want run, got %s", out)
	}
	if out := call("sync", map[string]any{}); !strings.HasPrefix(out, "ran sync") {
		t.Fatalf("sync is local-only and stays ungated, got %s", out)
	}
	if len(calls) != 2 {
		t.Fatalf("want exactly the confirmed nsm_login and sync to run: %v", calls)
	}
}
