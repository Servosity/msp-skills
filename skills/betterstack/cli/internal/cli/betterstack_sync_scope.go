// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// betterstack handfix (class incomplete-snapshot-read-as-complete, filtered
// walk): a sync that sent
// a filter (--param, --resource-param or --global-param: GET /incidents
// from, to, monitor_id, heartbeat_id, resolved and acknowledged, GET /monitors
// url and pronounceable_name, or a paging key) finishes as a complete walk of
// what the filter matched, and its sync_state says so: the press keeps ordinary
// filters out of the checkpoint scope by design. The novels read that state as
// the whole account, so `sync --resources incidents --param resolved=true` then
// `triage` reported no open incidents.
//
// The sync command records, per resource, that its walk was filtered (the
// parameter names, never their values) before the walk starts, and clears the
// record only when an unfiltered walk of that resource then finishes: the
// checkpoint is set incomplete before that walk and read back after it.
// requireFinishedSync refuses a resource with a record. A filtered sync that
// fails, or an unfiltered one that does not finish, leaves the record or an
// incomplete checkpoint: fail closed.
//
// Codex r1 (betterstack 4.34.2 reprint), two classes closed here, with the
// filter names above: a walk is the account's whole set only when it sends no
// filter and removes what the account no longer has. Allowlist, not a list of
// known filters: a walk is recorded as not a snapshot unless it is a full walk
// (--full, which every plain sync now is) that reconciles deletions (the
// resource is flat-reconcilable and --no-prune is off), with no --since, no
// --latest-only and no parameter.
//   - time-filtered-walk-read-as-complete: `sync --since 7d` sent GET
//     /incidents from=<date> (spec.yaml resources.incidents.endpoints.list.
//     params[from]) and every later plain sync sent from=<last sync>, so an
//     incident open since before the window was never stored and one stored
//     open was never seen resolved; triage reported both wrong.
//   - deleted-rows-read-as-current: no resource reconciled deletions (the
//     render's flatReconcileModes was empty; tools/apply_edits.py fills it),
//     so a monitor deleted in Better Stack stayed in the store after every
//     sync, --full included, and fleet reported it down.

// Codex r2 (same class): auto-refresh and workflow archive called syncResource
// directly, around the sync command's record, so an auto-refresh with
// from=<last sync> saved a complete checkpoint and triage kept a resolved
// incident open. The decision now sits where every walk runs: the generated
// walk is syncResourceWalk, and syncResource here wraps it, so no caller can go
// around it (a re-render that restores the generated name fails the build).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"betterstack-pp-cli/internal/store"
)

// syncFilteredTable is the store table of filtered walks (hand-owned; the
// generated schema never names it).
const syncFilteredTable = "betterstack_sync_filtered"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "sync" && c.RunE != nil {
				wrapSyncScope(c, flags)
			}
		}
	})
}

// wrapSyncScope makes every plain sync a full walk: it reconciles deletions and
// sends no incremental from= window (deleted-rows-read-as-current,
// time-filtered-walk-read-as-complete). --full=false keeps the incremental
// walk, which syncResource records as not a snapshot.
func wrapSyncScope(cmd *cobra.Command, flags *rootFlags) {
	inner := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if f := c.Flags().Lookup("full"); f != nil && !f.Changed && !dryRunOK(flags) {
			_ = f.Value.Set("true")
		}
		return inner(c, args)
	}
}

// syncResource is the one door to a walk of a resource. It records a walk that
// is not the account's whole set before any request goes out, and clears an
// earlier record only when a whole walk then finishes.
func syncResource(ctx context.Context, c interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
	RateLimit() float64
}, db *store.Store, resource, sinceTS string, full bool, maxPages int, latestOnly bool, prune bool, userParams *syncUserParams, syncEvents io.Writer) syncResult {
	if preview, ok := c.(interface{ IsDryRun() bool }); ok && preview.IsDryRun() {
		return syncResourceWalk(ctx, c, db, resource, sinceTS, full, maxPages, latestOnly, prune, userParams, syncEvents)
	}
	reasons := walkNotWhole(resource, sinceTS, full, latestOnly, prune, userParams)
	if len(reasons) > 0 {
		if err := recordNotWhole(db, resource, reasons); err != nil {
			return syncResult{Resource: resource, Err: fmt.Errorf("recording the scope of the %s walk: %w", resource, err)}
		}
	}
	res := syncResourceWalk(ctx, c, db, resource, sinceTS, full, maxPages, latestOnly, prune, userParams, syncEvents)
	if len(reasons) == 0 && res.Err == nil {
		if err := clearWholeWalk(db, resource); err != nil {
			res.Err = fmt.Errorf("clearing the scope record of the %s walk: %w", resource, err)
		}
	}
	return res
}

// walkNotWhole names why a walk is not the account's whole set. Allowlist: a
// walk is a snapshot only with no reason here. --param reaches every flat walk,
// --global-param every walk, --resource-param its resource's walk (applyTo).
func walkNotWhole(resource, sinceTS string, full, latestOnly, prune bool, params *syncUserParams) []string {
	var reasons []string
	if params != nil {
		sent := map[string]string{}
		params.applyTo(resource, sent, false) // betterstack syncs no path-scoped dependents
		for k := range sent {
			reasons = append(reasons, k)
		}
		sort.Strings(reasons)
	}
	if p := syncResourceSinceParam(resource); p != "" && sinceTS != "" {
		reasons = append(reasons, "--since ("+p+")")
	}
	if !full {
		reasons = append(reasons, "--full=false (an incremental walk that keeps deleted records)")
	}
	if !prune {
		reasons = append(reasons, "--no-prune (deleted records kept)")
	}
	if latestOnly {
		reasons = append(reasons, "--latest-only")
	}
	if !resourceIsFlatReconcilable(resourceReconcileMode(resource)) {
		reasons = append(reasons, "no deletion reconcile for "+resource)
	}
	return reasons
}

func recordNotWhole(s *store.Store, resource string, reasons []string) error {
	db := s.DB()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + syncFilteredTable + ` (resource_type TEXT PRIMARY KEY, params TEXT NOT NULL)`); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO `+syncFilteredTable+` (resource_type, params) VALUES (?, ?)`, resource, strings.Join(reasons, ","))
	return err
}

// clearWholeWalk clears the record of resource when its checkpoint says the
// walk finished. A walk that stopped part way (--max-pages, a cancelled
// context) leaves the record.
func clearWholeWalk(s *store.Store, resource string) error {
	if _, recorded, err := syncWalkFiltered(s, resource); err != nil || !recorded {
		return err
	}
	var complete int
	var synced sql.NullString
	db := s.DB()
	if err := db.QueryRow(`SELECT last_attempt_complete, CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, resource).Scan(&complete, &synced); err != nil {
		return err
	}
	if complete != 1 || !synced.Valid || synced.String == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM `+syncFilteredTable+` WHERE resource_type = ?`, resource)
	return err
}

// syncWalkFiltered reports the filter parameter names the last walk of
// resourceType sent, when it was filtered and no unfiltered walk finished
// since.
func syncWalkFiltered(s *store.Store, resourceType string) (string, bool, error) {
	var n int
	if _, err := bsQueryOne(s, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, []any{syncFilteredTable}, &n); err != nil || n == 0 {
		return "", false, err
	}
	var params string
	found, err := bsQueryOne(s, `SELECT params FROM `+syncFilteredTable+` WHERE resource_type = ?`, []any{resourceType}, &params)
	return params, found, err
}
