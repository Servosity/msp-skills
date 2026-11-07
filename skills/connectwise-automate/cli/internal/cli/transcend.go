// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/store"
)

// openTranscendStore opens the local SQLite mirror the transcendence commands
// read from, creating the data directory (and an empty DB) if it does not yet
// exist. An empty/just-created store yields empty roll-ups rather than an error,
// so the commands stay safe to run before the first `sync`.
func openTranscendStore(cmd *cobra.Command) (*store.Store, error) {
	dbPath := defaultDBPath("connectwise-automate-cli")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir %s: %w", filepath.Dir(dbPath), err)
	}
	return store.OpenWithContext(cmd.Context(), dbPath)
}

// loadResource returns the rows of one synced resource that the latest
// finished, unfiltered sync walk returned (cwa_walk.go records it), as decoded
// maps (json.Number-preserving), ready for the fleet aggregation functions.
// Every fleet report reads through here, so one rule decides what counts as
// the fleet:
//   - a resource with no rows and no walk reads as empty (the reports stay
//     safe to run before the first sync);
//   - a resource whose last unfiltered walk finished keeps exactly the records
//     that walk returned (with their latest stored data): a record the API
//     stopped returning, a live read's write-through, or a row from another
//     server is not current, even though a sync without --full never prunes it;
//   - anything else (no unfiltered walk has finished, or one is in progress or
//     was interrupted) is an incomplete mirror: the report fails, naming the
//     resource, unless --allow-partial-failure is set, in which case every row
//     is used and a warning says the report is incomplete.
func loadResource(cmd *cobra.Command, flags *rootFlags, s *store.Store, resourceType string) ([]map[string]any, error) {
	walk, walked, err := cwaWalkRead(cmd.Context(), s.DB(), resourceType)
	if err != nil {
		return nil, fmt.Errorf("reading %s sync walk: %w", resourceType, err)
	}
	finished := walked && walk.state == cwaWalkFinished
	member := make(map[string]bool, len(walk.members))
	for _, id := range walk.members {
		member[id] = true
	}
	rows, err := s.DB().QueryContext(cmd.Context(),
		`SELECT id, data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
	}
	defer rows.Close()
	var out, other []map[string]any
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
		}
		obj, err := store.DecodeJSONObject([]byte(data))
		if err != nil {
			continue
		}
		if finished && !member[id] {
			other = append(other, obj)
			continue
		}
		out = append(out, obj)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
	}
	if finished || (!walked && len(out) == 0) {
		return out, nil
	}
	why, fix := "no unfiltered sync of it has walked to the end (its rows come from a filtered sync or live reads)", "sync --full --resources "+resourceType
	switch {
	case walked && walk.state == cwaWalkWalking:
		why, fix = "its last sync stopped before the end of the list", "sync --resources "+resourceType
	case walked:
		why = "its last sync walk was interrupted"
	}
	if !flags.allowPartialFailure {
		return nil, fmt.Errorf("local %s mirror is incomplete: %s; run 'connectwise-automate-cli %s' to finish it, or pass --allow-partial-failure to report on it anyway", resourceType, why, fix)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: report is incomplete: local %s mirror is incomplete (%s)\n", resourceType, why)
	return append(out, other...), nil
}

// emitResult prints a Go-typed result through the shared output pipeline so the
// transcendence commands pick up --json, --select, --compact, --csv, and
// --quiet for free.
func emitResult(cmd *cobra.Command, flags *rootFlags, v any) error {
	return printJSONFiltered(cmd.OutOrStdout(), v, flags)
}
