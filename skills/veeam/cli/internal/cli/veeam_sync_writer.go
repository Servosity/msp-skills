// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"sync"
)

// serialWriter lets sync's parallel workers share one event writer
// (handfixes.json sync-event-writer-serialized). Only os.Stdout is safe for
// concurrent Write calls; the writers --deliver (mirrorWriter into a
// bytes.Buffer) and --receipt-file / --audit-dir (a running SHA-256) install
// are not, and two workers writing at once corrupted them.
type serialWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *serialWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
