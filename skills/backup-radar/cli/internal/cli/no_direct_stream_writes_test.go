// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/backup-radar/handfixes.json
// under the "output-escapes-c1-controls" entry.

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDirectTerminalWrites is the regression guard for the output-boundary
// class: generated code used to write warnings, errors, retry notes and
// dry-run request bodies with fmt.Fprintf(os.Stderr, ...), which bypasses the
// escaping writer, so a tenant string carrying ESC/BEL/OSC 52 reached the
// terminal. Every write in every package of this module (the CLI, the HTTP
// client, the store, config, learn, platform, cliutil and both binaries) must
// go through termio.Stdout()/termio.Stderr() (stdoutW()/stderrW() in this
// package, or the Cobra command writers).
func TestNoDirectTerminalWrites(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join("internal", "termio") || strings.HasPrefix(rel, ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, bad := range []string{"(os.Stderr,", "(os.Stdout,", "NewEncoder(os.Stdout)", "NewEncoder(os.Stderr)", "NewWriter(os.Stdout)", "NewWriter(os.Stderr)", "= os.Stderr\n", "= os.Stdout\n", "nil, os.Stderr)"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s writes to the terminal directly (%s); use termio.Stdout()/termio.Stderr()", rel, strings.TrimSpace(bad))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
