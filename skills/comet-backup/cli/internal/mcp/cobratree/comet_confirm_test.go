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

func confirmTestServer(t *testing.T, calls *[]map[string]any) (*server.MCPServer, *cobra.Command) {
	t.Helper()
	root := &cobra.Command{Use: "comet-backup-cli"}
	root.PersistentFlags().Bool("yes", false, "")
	fleet := &cobra.Command{Use: "fleet"}
	ack := &cobra.Command{Use: "ack", Annotations: map[string]string{SendsWithYesAnnotation: "true"}, Run: func(*cobra.Command, []string) {}}
	ack.Flags().String("filter", "", "")
	fleet.AddCommand(ack)
	root.AddCommand(fleet)
	root.PersistentFlags().Bool("dry-run", false, "")
	imp := &cobra.Command{Use: "import", Run: func(*cobra.Command, []string) {}}
	imp.Flags().String("filter", "", "")
	root.AddCommand(imp)
	root.AddCommand(&cobra.Command{Use: "sync", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "health", Run: func(*cobra.Command, []string) {}})

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
	add("fleet_ack", "fleet ack", false)
	add("import", "import", false)
	add("sync", "sync", false)
	add("teach", "teach", false)
	add("health", "health", true)
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

func TestMirroredWriteNeedsBooleanConfirm(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))

	for _, name := range []string{"import", "unknown_write"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; !ok {
			t.Fatalf("%s: no confirm parameter", name)
		}
		for _, confirm := range []any{nil, false, "true", 1.0} {
			args := map[string]any{"filter": "x"}
			if confirm != nil {
				args["confirm"] = confirm
			}
			out := callTool(t, s, name, args)
			// A command in the tree previews as its own --dry-run; one that is
			// not echoes the arguments.
			want := "ARGS: import --dry-run --filter=x"
			if name == "unknown_write" {
				want = `"confirmation_required": true`
			}
			if !strings.Contains(out, want) || !strings.Contains(out, "Preview only: nothing was sent") {
				t.Fatalf("%s confirm=%v: want preview %q, got %s", name, confirm, want, out)
			}
		}
	}
	// The preview keeps the mirror's argument checks: an unknown or blocked
	// argument is refused by name.
	if out := callTool(t, s, "import", map[string]any{"input": "/etc/passwd"}); !strings.Contains(out, `"input"`) || strings.Contains(out, "ARGS:") {
		t.Fatalf("a blocked argument must be refused by name, got %s", out)
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

// A command that previews by itself until --yes: without boolean confirm the
// mirrored command runs as is (its own preview, no --yes) with a note; with
// confirm: true the gate runs it with --yes; a "yes" argument never reaches it.
func TestSendsWithYesPreviewsThenSendsWithYes(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))

	for _, confirm := range []any{nil, false, "true"} {
		args := map[string]any{"filter": "name = Ping"}
		if confirm != nil {
			args["confirm"] = confirm
		}
		out := callTool(t, s, "fleet_ack", args)
		if !strings.Contains(out, "ran fleet ack") || !strings.Contains(out, "Preview only: nothing was sent") {
			t.Fatalf("confirm=%v: want the command's own preview and the note, got %s", confirm, out)
		}
	}
	for _, c := range calls {
		if _, leaked := c["confirm"]; leaked {
			t.Fatal("confirm was forwarded to the preview")
		}
	}
	out := callTool(t, s, "fleet_ack", map[string]any{"filter": "name = Ping", "confirm": true})
	if !strings.Contains(out, "ARGS: fleet ack --yes --filter=name = Ping") {
		t.Fatalf("confirm=true: want the command run with --yes, got %s", out)
	}
	if strings.Contains(out, "confirm") {
		t.Fatalf("confirm reached the CLI argv: %s", out)
	}
	if out := callTool(t, s, "fleet_ack", map[string]any{"filter": "x", "yes": true, "confirm": true}); strings.Contains(out, "ARGS:") {
		t.Fatalf("a yes argument must be refused, got %s", out)
	}
}

