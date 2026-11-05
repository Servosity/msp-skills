// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "credkit-wiring" (class secret-echo).

//go:build unix

package config

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestHoldFilesNeverReadsAFIFO: a config path naming a FIFO (or a
// device such as /dev/stdin) is skipped at start, so the program neither
// blocks on it nor drains what Load, or the MCP stdio stream, reads from it.
func TestHoldFilesNeverReadsAFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "config.toml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	t.Setenv("AVANAN_CONFIG", fifo)
	done := make(chan struct{})
	go func() {
		HoldFiles(ReadPaths(""))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HoldFiles blocked reading a FIFO named as the config")
	}
}
