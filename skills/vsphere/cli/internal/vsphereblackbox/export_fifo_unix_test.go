// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package vsphereblackbox

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// A file export cannot write in full fails the command: the reader of this pipe is
// gone before the first write.
func TestExportFailsWhenItsFileCannotBeWritten(t *testing.T) {
	t.Parallel()
	e := newEnv(t, quietServer(t), true)
	fifo := filepath.Join(e.home, "out.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bbCLI, "export", "vm", "--format", "json", "--output", fifo)
	cmd.Env, cmd.Dir = e.env, e.home
	done := make(chan error, 1)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	r, err := os.OpenFile(fifo, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if err := <-done; err == nil {
		t.Fatalf("export exited 0 with its file unwritten:\n%s", out.String())
	}
}
