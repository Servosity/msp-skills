// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored shared plumbing for the fleet-* novel commands (not generator-emitted).
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
	"skykick-pp-cli/internal/store"
)

// cmdContext returns the command's context, falling back to Background when
// RunE is invoked outside Execute (direct calls in tests) where cobra has not
// seeded a context yet.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// openFleetStore opens the local store at the conventional path (or --db
// override) for fleet reads/writes.
func openFleetStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("skykick-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return db, nil
}

// errNoFleetRuns is the friendly guidance returned when a fleet command runs
// before any fleet-sync has populated the store.
func errNoFleetRuns() error {
	return notFoundErr(fmt.Errorf("no completed fleet-sync runs in the local store; run `skykick-cli fleet-sync` first"))
}

// latestFleetState loads the newest finished fleet run and its full state.
func latestFleetState(ctx context.Context, db *store.Store) (fleet.FleetState, store.FleetRun, error) {
	if err := db.EnsureFleetSchema(ctx); err != nil {
		return fleet.FleetState{}, store.FleetRun{}, err
	}
	runs, err := db.LatestFleetRuns(ctx, 1)
	if err != nil {
		return fleet.FleetState{}, store.FleetRun{}, err
	}
	if len(runs) == 0 {
		return fleet.FleetState{}, store.FleetRun{}, errNoFleetRuns()
	}
	state, err := db.LoadFleetState(ctx, runs[0].ID)
	if err != nil {
		return fleet.FleetState{}, store.FleetRun{}, err
	}
	return state, runs[0], nil
}

// latestTwoFleetStates loads the two newest finished runs for drift.
func latestTwoFleetStates(ctx context.Context, db *store.Store) (prev, cur fleet.FleetState, prevRun, curRun store.FleetRun, err error) {
	if err = db.EnsureFleetSchema(ctx); err != nil {
		return
	}
	runs, lerr := db.LatestFleetRuns(ctx, 2)
	if lerr != nil {
		err = lerr
		return
	}
	if len(runs) == 0 {
		err = errNoFleetRuns()
		return
	}
	if len(runs) < 2 {
		// One run is a valid state for drift: the command reports "no prior
		// run to compare" honestly instead of failing. Caller checks
		// prevRun.ID == 0.
		curRun = runs[0]
		cur, err = db.LoadFleetState(ctx, curRun.ID)
		return
	}
	curRun, prevRun = runs[0], runs[1]
	if cur, err = db.LoadFleetState(ctx, curRun.ID); err != nil {
		return
	}
	prev, err = db.LoadFleetState(ctx, prevRun.ID)
	return
}

// fleetEnvelopeMeta is embedded in every fleet command's JSON envelope so
// agents can tell how fresh the underlying data is.
type fleetEnvelopeMeta struct {
	RunID    int64  `json:"run_id"`
	SyncedAt string `json:"synced_at"`
}

func metaForRun(run store.FleetRun) fleetEnvelopeMeta {
	return fleetEnvelopeMeta{RunID: run.ID, SyncedAt: run.FinishedAt}
}

// fleetPrint emits v as JSON (honoring --select/--compact/--csv/--quiet) when
// any machine-output mode is active or stdout is piped; otherwise it renders
// the humanRows table followed by a one-line summary.
func fleetPrint(cmd *cobra.Command, flags *rootFlags, v any, humanRows []map[string]any, summary string) error {
	out := cmd.OutOrStdout()
	if flags.asJSON || flags.agent || flags.csv || flags.quiet || flags.selectFields != "" || flags.compact || !isTerminal(out) {
		return printJSONFiltered(out, v, flags)
	}
	if len(humanRows) > 0 {
		if err := printAutoTable(out, humanRows); err != nil {
			return err
		}
	}
	if summary != "" {
		fmt.Fprintln(out, summary)
	}
	return nil
}

// triStateWord renders *bool for human tables.
func triStateWord(b *bool) string {
	switch {
	case b == nil:
		return "unknown"
	case *b:
		return "on"
	default:
		return "off"
	}
}
