// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by handfixes.json under "mcp-write-confirm".

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

func confirmTestServer(t *testing.T, calls *[]map[string]any) (*server.MCPServer, *cobra.Command) {
	t.Helper()
	root := &cobra.Command{Use: "xero-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("receipt-file", "", "")
	root.Annotations = map[string]string{FlagWriteSinksAnnotation: "receipt-file"}
	for _, c := range []*cobra.Command{
		{Use: "import"},
		{Use: "sync"},
		{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}},
		{Use: "aging"},
	} {
		c.Run = func(*cobra.Command, []string) {}
		c.Flags().String("filter", "", "")
		root.AddCommand(c)
	}

	s := server.NewMCPServer("confirm-test", "test")
	add := func(name, path string, readOnly bool) {
		opts := []mcplib.ToolOption{mcplib.WithString("filter")}
		if readOnly {
			opts = append(opts, mcplib.WithReadOnlyHintAnnotation(true))
		}
		tool := mcplib.NewTool(name, opts...)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			*calls = append(*calls, req.GetArguments())
			return mcplib.NewToolResultText("ran " + path), nil
		})
	}
	add("import", "import", false)
	add("sync", "sync", false)
	add("teach", "teach", false)
	add("aging", "aging", true)
	add("unknown_write", "no such command", false)
	return s, root
}

// fakeCLI is a CLI stand-in that prints the arguments it was given.
func fakeCLI(t *testing.T) func() (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	p := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho \"ARGS: $*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return func() (string, error) { return p, nil }
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

// Every mirrored tool that is not read-only, local writes included (xero has
// no local-only exemption), previews as its own --dry-run through the mirror's
// argument checks and runs only with confirm set to boolean true.
func TestMirroredWriteNeedsBooleanConfirm(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))

	for _, name := range []string{"import", "sync", "teach", "unknown_write"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; !ok {
			t.Fatalf("%s: no confirm parameter", name)
		}
		for _, confirm := range []any{nil, false, "true", 1.0} {
			args := map[string]any{"filter": "x", "dry-run": false}
			if confirm != nil {
				args["confirm"] = confirm
			}
			out := callTool(t, s, name, args)
			want := "ARGS: " + name + " --dry-run"
			if name == "unknown_write" {
				want = `"confirmation_required": true`
			}
			if !strings.Contains(out, want) || !strings.Contains(out, "Preview only: nothing was sent") {
				t.Fatalf("%s confirm=%v: want preview %q, got %s", name, confirm, want, out)
			}
			if strings.Contains(out, "--dry-run=false") {
				t.Fatalf("%s: a caller dry-run value reached the preview argv: %s", name, out)
			}
		}
	}
	// The preview keeps the mirror's checks: a host-file flag is refused by
	// name and never reaches the CLI; an ordinary flag does.
	out := callTool(t, s, "import", map[string]any{"receipt-file": "/tmp/x"})
	if !strings.Contains(out, "receipt-file") || strings.Contains(out, "ARGS:") {
		t.Fatalf("a file flag must be refused by name, got %s", out)
	}
	if out := callTool(t, s, "import", map[string]any{"filter": "ordinary"}); !strings.Contains(out, "--filter=ordinary") {
		t.Fatalf("an ordinary flag must reach the preview, got %s", out)
	}
	if len(calls) != 0 {
		t.Fatalf("a write ran without confirm: %v", calls)
	}
	if out := callTool(t, s, "import", map[string]any{"filter": "x", "confirm": true}); out != "ran import" {
		t.Fatalf("confirm=true: want run, got %s", out)
	}
	if len(calls) != 1 || calls[0]["filter"] != "x" {
		t.Fatalf("confirmed call args: %v", calls)
	}
	if _, leaked := calls[0]["confirm"]; leaked {
		t.Fatal("confirm was forwarded to the CLI child")
	}
}

func TestReadOnlyMirrorStaysUngated(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	if _, ok := s.GetTool("aging").Tool.InputSchema.Properties["confirm"]; ok {
		t.Fatal("aging: unexpected confirm parameter")
	}
	if out := callTool(t, s, "aging", map[string]any{}); out != "ran aging" {
		t.Fatalf("aging: want run, got %s", out)
	}
}
