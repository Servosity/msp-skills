// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Sync provenance for the fleet audits and trace.
//
// sync_state says only that the latest attempt of a resource completed. It
// cannot tell an unfiltered walk from one narrowed by --resource-param,
// --param, --global-param, --path-context, --since or --latest-only, and an
// ordinary (non --full) walk deletes nothing, so a row
// the API stopped returning survives it. The fleet commands draw conclusions
// from absence (removed, missing, zero, dangling), so they need to know which
// walk produced the rows.
//
// This file wraps the sync command (a registerNovelCommand hook, regen-safe).
// Around every non-dry-run sync it:
//   - scrubs credential fields an older build may have stored (once per store);
//   - holds store.BeginSyncReplace, so a walk replaces each row instead of the
//     keep-richer merge (a field the API now returns empty is cleared);
//   - logs every sync_state write through triggers, and afterwards records, for
//     each resource whose last logged write is a completed checkpoint, the walk's
//     start time and whether the walk was authoritative (unfiltered, not
//     incremental) in fleet_sync_provenance. A resource left incomplete loses
//     its record.
//
// A walk can span runs: a flat resource cut short (--max-pages, an error, an
// interrupt) keeps a resume cursor and the next run continues it. Its rows from
// the earlier run are part of the walk, so the walk's start is the run that
// began it (fleet_sync_walks), and any narrowing in any of its runs counts. A
// resumed walk whose start was not recorded is not authoritative.
//
// The event log cannot tell two concurrent sync runs apart, so a run that
// overlapped another (fleet_sync_runs) credits nothing as authoritative, and
// neither does the other.
//
// fleetCoverageOf (netsapiens_fleet.go) counts a resource as coverage only when
// its latest completed checkpoint is the one recorded here as authoritative,
// and for a dependent only when its parent is covered by a walk that started no
// later. Rows older than the covering walk's start were not returned by it and
// are not read.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/store"
)

const fleetProvenanceDDL = `CREATE TABLE IF NOT EXISTS fleet_sync_provenance (
	resource_type TEXT PRIMARY KEY,
	run_started TEXT NOT NULL,
	state_synced_at TEXT NOT NULL,
	authoritative INTEGER NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
)`

const fleetWalksDDL = `CREATE TABLE IF NOT EXISTS fleet_sync_walks (
	resource_type TEXT PRIMARY KEY,
	walk_started TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
)`

const fleetRunsDDL = `CREATE TABLE IF NOT EXISTS fleet_sync_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pid INTEGER NOT NULL,
	started TEXT NOT NULL,
	finished TEXT
)`

// fleetRunMaxAge bounds how long an unfinished run (a sync killed before it
// could record itself) counts as possibly running.
// ponytail: fixed 6h; a sync that runs longer is treated as gone.
const fleetRunMaxAge = 6 * time.Hour

var fleetSyncEventDDL = []string{
	`CREATE TABLE IF NOT EXISTS fleet_sync_events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_type TEXT NOT NULL,
		complete INTEGER NOT NULL,
		synced_at TEXT NOT NULL
	)`,
	`CREATE TRIGGER IF NOT EXISTS fleet_sync_state_ai AFTER INSERT ON sync_state BEGIN
		INSERT INTO fleet_sync_events (resource_type, complete, synced_at)
		VALUES (NEW.resource_type, NEW.last_attempt_complete, COALESCE(CAST(NEW.last_synced_at AS TEXT), ''));
	END`,
	`CREATE TRIGGER IF NOT EXISTS fleet_sync_state_au AFTER UPDATE ON sync_state BEGIN
		INSERT INTO fleet_sync_events (resource_type, complete, synced_at)
		VALUES (NEW.resource_type, NEW.last_attempt_complete, COALESCE(CAST(NEW.last_synced_at AS TEXT), ''));
	END`,
	`CREATE TRIGGER IF NOT EXISTS fleet_sync_state_ad AFTER DELETE ON sync_state BEGIN
		INSERT INTO fleet_sync_events (resource_type, complete, synced_at) VALUES (OLD.resource_type, 0, '');
	END`,
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "sync" && c.RunE != nil {
				wrapSyncWithProvenance(c, flags)
			}
		}
	})
}

