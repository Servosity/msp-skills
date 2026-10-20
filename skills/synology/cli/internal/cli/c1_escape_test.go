// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "output-escapes-c1-controls" entry.

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"synology-pp-cli/internal/termio"
)

// TestExecuteEscapesControlsOnStdoutAndStderr runs the real entry point.
// encoding/json escapes C0 controls but writes UTF-8 C1 (CSI, OSC, ST) raw,
// so a name in --json output, and an argument echoed in an error, reached the
// terminal as live control sequences before Execute wrapped both streams.
func TestExecuteEscapesControlsOnStdoutAndStderr(t *testing.T) {
	sweepIsolate(t)
	name := "acme\u009d52;c;QQ==\u009cnas"
	out, _, code := sweepExecute(t, "profile", "save", name, "--json")
	if code != 0 {
		t.Fatalf("profile save exited %d: %q", code, out)
	}
	assertNoTerminalControls(t, "stdout", out)
	if !strings.Contains(out, `\u009d52;c;QQ==\u009c`) {
		t.Fatalf("stdout lost the escaped name: %q", out)
	}
	_, errOut, code := sweepExecute(t, "version", "--bogus\u009b2J\x1b]52;c;QQ\x07")
	if code != 2 {
		t.Fatalf("unknown flag exited %d, want 2: %q", code, errOut)
	}
	assertNoTerminalControls(t, "stderr", errOut)
	if !strings.Contains(errOut, `\u009b2J\u001b]52;c;QQ\u0007`) {
		t.Fatalf("stderr lost the escaped flag text: %q", errOut)
	}
}

// TestIsTerminalUnwrapsEscapingWriter: commands decide table vs JSON with
// isTerminal(cmd.OutOrStdout()), which is now the escaping writer; it must
// still see the terminal underneath.
func TestIsTerminalUnwrapsEscapingWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.DevNull is not a character device on Windows")
	}
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !isTerminal(f) {
		t.Skip("os.DevNull is not a character device here")
	}
	if !isTerminal(termio.New(f)) {
		t.Fatal("isTerminal does not see the character device behind the escaping writer")
	}
}

// TestNoDirectTerminalWrites is the regression guard for the class: generated
// code wrote warnings, retry notes, dry-run request bodies and progress with
// fmt.Fprintf(os.Stderr, ...), bypassing the escaping writer, so a NAS-sourced
// string carrying ESC/BEL/OSC 52 reached the terminal. Every package in this
// module must write through termio.Stdout()/termio.Stderr() (stdoutW()/
// stderrW() here) or the Cobra command writers.
func TestNoDirectTerminalWrites(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join("internal", "termio") || rel == filepath.Join("internal", "cliutil", "testenv") || (strings.HasPrefix(rel, ".") && rel != ".") {
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
		for _, bad := range []string{"(os.Stderr,", "(os.Stdout,", "NewEncoder(os.Stdout)", "NewEncoder(os.Stderr)", "NewWriter(os.Stdout)", "NewWriter(os.Stderr)", "= os.Stderr\n", "= os.Stdout\n", "fmt.Printf(", "fmt.Println("} {
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
