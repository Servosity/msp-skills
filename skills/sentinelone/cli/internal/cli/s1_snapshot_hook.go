// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/store"
)

// The history novels (whatchanged, versions rollout, threats verdicts) diff
// fleet_snapshots captures, and store.CaptureFleetSnapshot existed from the
// first print, but nothing ever called it: every one of them answered "needs
// at least 2 syncs" no matter how often the operator synced. This hook wraps
// the generated `sync` command (no generated file is edited, so a reprint
// keeps it).
//
// A capture claims "this was the whole fleet at this moment", so it is only
// recorded for a walk that re-read every record, unfiltered:
//
//   - The generated incremental sync filters agents and threats on
//     createdAt__gte (spec.yaml /agents and /threats: records created at or
//     after the last sync), which never refreshes a record that already
//     exists. Before an ordinary sync, the hook drops the watermark of each
//     snapshot entity the run will walk, so that walk is unbounded and the
//     local store holds current state, not first-sync state.
//   - --since, --param, --global-param, a --resource-param for the entity,
//     --max-pages, --latest-only, the dogfood page cap and --dry-run all walk
//     a subset (or nothing): no capture, and the watermark is left alone.
//   - The capture holds exactly the rows THIS invocation wrote, never rows
//     picked by a timestamp: synced_at is a second-resolution wall-clock
//     stamp, so a record an earlier walk wrote in the same second (or before
//     the clock stepped back) and the API no longer returns would read as
//     current. The hook registers a walk before the sync opens its store
//     (store.BeginFleetWalk); every connection the sync then opens records
//     the agents/threats rows and the sync_state it writes under the walk's
//     id, and only that record becomes the capture.
//   - Completion is proven per invocation too: an entity is captured only when
//     this walk's own sync_state writes show it started and finished the read.
//   - When the walk cannot prove its record is complete, nothing is recorded
//     and the run says so (fail closed: no capture beats a wrong capture).
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() != "sync" || c.RunE == nil {
				continue
			}
			run := c.RunE
			c.RunE = func(cmd *cobra.Command, args []string) error {
				dbPath, _ := cmd.Flags().GetString("db")
				if dbPath == "" {
					dbPath = defaultDBPath("sentinelone-cli") // the path the generated sync resolves too
				}
				entities := fleetHistoryEntities(cmd, flags)
				var walk *store.FleetWalk
				if len(entities) > 0 {
					w, err := beginFleetWalk(cmd, dbPath, entities)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: history snapshot not recorded: %v\n", err)
					} else {
						walk = w
						defer walk.Stop() // never leave the recorder armed past this run
					}
				}
				runErr := run(cmd, args)
				if walk == nil {
					return runErr
				}
				// Disarm before any other store is opened: from here on no
				// connection records into this walk.
				walk.Stop()
				if runErr != nil {
					abandonFleetWalk(cmd, dbPath, walk)
					return runErr
				}
				if err := captureFleetHistory(cmd, dbPath, walk, entities); err != nil {
					// Best effort: the sync itself succeeded and its data is stored.
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: history snapshot not recorded: %v\n", err)
				}
				return nil
			}
		}
	})
}

// fleetHistoryEntities returns the snapshot entities this sync invocation
// walks in full and unfiltered, or nothing when the invocation cannot be
// proven to cover the whole fleet. Any flag it cannot read counts as a
// narrowing (fail closed).
func fleetHistoryEntities(cmd *cobra.Command, flags *rootFlags) []string {
	if flags != nil && flags.dryRun {
		return nil
	}
	if cliutil.IsDogfoodEnv() {
		return nil
	}
	f := cmd.Flags()
	if v, err := f.GetString("since"); err != nil || strings.TrimSpace(v) != "" {
		return nil
	}
	if v, err := f.GetInt("max-pages"); err != nil || v != 0 {
		return nil
	}
	if v, err := f.GetBool("latest-only"); err != nil || v {
		return nil
	}
	for _, name := range []string{"param", "global-param"} {
		if v, err := f.GetStringArray(name); err != nil || len(v) > 0 {
			return nil
		}
	}
	narrowed := map[string]bool{}
	resourceParams, err := f.GetStringArray("resource-param")
	if err != nil {
		return nil
	}
	for _, rp := range resourceParams {
		resource, _, _ := strings.Cut(rp, ":")
		narrowed[strings.TrimSpace(resource)] = true
	}
	resources, err := f.GetStringSlice("resources")
	if err != nil {
		return nil
	}
	if len(resources) == 0 {
		resources = defaultSyncResources()
	}
	walked := map[string]bool{}
	for _, r := range resources {
		walked[strings.TrimSpace(r)] = true
	}
	var out []string
	for _, entity := range store.SnapshotEntities {
		if walked[entity] && !narrowed[entity] {
			out = append(out, entity)
		}
	}
	return out
}

// beginFleetWalk drops each planned entity's watermark (so the walk is
// unbounded) and registers the walk. The store is opened, and closed, before
// the sync opens its own, so every connection the sync uses records the walk.
func beginFleetWalk(cmd *cobra.Command, dbPath string, entities []string) (*store.FleetWalk, error) {
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	for _, entity := range entities {
		if err := db.ResetSyncWatermark(entity); err != nil {
			return nil, err
		}
	}
	return db.BeginFleetWalk()
}

// abandonFleetWalk discards a walk whose sync failed. A walk it cannot discard
// is reaped later and can never be captured.
func abandonFleetWalk(cmd *cobra.Command, dbPath string, walk *store.FleetWalk) {
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	_ = db.AbandonFleetWalk(walk) // best effort: the sync's own error is what the caller reports
}

// captureFleetHistory records a capture of exactly the rows this walk wrote,
// for every planned entity the walk started and completed. A partial walk, a
// skipped resource and a failed one record nothing. An error means the walk
// could not prove its record (nothing was recorded).
func captureFleetHistory(cmd *cobra.Command, dbPath string, walk *store.FleetWalk, entities []string) error {
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.CaptureFleetWalk(walk, time.Now(), entities)
	return err
}