func wrapSyncWithProvenance(syncCmd *cobra.Command, flags *rootFlags) {
	run := syncCmd.RunE
	syncCmd.RunE = func(c *cobra.Command, args []string) error {
		if flags != nil && flags.dryRun {
			return run(c, args)
		}
		dbPath, _ := c.Flags().GetString("db")
		if dbPath == "" {
			dbPath = defaultDBPath("netsapiens-cli")
		}
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		startSeq, runID, before, prepErr := prepareSyncProvenance(ctx, dbPath)
		if prepErr != nil {
			// Without the event log nothing can be credited; the run still
			// happens and every resource it touches is left unrecorded.
			fmt.Fprintf(c.ErrOrStderr(), "warning: sync provenance unavailable (%v); fleet audits will treat this run as incomplete\n", prepErr)
		}
		// Timestamps have one-second resolution: a walk starts on the next whole
		// second, so no row an earlier walk wrote can carry this walk's start.
		started := time.Now().UTC().Truncate(time.Second).Add(time.Second)
		time.Sleep(time.Until(started))
		done := store.BeginSyncReplace()
		runErr := run(c, args)
		done()
		if prepErr == nil {
			if err := recordSyncProvenance(ctx, dbPath, startSeq, runID, before, started, syncRunNarrowing(c)); err != nil {
				fmt.Fprintf(c.ErrOrStderr(), "warning: recording sync provenance: %v; fleet audits will treat this run as incomplete\n", err)
			}
		}
		return runErr
	}
}

// syncCheckpoint is a resource's sync_state as the run found it.
type syncCheckpoint struct {
	cursor, scope string
	complete      bool
}

// prepareSyncProvenance installs the event log,
// registers this run, and returns the last logged event (only later writes
// are read afterwards) plus every checkpoint as the run found it.
func prepareSyncProvenance(ctx context.Context, dbPath string) (int64, int64, map[string]syncCheckpoint, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return 0, 0, nil, err
	}
	defer db.Close()
	for _, ddl := range append([]string{fleetProvenanceDDL, fleetWalksDDL, fleetRunsDDL}, fleetSyncEventDDL...) {
		if _, err := db.DB().ExecContext(ctx, ddl); err != nil {
			return 0, 0, nil, err
		}
	}
	before, err := readSyncCheckpoints(ctx, db)
	if err != nil {
		return 0, 0, nil, err
	}
	var seq sql.NullInt64
	if err := db.DB().QueryRowContext(ctx, `SELECT MAX(seq) FROM fleet_sync_events`).Scan(&seq); err != nil {
		return 0, 0, nil, err
	}
	others, err := overlappingRuns(ctx, db, 0, time.Now().UTC())
	if err != nil {
		return 0, 0, nil, err
	}
	if !others {
		// No other run is open, so older events belong to no one; drop them.
		if _, err := db.DB().ExecContext(ctx, `DELETE FROM fleet_sync_events WHERE seq <= ?`, seq.Int64); err != nil {
			return 0, 0, nil, err
		}
	}
	res, err := db.DB().ExecContext(ctx, `INSERT INTO fleet_sync_runs (pid, started) VALUES (?, ?)`, os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, 0, nil, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, 0, nil, err
	}
	return seq.Int64, runID, before, nil
}

