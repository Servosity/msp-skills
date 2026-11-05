// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Codex round 3 on the zammad 4.34.0 reprint
// (2026-10-24). Pinned by this connector's handfixes.json under the
// "credkit-wiring" entry.

package cli

import (
	"path/filepath"
	"testing"
)

// TestR4LocalWorkflowDryRunNeedsNoStore: load and orphans opened the store
// read-only before honoring --dry-run, so a dry run before the first sync
// failed (press verify, dry-run column). They now return first, as overdue does.
func TestR4LocalWorkflowDryRunNeedsNoStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.db")
	for _, c := range []string{"load", "orphans"} {
		if _, _, err := runR1(t, c, "--dry-run", "--db", missing); err != nil {
			t.Errorf("%s --dry-run without a store: %v", c, err)
		}
	}
}
