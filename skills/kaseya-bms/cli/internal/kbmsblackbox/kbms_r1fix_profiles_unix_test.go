// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package kbmsblackbox

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A profiles.json that is a named pipe is no profile store: startup reads only
// a regular file, so it neither waits on the pipe nor refuses to run.
func TestProfileStorePipeNeverBlocksStartup(t *testing.T) {
	t.Parallel() // its own home: independent of every other test
	env := profileEnv(t)
	home := strings.TrimPrefix(env[1], "HOME=")
	if out, code := runCode(t, env, "", "profile", "save", "seed", "--select", "Id"); code != 0 {
		t.Fatalf("profile save seed: exit %d:\n%.1500s", code, out)
	}
	store := profileStoreUnder(t, home)
	if err := os.Remove(store); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(store, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	// The legacy store's place too.
	legacy := filepath.Join(home, ".kaseya-bms-cli", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(legacy, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	out, code := runCode(t, env, "", "version")
	if code != 0 || strings.Contains(out, "credkit:") || !strings.Contains(out, "kaseya-bms-cli") {
		t.Fatalf("version with a pipe for profiles.json: exit %d, want it to run as usual:\n%.1500s", code, out)
	}
}
