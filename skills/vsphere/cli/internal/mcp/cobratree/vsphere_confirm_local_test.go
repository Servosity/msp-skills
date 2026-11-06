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

// markerCLI is a CLI stand-in that records every run in a marker file, so a
// test can prove a preview never started the command.
func markerCLI(t *testing.T) (func() (string, error), string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	p := filepath.Join(dir, "fake-cli")
	script := "#!/bin/sh\necho \"$*\" >> '" + marker + "'\necho \"ARGS: $*\"\n"
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return func() (string, error) { return p, nil }, marker
}

// Class mcp-write-without-confirm (Codex r1): every MCP tool that is not a
// proven read (vendor writes, local writes, commands with no classification,
// tools outside the mirror) previews and acts only with confirm boolean true.
// The local writers the gate used to exempt (learnings forget, a command
// annotated mcp:local-write, sync) preview through --dry-run only when listed
// in LocalWriterDryRunPreviews (proven in vsphereblackbox); an unlisted local
// writer and an unclassified command never start the CLI before confirm. Reads
// stay ungated.
func TestLocalAndUnclassifiedWritersNeedConfirm(t *testing.T) {
	root := &cobra.Command{Use: "vsphere-cli"}
	root.PersistentFlags().Bool("dry-run", false, "")
	learnings := &cobra.Command{Use: "learnings"}
	forget := &cobra.Command{Use: "forget <query>", Run: func(*cobra.Command, []string) {}}
	forget.Flags().String("query", "", "")
	forget.Flags().Bool("all", false, "")
	learnings.AddCommand(forget)
	root.AddCommand(learnings)
	dryRunPreviews := []*cobra.Command{
		{Use: "teach", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}},
		{Use: "sync", Run: func(*cobra.Command, []string) {}},
		{Use: "export", Run: func(*cobra.Command, []string) {}},
	}
	for _, c := range dryRunPreviews {
		c.Flags().String("query", "", "")
		c.Flags().Bool("all", false, "")
		root.AddCommand(c)
	}
	root.AddCommand(&cobra.Command{Use: "unproven", Annotations: map[string]string{LocalWriteAnnotation: "true"}, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "brandnew", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "health", Annotations: map[string]string{ReadOnlyAnnotation: "true"}, Run: func(*cobra.Command, []string) {}})
	// A tool claiming read-only whose command is not annotated read-only is not
	// a proven read.
	root.AddCommand(&cobra.Command{Use: "claims", Run: func(*cobra.Command, []string) {}})

	cliPath, marker := markerCLI(t)
	var calls []string
	s := server.NewMCPServer("confirm-local", "test")
	addMirrored := func(name, path string, readOnly bool) {
		opts := []mcplib.ToolOption{mcplib.WithString("query"), mcplib.WithBoolean("all")}
		if readOnly {
			opts = append(opts, mcplib.WithReadOnlyHintAnnotation(true))
		}
		tool := mcplib.NewTool(name, opts...)
		tool.Meta = &mcplib.Meta{AdditionalFields: map[string]any{"pp:tenant-gate": "child-cli", mirrorCLICommandMetaKey: path}}
		s.AddTool(tool, func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			calls = append(calls, name)
			return mcplib.NewToolResultText("ran " + path), nil
		})
	}
	addMirrored("learnings_forget", "learnings forget", false)
	addMirrored("teach", "teach", false)
	addMirrored("sync", "sync", false)
	addMirrored("export", "export", false)
	addMirrored("unproven", "unproven", false)
	addMirrored("brandnew", "brandnew", false)
	addMirrored("claims", "claims", true)
	addMirrored("health", "health", true)
	// A typed tool outside the mirror that is not marked read-only.
	s.AddTool(mcplib.NewTool("typed_write", mcplib.WithString("query"), mcplib.WithBoolean("all")), func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		calls = append(calls, "typed_write")
		return mcplib.NewToolResultText("ran typed_write"), nil
	})
	s.AddTool(mcplib.NewTool("typed_read", mcplib.WithReadOnlyHintAnnotation(true)), func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		calls = append(calls, "typed_read")
		return mcplib.NewToolResultText("ran typed_read"), nil
	})

	RequireConfirmForMirroredWrites(s, root, cliPath)

	viaDryRun := []string{"learnings_forget", "teach", "sync", "export"}
	echoing := []string{"unproven", "brandnew", "claims", "typed_write"}
	gated := append(append([]string{}, viaDryRun...), echoing...)
	for _, name := range viaDryRun {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s: no confirm parameter", name)
		}
		for _, confirm := range []any{nil, false, "true", 1.0} {
			args := map[string]any{"query": "find application", "all": true}
			if confirm != nil {
				args["confirm"] = confirm
			}
			if out := callTool(t, s, name, args); !strings.Contains(out, "Preview only: nothing was sent") || !strings.Contains(out, " --dry-run --all --query=find application") {
				t.Errorf("%s confirm=%v: want the --dry-run preview, got %s", name, confirm, out)
			}
		}
	}
	_ = os.Remove(marker)
	for _, name := range echoing {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s: no confirm parameter", name)
		}
		for _, confirm := range []any{nil, false, "true", 1.0} {
			// Codex's input: {"query":"find application","all":true}.
			args := map[string]any{"query": "find application", "all": true}
			if confirm != nil {
				args["confirm"] = confirm
			}
			out := callTool(t, s, name, args)
			if !strings.Contains(out, "Preview only: nothing was sent") {
				t.Errorf("%s confirm=%v: want a preview, got %s", name, confirm, out)
			}
			// The echo names the arguments and repeats no value (a value
			// may become a held credential later in the session).
			if strings.Contains(out, "find application") || !strings.Contains(out, `"query"`) {
				t.Errorf("%s confirm=%v: the preview must name the arguments and repeat no value, got %s", name, confirm, out)
			}
		}
	}
	if len(calls) != 0 {
		t.Fatalf("a write ran without confirm: %v", calls)
	}
	if b, err := os.ReadFile(marker); err == nil {
		t.Fatalf("a preview started the CLI: %s", b)
	}
	for _, name := range gated {
		out := callTool(t, s, name, map[string]any{"query": "find application", "all": true, "confirm": true})
		if !strings.HasPrefix(out, "ran ") {
			t.Errorf("%s confirm=true: want run, got %s", name, out)
		}
	}
	if len(calls) != len(gated) {
		t.Fatalf("confirmed calls: %v", calls)
	}
	for _, name := range []string{"health", "typed_read"} {
		if _, ok := s.GetTool(name).Tool.InputSchema.Properties["confirm"]; ok {
			t.Errorf("%s: a read must stay ungated", name)
		}
		if out := callTool(t, s, name, map[string]any{}); !strings.HasPrefix(out, "ran ") {
			t.Errorf("%s: want run, got %s", name, out)
		}
	}
}
