// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"sync"
)

// syncSerialWriter lets sync's parallel workers share one event writer.
// Hand-fix (handfixes.json sync-event-writer-serialized), upstream as
// mvanhorn/cli-printing-press#5369; tools/maintainer/replay_handfixes.py
// writes this file from handfix_replay/sync_event_writer.go.tmpl after a
// reprint. Only os.Stdout is safe for concurrent Write calls. The writers
// --deliver (a copy into a bytes.Buffer) and --receipt-file / --audit-dir (a
// running SHA-256 of the output) install are not: two workers writing at once
// could panic in bytes.Buffer.grow or leave a receipt hash that does not match
// the output.
type syncSerialWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write hands p to the wrapped writer, one call at a time.
func (s *syncSerialWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
