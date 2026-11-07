// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-event-writer-serialized): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_event_writer_test.go.tmpl after a reprint.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/store"
)

// syncOverlapWriter records whether two Write calls were ever in flight at
// once. The writers sync hands its workers in a real run are not all safe for
// that: --deliver copies the output into a bytes.Buffer and --receipt-file /
// --audit-dir hash it as it goes.
type syncOverlapWriter struct {
	inFlight atomic.Int32
	overlaps atomic.Int32
	mu       sync.Mutex
	buf      bytes.Buffer
}

func (w *syncOverlapWriter) Write(p []byte) (int, error) {
	if w.inFlight.Add(1) > 1 {
		w.overlaps.Add(1)
	}
	defer w.inFlight.Add(-1)
	time.Sleep(time.Millisecond) // widen the window so an unserialized pair shows
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// syncWriterChildEnv marks the child process that runs the sync. --dry-run
// can switch process-wide state on (a read-only store, say) that would leak
// into the package's other tests, so the sync runs in a process of its own.
const syncWriterChildEnv = "MSP_SKILLS_SYNC_WRITER_CHILD"

// TestSyncWorkerPoolSerializesEventWrites runs the real sync command with four
// workers over its default resources (every resource it knows, when it has
// only one default), in machine and human output, under --dry-run so no
// request leaves the process. It fails if two workers ever wrote at once.
func TestSyncWorkerPoolSerializesEventWrites(t *testing.T) {
	if os.Getenv(syncWriterChildEnv) == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		name := "TestSyncWorkerPoolSerializesEventWrites"
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+name+"$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), syncWriterChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		switch {
		case err != nil:
			t.Fatalf("%s in its own process: %v\n%s", name, err, out)
		case bytes.Contains(out, []byte("--- SKIP: "+name)):
			t.Skipf("%s", out)
		case !bytes.Contains(out, []byte("--- PASS: "+name)):
			t.Fatalf("%s in its own process did not report a pass:\n%s", name, out)
		}
		return
	}
	home := t.TempDir()
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, home)
	}
	// The press verifier forces a single worker; this test needs the pool.
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	names := defaultSyncResources()
	if len(names) < 2 {
		names = knownSyncResourceNames()
	}
	if len(names) < 2 {
		t.Skipf("sync knows %d resource(s), so no two of its workers ever run at once", len(names))
	}
	// A dry run never creates the store, and some prints refuse to sync into
	// one that does not exist yet (and, once a dry run has run, refuse to
	// create one in this process), so every store is made before any sync.
	modes := []string{"--json", "", "--json", "", "--json", ""}
	dbs := make([]string, len(modes))
	for i := range modes {
		dbs[i] = filepath.Join(home, fmt.Sprintf("sync-%d.db", i))
		s, err := store.OpenWithContext(context.Background(), dbs[i])
		if err != nil {
			t.Fatalf("creating the local store: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("closing the local store: %v", err)
		}
	}
	for i, mode := range modes {
		w := &syncOverlapWriter{}
		cmd := RootCmd()
		cmd.SetOut(w)
		cmd.SetErr(w)
		args := []string{"sync", "--db", dbs[i],
			"--resources", strings.Join(names, ","), "--dry-run", "--rate-limit", "0", "--concurrency", "4"}
		if mode != "" {
			args = append(args, mode)
		}
		cmd.SetArgs(args)
		_ = cmd.Execute() // a dry run may report per-resource errors; only the writes matter here
		if n := w.overlaps.Load(); n != 0 {
			t.Fatalf("sync %q, run %d: %d overlapping writes from concurrent sync workers", mode, i, n)
		}
		if n := strings.Count(w.buf.String(), `"event":"sync_start"`); n < 2 {
			// The ledger's asserts still pin the wrap in sync.go.
			first, _, _ := strings.Cut(strings.TrimSpace(w.buf.String()), "\n")
			t.Skipf("--dry-run does not reach this print's worker pool (%d resource(s) started): %s", n, first)
		}
	}
}
