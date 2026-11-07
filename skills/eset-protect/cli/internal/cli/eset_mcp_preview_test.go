// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"github.com/spf13/cobra"
)

// TestMCPPreviewParsesOnly guards the child half of mcp-mirror-write-confirm:
// with both MCP markers set, Execute parses the command line, prints what it
// parsed, runs none of the command's hooks or RunE, and writes no learn
// journal or any other file. With the child marker alone the same command
// runs and is journaled, so the empty checks are not blind.
func TestMCPPreviewParsesOnly(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	var ran []string
	saved := novelCommandHooks
	t.Cleanup(func() { novelCommandHooks = saved })
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		probe := &cobra.Command{
			Use:                "zz-preview-probe <name>",
			Args:               cobra.ExactArgs(1),
			PersistentPreRunE:  func(*cobra.Command, []string) error { ran = append(ran, "PersistentPreRunE"); return nil },
			PreRunE:            func(*cobra.Command, []string) error { ran = append(ran, "PreRunE"); return nil },
			RunE:               func(*cobra.Command, []string) error { ran = append(ran, "RunE"); return nil },
			PostRunE:           func(*cobra.Command, []string) error { ran = append(ran, "PostRunE"); return nil },
			PersistentPostRunE: func(*cobra.Command, []string) error { ran = append(ran, "PersistentPostRunE"); return nil },
		}
		probe.Flags().String("since", "", "")
		probe.Flags().Int("max-pages", 0, "")
		root.AddCommand(probe)
	})
	// learn.JournalDir creates the directory, so the path is built here.
	stateDir, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	journalDir := filepath.Join(stateDir, "learn")
	t.Setenv(cliutil.MCPChildEnvVar, cliutil.MCPChildPreview)
	if !cliutil.IsMCPPreviewEnv() || !cliutil.IsMCPChildEnv() {
		t.Fatal("the preview marker is set, but it does not read as a preview child")
	}

	before := listFiles(t, home)
	stdout, stderr, err := captureExecuteAll(t, "zz-preview-probe", "--since=2h", "a b")
	if err != nil {
		t.Fatalf("preview: %v\nstderr: %s", err, stderr)
	}
	out := string(stdout)
	if len(ran) != 0 {
		t.Errorf("a parse-only preview ran %v", ran)
	}
	for _, want := range []string{"eset-protect-cli zz-preview-probe", "--since=2h", `"a b"`, "nothing was run"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview output %q does not show %q", out, want)
		}
	}
	// A value or an argument count the command would refuse still fails.
	if _, _, err := captureExecuteAll(t, "zz-preview-probe", "--max-pages=x", "a"); err == nil || !strings.Contains(err.Error(), "max-pages") {
		t.Errorf("unparsable value in a preview: err=%v", err)
	}
	if _, _, err := captureExecuteAll(t, "zz-preview-probe"); err == nil {
		t.Error("a preview with a missing positional did not fail")
	}
	if len(ran) != 0 {
		t.Errorf("a failed preview ran %v", ran)
	}
	if after := listFiles(t, home); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("a preview created files:\nbefore %v\nafter  %v", before, after)
	}
	if _, err := os.Stat(journalDir); !os.IsNotExist(err) {
		t.Fatalf("a preview created the learn journal %s (stat err %v)", journalDir, err)
	}

	// Any other marker value is not a preview: the command runs and is journaled.
	for _, v := range []string{"1", "", "Preview", "preview ", "true"} {
		t.Setenv(cliutil.MCPChildEnvVar, v)
		if cliutil.IsMCPPreviewEnv() {
			t.Errorf("child marker %q reads as a preview", v)
		}
		ran = nil
		if _, stderr, err := captureExecuteAll(t, "zz-preview-probe", "--since=2h", "a"); err != nil {
			t.Fatalf("child marker %q: %v\nstderr: %s", v, err, stderr)
		}
		if !strings.Contains(strings.Join(ran, ","), "RunE") {
			t.Errorf("child marker %q: the command did not run (%v)", v, ran)
		}
	}
	if entries, err := os.ReadDir(journalDir); err != nil || len(entries) == 0 {
		t.Fatalf("a real run wrote no learn journal under %s (err %v); the journal check above is blind", journalDir, err)
	}
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}
