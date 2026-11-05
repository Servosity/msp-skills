// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// mimecast hand-fix (handfixes.json mimecast-mcp-mirror-write-confirm): every
// mirrored tool that can write, local writes included, previews and runs
// nothing until confirm is boolean true.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// confirmTestTree is a root with a local-only write (jobs prune), a
// local-write teach command, and a read-only command whose --new flag writes.
func confirmTestTree() *cobra.Command {
	run := func(*cobra.Command, []string) error { return nil }
	root := &cobra.Command{Use: "mimecast-cli"}
	jobs := &cobra.Command{Use: "jobs", Short: "jobs"}
	prune := &cobra.Command{Use: "prune", Short: "Prune the jobs ledger", RunE: run}
	prune.Flags().String("older-than", "", "age")
	jobs.AddCommand(prune)
	teach := &cobra.Command{Use: "teach", Short: "Teach", RunE: run, Annotations: map[string]string{LocalWriteAnnotation: "true"}}
	teach.Flags().String("note", "", "note")
	held := &cobra.Command{Use: "held", Short: "held"}
	triage := &cobra.Command{Use: "triage", Short: "Triage", RunE: run,
		Annotations: map[string]string{ReadOnlyAnnotation: "true", ReadOnlyUnlessFlagsAnnotation: "new"}}
	triage.Flags().Bool("new", false, "keep a local snapshot")
	triage.Flags().String("since", "24h", "window")
	held.AddCommand(triage)
	root.AddCommand(jobs, teach, held)
	return root
}

const argvLogEnv = "MIMECAST_CONFIRM_TEST_ARGV"

// TestMain doubles as the companion CLI: with argvLogEnv set, the test binary
// records the argv it was run with and exits.
func TestMain(m *testing.M) {
	if log := os.Getenv(argvLogEnv); log != "" {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			_ = f.Close()
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) string {
	t.Helper()
	entry, ok := s.ListTools()[name]
	if !ok {
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

func TestMirroredLocalWritesNeedConfirm(t *testing.T) {
	t.Setenv("MIMECAST_MCP_READ_ONLY", "")
	log := filepath.Join(t.TempDir(), "argv.log")
	t.Setenv(argvLogEnv, log)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cli := func() (string, error) { return self, nil }
	root := confirmTestTree()
	s := server.NewMCPServer("t", "1")
	RegisterAll(s, root, cli)
	RequireConfirmForMirroredWrites(s, root, cli)

	// ran reports whether the last call ran the command for real: the
	// companion was invoked without --help (a --help preview runs no RunE).
	ran := func(string) bool {
		data, _ := os.ReadFile(log)
		_ = os.Remove(log)
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		last := lines[len(lines)-1]
		return last != "" && !strings.Contains(" "+last+" ", " --help ")
	}
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"jobs_prune", map[string]any{"older-than": "0s"}},
		{"jobs_prune", map[string]any{"older-than": "0s", "confirm": "true"}},
		{"teach", map[string]any{"note": "x"}},
		{"held_triage", map[string]any{"new": true}},
	} {
		if text := callTool(t, s, tc.tool, tc.args); ran(text) || !strings.Contains(text, "confirm") {
			t.Errorf("%s %v ran (or did not preview) without confirm: true: %s", tc.tool, tc.args, text)
		}
	}
	if text := callTool(t, s, "jobs_prune", map[string]any{"older-than": "0s", "confirm": true}); !ran(text) {
		t.Errorf("jobs_prune with confirm: true must run: %s", text)
	}
	if text := callTool(t, s, "held_triage", map[string]any{"since": "1h"}); !ran(text) {
		t.Errorf("a read-only call without its write flag must run without confirm: %s", text)
	}
	if text := callTool(t, s, "held_triage", map[string]any{"new": true, "confirm": true}); !ran(text) {
		t.Errorf("held_triage --new with confirm: true must run: %s", text)
	}
	for _, name := range []string{"jobs_prune", "teach", "held_triage"} {
		if _, ok := s.ListTools()[name].Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s does not declare confirm", name)
		}
	}
}
