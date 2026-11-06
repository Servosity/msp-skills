// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// Class preview-drops-file-flags (check_mcp_file_flags on the publish lane):
// an unconfirmed call to a local writer answered with an echo before any
// argument check, so {"config": "/etc/x"} read as accepted and an ordinary
// flag never reached the CLI. A preview refuses a host-file flag by name
// first, and a local writer whose --dry-run is proven side-effect free
// (TestLocalWriterPreviewsChangeNothing in vsphereblackbox) previews through
// that run, so ordinary flags reach the CLI. Any other tool still starts
// nothing before confirm.
func TestPreviewRefusesFileFlagsAndForwardsOrdinaryFlags(t *testing.T) {
	root := &cobra.Command{Use: "vsphere-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("config", "", "")
	root.PersistentFlags().String("receipt-file", "", "")
	teach := &cobra.Command{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}}
	teach.Flags().String("select", "", "")
	teach.Flags().String("playbook-file", "", "")
	learnings := &cobra.Command{Use: "learnings"}
	forget := &cobra.Command{Use: "forget <query>", Run: func(*cobra.Command, []string) {}}
	forget.Flags().String("select", "", "")
	learnings.AddCommand(forget)
	unproven := &cobra.Command{Use: "unproven", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}}
	unproven.Flags().String("note", "", "")
	retire := &cobra.Command{Use: "retire [id...]", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}}
	root.AddCommand(teach, learnings, unproven, retire)
	LocalWriterDryRunPreviews["retire"] = true
	t.Cleanup(func() { delete(LocalWriterDryRunPreviews, "retire") })

	cliPath, marker := argvCLI(t)
	s := server.NewMCPServer("preview-file-flags", "test")
	add := func(name, path string, opts ...mcplib.ToolOption) {
		tool := mcplib.NewTool(name, opts...)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText("ran " + path), nil
		})
	}
	add("teach", "teach", mcplib.WithString("select"))
	add("learnings_forget", "learnings forget", mcplib.WithString("query"), mcplib.WithString("select"))
	add("unproven", "unproven", mcplib.WithString("note"))
	add("retire", "retire", mcplib.WithString("args"))
	RequireConfirmForMirroredWrites(s, root, cliPath)

	ran := func() string {
		b, _ := os.ReadFile(marker)
		_ = os.Remove(marker)
		return string(b)
	}

	// The checker's input: a host-file flag on an unconfirmed call.
	for _, c := range []struct {
		tool string
		flag string
	}{
		{"teach", "config"}, {"teach", "receipt-file"}, {"teach", "playbook-file"},
		{"learnings_forget", "config"}, {"unproven", "config"}, {"unproven", "receipt-file"},
	} {
		out := callToolResult(t, s, c.tool, map[string]any{c.flag: "/etc/hosts"})
		if !out.isError || !strings.Contains(out.text, c.flag) {
			t.Errorf("%s --%s: want an error naming the flag, got error=%v %s", c.tool, c.flag, out.isError, out.text)
		}
		if got := ran(); strings.Contains(got, "/etc/hosts") {
			t.Errorf("%s --%s: the value reached the CLI: %s", c.tool, c.flag, got)
		}
	}

	// An ordinary flag reaches the CLI, in a run that cannot write.
	out := callToolResult(t, s, "teach", map[string]any{"select": "a,b"})
	if out.isError || !strings.Contains(out.text, "Preview only") {
		t.Errorf("teach select: want a preview, got error=%v %s", out.isError, out.text)
	}
	if got := ran(); got != "teach\n--dry-run\n--select=a,b\n" {
		t.Errorf("teach select: want the flag on a --dry-run argv, got %q", got)
	}

	// Nothing the caller sends becomes a second --dry-run argument, which is
	// the one way to turn the preview off (one recorded line per argument).
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"learnings_forget", map[string]any{"select": "a", "dry-run": false}},
		{"learnings_forget", map[string]any{"select": "a", "dry-run": "false"}},
		{"learnings_forget", map[string]any{"query": "--dry-run=false"}},
		{"learnings_forget", map[string]any{"args": "x --dry-run=false"}},
		{"learnings_forget", map[string]any{"args": "--dry-run=false"}},
		{"retire", map[string]any{"args": "x --dry-run=false"}},
		{"retire", map[string]any{"args": "--dry-run=false"}},
	} {
		out := callToolResult(t, s, c.tool, c.args)
		got := ran()
		if strings.Contains("\n"+got, "\n--dry-run=") {
			t.Errorf("%s %v turned the preview off: %q", c.tool, c.args, got)
		}
		if got != "" && !strings.Contains(got, "\n--dry-run\n") {
			t.Errorf("%s %v: started the CLI without the preview flag: %q", c.tool, c.args, got)
		}
		if got == "" && !out.isError {
			t.Errorf("%s %v: neither ran the preview nor refused: %s", c.tool, c.args, out.text)
		}
	}

	// A local writer with no proven --dry-run still starts nothing.
	out = callToolResult(t, s, "unproven", map[string]any{"note": "a"})
	if out.isError || !strings.Contains(out.text, "Preview only") {
		t.Errorf("unproven note: want the echo preview, got error=%v %s", out.isError, out.text)
	}
	if got := ran(); got != "" {
		t.Errorf("unproven started the CLI: %q", got)
	}

	// confirm: true still sends what the caller gave.
	if out := callToolResult(t, s, "teach", map[string]any{"select": "a", "confirm": true}); out.text != "ran teach" {
		t.Errorf("teach confirm: %s", out.text)
	}
}

// argvCLI is markerCLI recording one line per argument, so a test tells one
// argument holding "x --dry-run=false" from two.
func argvCLI(t *testing.T) (func() (string, error), string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	cliPath, marker := markerCLI(t)
	p, _ := cliPath()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> '" + marker + "'\necho ran\n"
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return cliPath, marker
}

type toolOutcome struct {
	isError bool
	text    string
}

func callToolResult(t *testing.T, s *server.MCPServer, name string, args map[string]any) toolOutcome {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := s.GetTool(name).Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return toolOutcome{isError: res.IsError, text: b.String()}
}
