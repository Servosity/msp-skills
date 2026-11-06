// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package vsphereblackbox

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Class export-read-back: the withheld check reads back only a regular file. A
// FIFO has nothing to read back (the read waited for a writer forever), and
// /dev/stdout through a pipe read back as empty, a false failure.
func TestExportReadsBackOnlyARegularFile(t *testing.T) {
	t.Parallel()
	e := newEnv(t, quietServer(t), true)
	run := func(t *testing.T, cmd *exec.Cmd) (string, error) {
		t.Helper()
		var out bytes.Buffer
		if cmd.Stdout == nil {
			cmd.Stdout = &out
		}
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return out.String(), err
		case <-time.After(60 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Fatalf("export did not finish:\n%s", out.String())
			return "", nil
		}
	}

	t.Run("a FIFO with a reader", func(t *testing.T) {
		fifo := filepath.Join(e.home, "out.fifo")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		got := make(chan string, 1)
		go func() {
			f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
			if err != nil {
				got <- ""
				return
			}
			b, _ := io.ReadAll(f)
			_ = f.Close()
			got <- string(b)
		}()
		cmd := exec.Command(bbCLI, "export", "vm", "--format", "jsonl", "--output", fifo)
		cmd.Env, cmd.Dir = e.env, e.home
		out, err := run(t, cmd)
		if err != nil || !strings.Contains(out, "Exported ") {
			t.Fatalf("export to a FIFO: %v\n%s", err, out)
		}
		if data := <-got; !strings.Contains(data, "HQ-Record") {
			t.Fatalf("the FIFO's reader got %q", data)
		}
	})

	t.Run("/dev/stdout through a pipe", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			// The kit's OpenFile resolves /dev/stdout to the /proc link
			// pipe:[n], which no path opens (.review/kit-findings.md).
			t.Skip("credkit.OpenFile cannot open /dev/stdout through a pipe on Linux")
		}
		cmd := exec.Command(bbCLI, "export", "vm", "--format", "json", "--output", "/dev/stdout")
		cmd.Env, cmd.Dir = e.env, e.home
		var stdout bytes.Buffer
		cmd.Stdout = &stdout // a pipe, not a terminal or a file
		out, err := run(t, cmd)
		if err != nil || !strings.Contains(stdout.String(), "HQ-Record") {
			t.Fatalf("export to /dev/stdout: %v\nstdout: %s\nstderr: %s", err, stdout.String(), out)
		}
	})
}
