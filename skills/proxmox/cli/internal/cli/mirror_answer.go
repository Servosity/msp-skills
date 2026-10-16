// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: a per-endpoint read that answers from the local mirror because
// Proxmox could not be reached says so on stderr, in every output form; the
// JSON meta.source label alone is lost in --csv, --plain and --quiet.

package cli

import (
	"fmt"
	"io"
	"os"
	"time"
)

// rowsOnly: --csv, --plain and --quiet print the report's rows, not its
// envelope (whose first scalar --quiet would otherwise print).
func rowsOnly(f *rootFlags) bool { return f.csv || f.plain || f.quiet }

// printQuietRows is --quiet for a fleet command: one identity per row.
func printQuietRows[T any](w io.Writer, rows []T, id func(T) string) {
	for _, r := range rows {
		fmt.Fprintln(w, id(r))
	}
}

func warnMirrorAnswer(w io.Writer, prov DataProvenance) {
	if w == nil {
		w = os.Stderr
	}
	synced := "never synced"
	if prov.SyncedAt != nil {
		synced = "synced " + prov.SyncedAt.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(w, "warning: Proxmox could not be reached; this answer is from the local mirror (%s), not live. Pass --data-source live to refuse a mirror answer.\n", synced)
}
