// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// overlapWriter records whether two Write calls were ever in flight at once.
// The writers sync hands its workers in a real run are not all safe for that:
// --deliver copies output into a bytes.Buffer (mirrorWriter) and
// --receipt-file / --audit-dir hash it (platform outputCapture). Windows CI
// panicked inside bytes.Buffer.grow on exactly this.
type overlapWriter struct {
	inFlight atomic.Int32
	overlaps atomic.Int32
	mu       sync.Mutex
	buf      bytes.Buffer
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.inFlight.Add(1) > 1 {
		w.overlaps.Add(1)
	}
	defer w.inFlight.Add(-1)
	time.Sleep(time.Millisecond) // widen the window so an unserialized pair shows
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// TestSyncWorkersSerializeEventWrites runs the real default sync, four workers,
// in both output modes and fails if two workers ever wrote at once.
func TestSyncWorkersSerializeEventWrites(t *testing.T) {
	for _, mode := range [][]string{{"--json"}, {}} {
		_, db := startWalkFake(t)
		w := &overlapWriter{}
		cmd := RootCmd()
		cmd.SetOut(w)
		cmd.SetErr(w)
		args := append([]string{"sync", "--db", db, "--rate-limit", "0", "--concurrency", "4"}, mode...)
		cmd.SetArgs(args)
		_ = cmd.Execute() // empty partitions may warn; only the writes matter here
		if n := w.overlaps.Load(); n != 0 {
			t.Errorf("%v: %d overlapping writes from concurrent sync workers", args, n)
		}
	}
}
