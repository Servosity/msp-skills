// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Class export-write-failure-reported (kit adoption C2): export --output
// whose file cannot take a byte (a file size limit of 0, as a full disk
// answers) must fail the command, never report success with a truncated
// file. The same export without the limit writes the records, so the
// failure is the write's.
func TestExportOutputWriteFailureFailsTheCommand(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	if runtime.GOOS == "windows" {
		t.Skip("the file size limit needs a POSIX shell's ulimit -f")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	export := func(limit bool, out string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		script := `exec "$0" "$@"`
		if limit {
			script = `ulimit -f 0 && exec "$0" "$@"`
		}
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script, bbCLI, "export", "timelogs", "--output", out)
		cmd.Env = e.env
		cmd.Dir = e.home
		b, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("export --output did not return:\n%s", b)
		}
		return b, err
	}
	ok := filepath.Join(e.home, "timelogs-ok.jsonl")
	if b, err := export(false, ok); err != nil {
		t.Fatalf("export --output without a size limit failed: %v\n%s", err, b)
	}
	if fi, err := os.Stat(ok); err != nil || fi.Size() == 0 {
		t.Fatalf("export --output without a size limit wrote no records (err %v)", err)
	}
	full := filepath.Join(e.home, "timelogs-full.jsonl")
	b, err := export(true, full)
	var size int64 = -1
	if fi, serr := os.Stat(full); serr == nil {
		size = fi.Size()
	}
	if err == nil {
		t.Fatalf("export --output reported success although its file took no byte (file size %d):\n%s", size, b)
	}
	assertNoEcho(t, "export write failure output", string(b))
}
