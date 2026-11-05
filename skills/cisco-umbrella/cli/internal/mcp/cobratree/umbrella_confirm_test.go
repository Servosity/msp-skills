// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func confirmTestServer(t *testing.T, calls *[]map[string]any) (*server.MCPServer, *cobra.Command) {
	t.Helper()
	root := &cobra.Command{Use: "cisco-umbrella-cli"}
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
	writer := &cobra.Command{Use: "localwriter", Run: func(*cobra.Command, []string) {}}
	writer.Flags().String("filter", "", "")
	root.AddCommand(writer)
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
	add("localwriter", "localwriter", false)
	add("sync", "sync", false)
	add("teach", "teach", false)
	add("health", "health", true)
	add("unknown_write", "no such command", false)
	return s, root
}

// fakeCLIEnv makes this test binary a CLI stand-in that prints the arguments it was
// given and exits. The same stand-in runs on every OS: a shell script needs a POSIX
// shell, and a .bat file echoes Windows' raw command line, not the argv.
const fakeCLIEnv = "CISCO_UMBRELLA_TEST_FAKE_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(fakeCLIEnv) == "1" {
		fmt.Println("ARGS: " + strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeCLI is this test binary, started as the CLI stand-in.
func fakeCLI(t *testing.T) func() (string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeCLIEnv, "1")
	return func() (string, error) { return exe, nil }
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

// Class 4 (kaseya-vsa review: sync --dry-run opened a writable store first): a
// command not reviewed into dryRunPreviews previews by running nothing and
// returning its argv. Nothing has proved its --dry-run opens no store and writes
// no file. A host-file flag is still refused by name, and an ordinary flag passes.
func TestUnreviewedCommandPreviewRunsNothing(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	for _, confirm := range []any{nil, false, "true"} {
		args := map[string]any{"filter": "x"}
		if confirm != nil {
			args["confirm"] = confirm
		}
		out := callTool(t, s, "localwriter", args)
		if strings.Contains(out, "ARGS:") || !strings.Contains(out, `"confirmation_required": true`) || !strings.Contains(out, `"filter": "x"`) || !strings.Contains(out, `"command": "localwriter"`) {
			t.Fatalf("confirm=%v: want the argv and nothing run, got %s", confirm, out)
		}
	}
	for _, flag := range []string{"config", "home", "deliver", "receipt-file", "audit-dir", "ca-cert"} {
		out := callTool(t, s, "localwriter", map[string]any{"filter": "x", flag: "/etc/passwd"})
		if !strings.Contains(out, flag) || strings.Contains(out, "confirmation_required") || strings.Contains(out, "ARGS:") {
			t.Fatalf("--%s must be refused by name before a preview, got %s", flag, out)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("a preview ran the command: %v", calls)
	}
	if out := callTool(t, s, "localwriter", map[string]any{"filter": "x", "confirm": true}); out != "ran localwriter" {
		t.Fatalf("confirm=true: want run, got %s", out)
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

func TestReadOnlyAndLocalOnlyMirrorsStayUngated(t *testing.T) {
	var calls []map[string]any
	s, root := confirmTestServer(t, &calls)
	RequireConfirmForMirroredWrites(s, root, fakeCLI(t))
	for _, name := range []string{"sync", "teach", "health"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; ok {
			t.Fatalf("%s: unexpected confirm parameter", name)
		}
		if out := callTool(t, s, name, map[string]any{}); !strings.HasPrefix(out, "ran ") {
			t.Fatalf("%s: want run, got %s", name, out)
		}
	}
}

// A command annotated mcp:read-only-unless-flags (destination-lists diff
// --apply) plans without confirm; with the flag present it returns the plan
// (the call without the flag) until confirm is boolean true.
func TestReadOnlyUnlessFlagsPlansThenApplies(t *testing.T) {
	root := &cobra.Command{Use: "cisco-umbrella-cli"}
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
		// Nothing runs: a plan would mint a token and open a writable store.
		if out := callTool(t, s, "destination_lists_diff", args); strings.Contains(out, "ran diff") || !strings.Contains(out, `"confirmation_required": true`) || !strings.Contains(out, "f.txt") || !strings.Contains(out, "Preview only") {
			t.Fatalf("confirm=%v: want the argv and the note, got %s", confirm, out)
		}
	}
	if out := callTool(t, s, "destination_lists_diff", map[string]any{"apply": true, "config": "/etc/passwd"}); !strings.Contains(out, "config") || strings.Contains(out, "confirmation_required") {
		t.Fatalf("a host-file flag must be refused by name before a preview, got %s", out)
	}
	if len(calls) != 1 {
		t.Fatalf("a preview ran the command: %v", calls)
	}
	if _, leaked := calls[0]["confirm"]; leaked {
		t.Fatal("confirm was forwarded")
	}
	callTool(t, s, "destination_lists_diff", map[string]any{"file": "f.txt", "apply": true, "confirm": true})
	if last := calls[len(calls)-1]; last["apply"] != true {
		t.Fatalf("confirm=true must apply, got %v", last)
	}
}