// overlappingRuns reports whether a sync run other than self was open at any
// point since 'since': unfinished and plausibly alive, or finished after since.
func overlappingRuns(ctx context.Context, db *store.Store, self int64, since time.Time) (bool, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT pid, started, COALESCE(finished, '') FROM fleet_sync_runs WHERE id != ?`, self)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var started, finished string
		if err := rows.Scan(&pid, &started, &finished); err != nil {
			return false, err
		}
		if finished != "" {
			if f, err := time.Parse(time.RFC3339Nano, finished); err != nil || !f.Before(since) {
				return true, nil
			}
			continue
		}
		if st, err := time.Parse(time.RFC3339Nano, started); err != nil || (time.Since(st) < fleetRunMaxAge && pidAlive(int(pid))) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// pidAlive reports whether a process with this pid exists (best effort; a
// reused pid reads as alive, which only withholds credit).
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true // FindProcess opens a handle and fails for a pid that does not exist
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func readSyncCheckpoints(ctx context.Context, db *store.Store) (map[string]syncCheckpoint, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT resource_type, COALESCE(last_cursor, ''), COALESCE(scope_key, ''), COALESCE(last_attempt_complete, 0) FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]syncCheckpoint{}
	for rows.Next() {
		var rt string
		var cp syncCheckpoint
		var complete int
		if err := rows.Scan(&rt, &cp.cursor, &cp.scope, &complete); err != nil {
			return nil, err
		}
		cp.complete = complete == 1
		out[rt] = cp
	}
	return out, rows.Err()
}

// continuesWalk reports whether this run resumed rt's earlier, unfinished walk
// rather than starting a new one: the run found an incomplete checkpoint with
// a resume cursor, did not clear it (--full, --latest-only) and kept its scope
// (a scope change clears the cursor).
func continuesWalk(rt string, before, after map[string]syncCheckpoint, cleared bool) bool {
	b, ok := before[rt]
	if !ok || b.complete || b.cursor == "" || cleared {
		return false
	}
	return after[rt].scope == b.scope
}

// syncRunNarrowing lists what made this run less than a complete walk of every
// resource it touched. Any entry makes every resource it completed non-authoritative.
func syncRunNarrowing(c *cobra.Command) []string {
	var why []string
	for _, name := range []string{"param", "resource-param", "global-param", "path-context"} {
		if vals, err := c.Flags().GetStringArray(name); err != nil || len(vals) > 0 {
			why = append(why, "--"+name)
		}
	}
	if v, err := c.Flags().GetString("since"); err != nil || strings.TrimSpace(v) != "" {
		why = append(why, "--since")
	}
	// --max-pages and the dogfood page caps are not listed: a capped walk is
	// left incomplete by sync itself and its resume is tracked as one walk.
	if v, err := c.Flags().GetBool("latest-only"); err != nil || v {
		why = append(why, "--latest-only")
	}
	if full, err := c.Flags().GetBool("full"); err != nil || !full {
		why = append(why, "incremental:not-full") // only applies to resources with an incremental filter
	}
	return why
}

// resourceNarrowing is the run's narrowing that applies to one resource.
func resourceNarrowing(rt string, run []string) []string {
	var out []string
	for _, w := range run {
		if w == "incremental:not-full" {
			if syncResourceSinceParam(rt) != "" {
				out = append(out, "incremental (no --full)")
			}
			continue
		}
		out = append(out, w)
	}
	if syncResourceHasDateWindow(rt) {
		out = append(out, "date window")
	}
	return out
}

func recordSyncProvenance(ctx context.Context, dbPath string, startSeq, runID int64, before map[string]syncCheckpoint, started time.Time, narrowing []string) error {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	// Close this run first, so a run starting now sees it finished.
	finished := time.Now().UTC()
	if _, err := db.DB().ExecContext(ctx, `UPDATE fleet_sync_runs SET finished = ? WHERE id = ?`, finished.Format(time.RFC3339Nano), runID); err != nil {
		return err
	}
	var mine string
	if err := db.DB().QueryRowContext(ctx, `SELECT started FROM fleet_sync_runs WHERE id = ?`, runID).Scan(&mine); err != nil {
		return err
	}
	since, err := time.Parse(time.RFC3339Nano, mine)
	if err != nil {
		return err
	}
	overlap, err := overlappingRuns(ctx, db, runID, since)
	if err != nil {
		return err
	}
	if overlap {
		narrowing = append(narrowing, "a concurrent sync run")
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT seq, resource_type, complete, synced_at FROM fleet_sync_events WHERE seq > ? ORDER BY seq`, startSeq)
	if err != nil {
		return err
	}
	type last struct {
		complete bool
		syncedAt string
	}
	final := map[string]last{}
	var maxSeq int64
	for rows.Next() {
		var seq int64
		var rt, at string
		var complete int
		if err := rows.Scan(&seq, &rt, &complete, &at); err != nil {
			_ = rows.Close()
			return err
		}
		final[rt] = last{complete: complete == 1, syncedAt: at}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	after, err := readSyncCheckpoints(ctx, db)
	if err != nil {
		return err
	}
	// --full and --latest-only clear the resume cursor before walking.
	cleared := containsString(narrowing, "--latest-only") || !containsString(narrowing, "incremental:not-full")
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	names := make([]string, 0, len(final))
	for rt := range final {
		names = append(names, rt)
	}
	sort.Strings(names)
	for _, rt := range names {
		f := final[rt]
		// The walk this run advanced: its start and its narrowing so far.
		walkStart, why := started.Format(time.RFC3339), resourceNarrowing(rt, narrowing)
		if continuesWalk(rt, before, after, cleared) {
			var prevStart, prevWhy string
			if err := tx.QueryRowContext(ctx, `SELECT walk_started, detail FROM fleet_sync_walks WHERE resource_type = ?`, rt).Scan(&prevStart, &prevWhy); err != nil {
				prevStart, prevWhy = walkStart, "resumed a walk begun outside a recorded sync"
			}
			walkStart = prevStart
			if prevWhy != "" {
				why = append(strings.Split(prevWhy, ", "), why...)
			}
		}
		if !f.complete || f.syncedAt == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_sync_provenance WHERE resource_type = ?`, rt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO fleet_sync_walks (resource_type, walk_started, detail) VALUES (?, ?, ?)`, rt, walkStart, strings.Join(why, ", ")); err != nil {
				return err
			}
			continue
		}
		auth := 0
		if len(why) == 0 {
			auth = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO fleet_sync_provenance (resource_type, run_started, state_synced_at, authoritative, detail) VALUES (?, ?, ?, ?, ?)`,
			rt, walkStart, f.syncedAt, auth, strings.Join(why, ", ")); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_sync_walks WHERE resource_type = ?`, rt); err != nil {
			return err
		}
	}
	if !overlap {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_sync_events WHERE seq <= ?`, maxSeq); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_sync_runs WHERE finished IS NOT NULL AND finished < ?`, finished.Add(-24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// fleetProvenance is the recorded walk behind a resource's latest completed checkpoint.
type fleetProvenance struct {
	runStarted    time.Time
	stateSyncedAt string
	authoritative bool
	detail        string
}

func readFleetProvenance(db *store.Store, rt string) (fleetProvenance, bool) {
	var p fleetProvenance
	var started string
	var auth int
	err := db.DB().QueryRow(`SELECT run_started, state_synced_at, authoritative, detail FROM fleet_sync_provenance WHERE resource_type = ?`, rt).
		Scan(&started, &p.stateSyncedAt, &auth, &p.detail)
	if err != nil {
		return p, false
	}
	t, ok := parseNSTime(started)
	if !ok {
		return p, false
	}
	p.runStarted, p.authoritative = t.UTC(), auth == 1
	return p, true
}

// fleetDependentParent maps a dependent sync resource to the parent table its
// walk enumerates.
var fleetDependentParent = func() map[string]string {
	m := map[string]string{}
	for _, d := range dependentResourceDefs() {
		m[d.Name] = d.ParentTable
	}
	return m
}()
