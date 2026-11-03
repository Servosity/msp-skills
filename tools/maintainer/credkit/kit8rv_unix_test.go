// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build unix

package credkit

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fifoAppends is Codex's FIFO input: two AppendFile calls to one FIFO.
func fifoAppends(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	AppendFile(p, []byte("secr"), 0o600)
	AppendFile(p, []byte("et!!"), 0o600)
	keep, _ := os.OpenFile(p, os.O_WRONLY, 0) // a writer open, so the read below ends at what is there
	defer keep.Close()
	b := make([]byte, 4096)
	n, _ := r.Read(b)
	if strings.Contains(string(b[:n]), "secret!!") {
		t.Errorf("two AppendFile calls to one FIFO completed the credential: %q", b[:n])
	}

}