func TestOnlyReadOnlyMirrorsStayUngated(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	if _, ok := s.GetTool("health").Tool.InputSchema.Properties["confirm"]; ok {
		t.Fatal("health: unexpected confirm parameter")
	}
	if out := callTool(t, s, "health", map[string]any{}); !strings.HasPrefix(out, "ran ") {
		t.Fatalf("health: want run, got %s", out)
	}
	// A local write previews through the CLI's --help, so the arguments pass the
	// mirror's checks and the command does not run: a sync preview that ran would
	// still open the writable store.
	for _, name := range []string{"sync", "teach"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; !ok {
			t.Fatalf("%s: no confirm parameter", name)
		}
		if out := callTool(t, s, name, map[string]any{}); strings.HasPrefix(out, "ran ") || !strings.Contains(out, "--help") || !strings.Contains(out, confirmNote) {
			t.Fatalf("%s: want a preview that runs nothing, got %s", name, out)
		}
		if out := callTool(t, s, name, map[string]any{"confirm": true}); !strings.HasPrefix(out, "ran ") {
			t.Fatalf("%s: confirm must run it, got %s", name, out)
		}
	}
}

// A command annotated mcp:read-only-unless-flags (destination-lists diff
// --apply) plans without confirm; with the flag present it returns the plan
// (the call without the flag) until confirm is boolean true.
func TestReadOnlyUnlessFlagsPlansThenApplies(t *testing.T) {
	root := &cobra.Command{Use: "comet-backup-cli"}
	lists := &cobra.Command{Use: "destination-lists"}
	diff := &cobra.Command{Use: "diff", Annotations: map[string]string{ReadOnlyUnlessFlagsAnnotation: "apply"}, Run: func(*cobra.Command, []string) {}}
	diff.Flags().Bool("apply", false, "")
	diff.Flags().String("file", "", "")
	lists.AddCommand(diff)
	root.AddCommand(lists)
	var calls []map[string]any
	s := server.NewMCPServer("confirm-test", "test")
	tool := mcplib.NewTool("destination_lists_diff", mcplib.WithString("file"), mcplib.WithBoolean("apply"))
	tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: "destination-lists diff"}}
	s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		calls = append(calls, req.GetArguments())
		return mcplib.NewToolResultText("ran diff"), nil
	})
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	if _, ok := s.GetTool("destination_lists_diff").Tool.InputSchema.Properties["confirm"]; !ok {
		t.Fatal("no confirm parameter")
	}
	if out := callTool(t, s, "destination_lists_diff", map[string]any{"file": "f.txt"}); out != "ran diff" {
		t.Fatalf("plan without apply must run as is, got %s", out)
	}
	for _, confirm := range []any{nil, false, "true", 1.0} {
		args := map[string]any{"file": "f.txt", "apply": true}
		if confirm != nil {
			args["confirm"] = confirm
		}
		if out := callTool(t, s, "destination_lists_diff", args); !strings.Contains(out, "ran diff") || !strings.Contains(out, "Preview only") {
			t.Fatalf("confirm=%v: want the plan and the note, got %s", confirm, out)
		}
	}
	for i, c := range calls {
		if _, applied := c["apply"]; applied {
			t.Fatalf("call %d ran with apply before confirm: %v", i, c)
		}
		if _, leaked := c["confirm"]; leaked {
			t.Fatal("confirm was forwarded")
		}
	}
	callTool(t, s, "destination_lists_diff", map[string]any{"file": "f.txt", "apply": true, "confirm": true})
	if last := calls[len(calls)-1]; last["apply"] != true {
		t.Fatalf("confirm=true must apply, got %v", last)
	}
}

// A clean --help answers with the echo; any refusal from the mirror's checks stays.
func TestHelpPreviewEchoesOnlyACleanHelp(t *testing.T) {
	run := func(res *mcplib.CallToolResult) string {
		h := helpPreview("sync", "sync", func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) { return res, nil })
		var req mcplib.CallToolRequest
		req.Params.Arguments = map[string]any{"select": "x"}
		out, err := h(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return out.Content[0].(mcplib.TextContent).Text
	}
	if got := run(toolResultFromCLIHelp("Usage:")); !strings.Contains(got, "confirmation_required") || !strings.Contains(got, `"select"`) {
		t.Errorf("clean help: %s", got)
	}
	if got := run(boundedToolResultError("--home is not allowed")); got != "--home is not allowed" {
		t.Errorf("refusal lost: %s", got)
	}
}

// The preview's own --dry-run is the last word: a caller cannot turn it off.
func TestPreviewDropsACallersDryRunFalse(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	out := callTool(t, s, "import", map[string]any{"dry-run": "false", "filter": "x"})
	if strings.Contains(out, "dry-run=false") || !strings.Contains(out, "--dry-run") {
		t.Fatalf("the preview ran without its dry run: %s", out)
	}
}
