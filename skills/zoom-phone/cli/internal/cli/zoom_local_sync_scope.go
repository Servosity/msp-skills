// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Sync coverage provenance for the slice-A ("local") commands.
//
// sync records a completed checkpoint per resource name whether or not the
// run was narrowed by --param / --resource-param / --global-param, so
// `sync --resource-param users:site_id=site-A` looks exactly like a full
// users sync in sync_state. The local audits draw conclusions from absence
// (a number's assignee missing from users, a user with no address), and a
// filtered sync makes those conclusions false. This file wraps the `sync`
// command (regen-safe: a registerNovelCommand hook in a hand-written file)
// and, after each run, records in a small zoom_sync_scope table whether the
// latest completed sync of each resource was filtered (and with which
// params) and whether it was a full reconciled walk. Ordinary syncs replace
// the rows the API returns but do not delete rows it no longer returns; only
// `sync --full` (without --no-prune, --latest-only, --max-pages or --since)
// prunes them. localCoverageFor then counts a resource as covering the
// account only after a completed, full, unfiltered sync.
//
// Which resources a run completed is never inferred from timestamps: two
// SQLite triggers installed on sync_state log every checkpoint write to
// zoom_sync_events, and a resource counts as completed by this run only when
// this run wrote its checkpoint and the last write is a completed one. A run
// that fails before touching a checkpoint (e.g. --resource-param typo) writes
// nothing and records nothing.
//
// Two kinds of filtered walk are useful on their own and are recorded as
// "walks" in zoom_sync_walks, with the exact row ids each walk wrote (logged
// by triggers on the resources table into zoom_row_events):
//   - locations site_id=X: /phone/locations needs site_id when Multiple
//     Sites is enabled, so locations can only be walked site by site there;
//     e911 audit accepts the union of per-site walks as location coverage.
//   - numbers type=byoc: only the type=byoc list carries the BYOC carrier
//     fields the e911 BYOC check reads. The walk is additive (press 4.34.2
//     never prunes a filtered walk, --full or not), it does not replace the
//     unfiltered numbers coverage record, and the BYOC check reads only the
//     rows that walk wrote, so a number that has left BYOC cannot linger.

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/store"
)

const localSyncScopeDDL = `CREATE TABLE IF NOT EXISTS zoom_sync_scope (
	resource    TEXT PRIMARY KEY,
	filtered    INTEGER NOT NULL,
	params      TEXT NOT NULL DEFAULT '',
	synced_at   TEXT NOT NULL DEFAULT '',
	recorded_at TEXT NOT NULL DEFAULT '',
	full        INTEGER NOT NULL DEFAULT 0
)`

// zoom_sync_walks: the latest completed filtered walk of a resource for one
// scope ("site_id=<id>" for locations, "type=byoc" for numbers), and
// zoom_sync_walk_rows: the row ids that walk wrote.
var localSyncWalkDDL = []string{
	`CREATE TABLE IF NOT EXISTS zoom_sync_walks (
		resource      TEXT NOT NULL,
		scope         TEXT NOT NULL,
		walk_complete INTEGER NOT NULL,
		full          INTEGER NOT NULL,
		synced_at     TEXT NOT NULL,
		recorded_at   TEXT NOT NULL,
		superseded    INTEGER NOT NULL DEFAULT 0,
		pending       INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (resource, scope)
	)`,
	`CREATE TABLE IF NOT EXISTS zoom_sync_walk_rows (
		resource TEXT NOT NULL,
		scope    TEXT NOT NULL,
		id       TEXT NOT NULL,
		PRIMARY KEY (resource, scope, id)
	)`,
}

// localWalkResourcesSQL are the resources whose row writes are logged.
const localWalkResourcesSQL = `('numbers', 'locations')`

// The event log and its triggers: every sync_state write appends one row.
var localSyncEventDDL = []string{
	`CREATE TABLE IF NOT EXISTS zoom_sync_events (
		seq       INTEGER PRIMARY KEY AUTOINCREMENT,
		resource  TEXT NOT NULL,
		complete  INTEGER,
		synced_at TEXT
	)`,
	`CREATE TRIGGER IF NOT EXISTS zoom_sync_state_ai AFTER INSERT ON sync_state BEGIN
		INSERT INTO zoom_sync_events (resource, complete, synced_at)
		VALUES (NEW.resource_type, NEW.last_attempt_complete, CAST(NEW.last_synced_at AS TEXT));
	END`,
	`CREATE TRIGGER IF NOT EXISTS zoom_sync_state_au AFTER UPDATE ON sync_state BEGIN
		INSERT INTO zoom_sync_events (resource, complete, synced_at)
		VALUES (NEW.resource_type, NEW.last_attempt_complete, CAST(NEW.last_synced_at AS TEXT));
	END`,
	`CREATE TABLE IF NOT EXISTS zoom_row_events (
		seq           INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_type TEXT NOT NULL,
		id            TEXT NOT NULL
	)`,
	`CREATE TRIGGER IF NOT EXISTS zoom_resources_ai AFTER INSERT ON resources
		WHEN NEW.resource_type IN ` + localWalkResourcesSQL + ` BEGIN
		INSERT INTO zoom_row_events (resource_type, id) VALUES (NEW.resource_type, NEW.id);
	END`,
	`CREATE TRIGGER IF NOT EXISTS zoom_resources_au AFTER UPDATE ON resources
		WHEN NEW.resource_type IN ` + localWalkResourcesSQL + ` BEGIN
		INSERT INTO zoom_row_events (resource_type, id) VALUES (NEW.resource_type, NEW.id);
	END`,
}

// localEnsureSyncScopeTable creates the scope tables, and adds the full
// column to a zoom_sync_scope written before it existed.
func localEnsureSyncScopeTable(db *store.Store) error {
	if _, err := db.DB().Exec(localSyncScopeDDL); err != nil {
		return err
	}
	if _, err := db.DB().Exec(`ALTER TABLE zoom_sync_scope ADD COLUMN full INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	for _, ddl := range localSyncWalkDDL {
		if _, err := db.DB().Exec(ddl); err != nil {
			return err
		}
	}
	for _, col := range []string{"superseded", "pending"} {
		if _, err := db.DB().Exec(`ALTER TABLE zoom_sync_walks ADD COLUMN ` + col + ` INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func localCmdContext(c *cobra.Command) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		var syncCmd *cobra.Command
		for _, c := range root.Commands() {
			if c.Name() == "sync" {
				syncCmd = c
			}
		}
		if syncCmd == nil || syncCmd.RunE == nil {
			return
		}
		run := syncCmd.RunE
		syncCmd.RunE = func(c *cobra.Command, args []string) error {
			if flags != nil && flags.dryRun {
				return run(c, args)
			}
			dbPath, _ := c.Flags().GetString("db")
			if dbPath == "" {
				dbPath = defaultDBPath(localCLIName)
			}
			// One sync per store at a time, across processes: the event logs
			// have no run identifier, so a second recorder would consume
			// (and delete) this run's evidence. Hold the lock for the whole
			// sync and recording; a second sync waits, never interleaves.
			// The wait for another sync honors an explicit --timeout; the
			// sync itself keeps the generated per-request timeout.
			waitCtx := localCmdContext(c)
			if flags != nil && flags.timeoutExplicit && flags.timeout > 0 {
				var cancel context.CancelFunc
				waitCtx, cancel = context.WithTimeout(waitCtx, flags.timeout)
				defer cancel()
			}
			return localWithSyncStoreLockCtx(waitCtx, dbPath, true, c.ErrOrStderr(), func() error {
				return localSyncWithProvenance(c, args, run, dbPath)
			})
		}
	})
}

// errLocalSyncBusy: the store's sync lock is held and the caller chose not
// to wait.
var errLocalSyncBusy = errors.New("another sync is running against the local store")

// localSyncLockPath is the canonical per-store sync lock (an absolute path,
// so every spelling of one store shares it).
func localSyncLockPath(dbPath string) string {
	return localCanonicalStorePath(dbPath) + ".zoom-sync"
}

// localCanonicalStorePath is the store's real path: absolute, with symlinks
// resolved, so a symlinked alias (alias.db -> actual.db) shares the real
// file's lock. A store that does not exist yet resolves through its
// directory.
func localCanonicalStorePath(dbPath string) string {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		abs = dbPath
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(dir, filepath.Base(abs))
	}
	return abs
}

// localCheckStoreLinks refuses a store file with more than one hard link: a
// hard-linked alias is a different path to the same database, so it would
// take a different lock while writing the same rows.
func localCheckStoreLinks(dbPath string) error {
	real := localCanonicalStorePath(dbPath)
	fi, err := os.Stat(real)
	if err != nil {
		return nil // not created yet
	}
	if n := localLinkCount(real, fi); n > 1 {
		return fmt.Errorf("refusing to use the local store %s: it has %d hard links, and a hard-linked alias would bypass the per-store sync lock; remove the extra links", real, n)
	}
	return nil
}

// localTryCacheWriteLock is for write-through caching from live reads: it
// takes the store's sync lock without waiting. ok=false means a sync is
// running (or the store is hard-linked), so the caller skips the cache write
// and still returns its live result; release must be called when ok.
func localTryCacheWriteLock(dbPath string) (release func(), ok bool) {
	if localCheckStoreLinks(dbPath) != nil {
		return func() {}, false
	}
	if localEnsureStoreDir(dbPath) != nil {
		return func() {}, false
	}
	r, ok, err := localTryHoldLock(localSyncLockPath(dbPath))
	if err != nil || !ok {
		return func() {}, false
	}
	return r, true
}

// localEnsureStoreDir creates the store's parent directory privately (0700)
// when it does not exist yet, so the lock beside the store can be created on
// an account's very first sync. An existing path must be a directory.
func localEnsureStoreDir(dbPath string) error {
	dir := filepath.Dir(localSyncLockPath(dbPath))
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700) // #nosec G302 -- a directory: owner-only rwx (0700) is the tightest usable mode
}

// localTryHoldLock makes ONE non-blocking attempt at lockPath. On success a
// goroutine holds the lock until release is called; on a busy lock nothing
// is left behind (TryWithFileLock never waits), so no attempt can acquire
// the lock after its caller has given up.
func localTryHoldLock(lockPath string) (release func(), ok bool, err error) {
	type attempt struct {
		ok  bool
		err error
	}
	acquired := make(chan attempt, 1)
	unlock := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ran, lockErr := cliutil.TryWithFileLock(lockPath, func() error {
			acquired <- attempt{ok: true}
			<-unlock
			return nil
		})
		if !ran || lockErr != nil {
			acquired <- attempt{err: lockErr}
		}
	}()
	if a := <-acquired; !a.ok {
		<-done
		return func() {}, false, a.err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			close(unlock)
			<-done
		})
	}, true, nil
}

// localWaitHoldLock retries localTryHoldLock with a short backoff until it
// succeeds or ctx ends, printing a waiting notice once. It never blocks on
// the lock itself, so cancellation returns promptly with ctx.Err().
func localWaitHoldLock(ctx context.Context, lockPath, dbPath string, stderr io.Writer) (func(), error) {
	backoff := 25 * time.Millisecond
	notified := false
	for {
		release, ok, err := localTryHoldLock(lockPath)
		if err != nil {
			return func() {}, fmt.Errorf("could not lock the local store for sync (%s): %w", lockPath, err)
		}
		if ok {
			return release, nil
		}
		if !notified && stderr != nil {
			fmt.Fprintf(stderr, "another sync is running against %s; waiting for it to finish before starting\n", dbPath)
			notified = true
		}
		select {
		case <-ctx.Done():
			return func() {}, fmt.Errorf("gave up waiting for another sync of %s to finish: %w", dbPath, ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < 500*time.Millisecond {
			backoff *= 2
		}
	}
}

// localAcquireSyncStoreLock takes the canonical per-store sync lock, waiting
// (cancellably, via ctx) like sync with the same notice, and returns its
// release. For callers whose locked region contains early returns, such as
// workflow archive's --full reset plus resource loop (hand-fix
// zoom-phone-sync-store-lock); code inside that region calls syncResource
// directly, never the lock again.
func localAcquireSyncStoreLock(ctx context.Context, dbPath string, stderr io.Writer) (release func(), err error) {
	if err := localCheckStoreLinks(dbPath); err != nil {
		return func() {}, err
	}
	if err := localEnsureStoreDir(dbPath); err != nil {
		return func() {}, fmt.Errorf("preparing the local store directory: %w", err)
	}
	return localWaitHoldLock(ctx, localSyncLockPath(dbPath), dbPath, stderr)
}

// localWithSyncStoreLock runs fn holding the canonical per-store sync lock
// shared by sync, the pre-read auto-refresh and workflow archive (the last
// two through hand-fixed call sites; see apply-handfixes.py
// zoom-phone-sync-store-lock). The lock is not reentrant: none of those
// callers runs inside another. With wait=false a held lock returns
// errLocalSyncBusy; with wait=true it prints a notice and waits until ctx
// ends (localWithSyncStoreLockCtx).
func localWithSyncStoreLock(dbPath string, wait bool, stderr io.Writer, fn func() error) error {
	return localWithSyncStoreLockCtx(context.Background(), dbPath, wait, stderr, fn)
}

func localWithSyncStoreLockCtx(ctx context.Context, dbPath string, wait bool, stderr io.Writer, fn func() error) error {
	if err := localCheckStoreLinks(dbPath); err != nil {
		return err
	}
	if err := localEnsureStoreDir(dbPath); err != nil {
		return fmt.Errorf("preparing the local store directory: %w", err)
	}
	lockPath := localSyncLockPath(dbPath)
	var release func()
	if wait {
		r, err := localWaitHoldLock(ctx, lockPath, dbPath, stderr)
		if err != nil {
			return err
		}
		release = r
	} else {
		r, ok, err := localTryHoldLock(lockPath)
		if err != nil {
			return fmt.Errorf("could not lock the local store for sync (%s): %w", lockPath, err)
		}
		if !ok {
			return errLocalSyncBusy
		}
		release = r
	}
	defer release()
	return fn()
}

// localSyncWithProvenance runs one sync and records its coverage. Callers
// hold the per-store sync lock.
func localSyncWithProvenance(c *cobra.Command, args []string, run func(*cobra.Command, []string) error, dbPath string) error {
	ctx := localCmdContext(c)
	prepared, prepErr := localPrepareScopedWalks(ctx, c, dbPath)
	mark, logErr := localBeginSyncEventLog(ctx, dbPath)
	err := run(c, args)
	if logErr == nil && prepErr != nil {
		logErr = prepErr
	}
	if logErr != nil {
		fmt.Fprintf(c.ErrOrStderr(), "warning: could not record which resources this sync covered (%v); local audits will report them unassessed\n", logErr)
		return err
	}
	mark.prepared = prepared
	if recErr := localRecordSyncScope(c, dbPath, mark); recErr != nil {
		// Nothing half-recorded is published: a scoped walk this run
		// attempted keeps its pending mark (and is reported as not
		// completed) until a later run records it.
		fmt.Fprintf(c.ErrOrStderr(), "warning: could not record which resources this sync covered (%v); scoped walks it attempted stay pending and are not used until re-run\n", recErr)
	}
	localClearScopedCursors(ctx, c, dbPath, prepared)
	return err
}

// localClearScopedCursors drops the resume cursor a scoped walk left behind
// (successful or failed), so a later plain or unfiltered sync of the same
// resource never resumes from a filtered listing's cursor. It runs after
// recording (the cursor write is not walk evidence) and changes only
// last_cursor: a failed walk stays pending and incomplete as before.
func localClearScopedCursors(ctx context.Context, c *cobra.Command, dbPath string, prepared []localPreparedWalk) {
	if len(prepared) == 0 {
		return
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(c.ErrOrStderr(), "warning: could not clear the scoped walk's resume cursor: %v\n", err)
		return
	}
	defer db.Close()
	for _, pw := range prepared {
		if _, err := db.DB().Exec(`UPDATE sync_state SET last_cursor = '' WHERE resource_type = ? AND COALESCE(last_cursor, '') <> ''`, pw.resource); err != nil {
			fmt.Fprintf(c.ErrOrStderr(), "warning: could not clear the %s resume cursor: %v\n", pw.resource, err)
		}
	}
}

// localPreparedWalk is a scoped walk this run is about to attempt, with the
// pending state its record had before (to restore when the run never
// started fetching it).
type localPreparedWalk struct {
	resource, scope string
	existed         bool
	prevPending     bool
}

// localPrepareScopedWalks readies every scoped walk (numbers type=byoc,
// locations site_id=X) this run selected, before anything is fetched:
//   - it clears that resource's resume cursor, so the walk starts from page
//     one and a completed run's row set is the whole walk (a resumed walk
//     would only see the rows after the interruption);
//   - it marks the walk record pending, so a walk that fails or is killed
//     part-way never leaves the previous record usable over rows it partly
//     rewrote. Only a successful completion clears the mark.
func localPrepareScopedWalks(ctx context.Context, c *cobra.Command, dbPath string) ([]localPreparedWalk, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := localEnsureSyncScopeTable(db); err != nil {
		return nil, err
	}
	out := make([]localPreparedWalk, 0)
	for rt := range localSyncSelection(c) {
		params, err := localSyncFilterParams(c, rt)
		if err != nil {
			return out, nil // sync rejects the same params before fetching
		}
		scope, isWalk := localSyncWalkScope(rt, params)
		if !isWalk {
			continue
		}
		if _, err := db.DB().Exec(`UPDATE sync_state SET last_cursor = '' WHERE resource_type = ? AND COALESCE(last_cursor, '') <> ''`, rt); err != nil {
			return out, err
		}
		pw := localPreparedWalk{resource: rt, scope: scope}
		var pending sql.NullInt64
		if db.DB().QueryRow(`SELECT pending FROM zoom_sync_walks WHERE resource = ? AND scope = ?`, rt, scope).Scan(&pending) == nil {
			pw.existed = true
			pw.prevPending = pending.Int64 == 1
		}
		if _, err := db.DB().Exec(`INSERT INTO zoom_sync_walks (resource, scope, walk_complete, full, synced_at, recorded_at, superseded, pending)
			VALUES (?, ?, 0, 0, '', ?, 0, 1)
			ON CONFLICT(resource, scope) DO UPDATE SET pending = 1`, rt, scope, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return out, err
		}
		out = append(out, pw)
	}
	return out, nil
}

// localSyncMark is where this run's evidence starts: the checkpoint and
// row-event high-water marks, the numbers checkpoint before the run, and
// the scoped walks prepared for it.
type localSyncMark struct {
	stateSeq    int64
	rowSeq      int64
	prevNumbers string
	prepared    []localPreparedWalk
}

// localBeginSyncEventLog opens (creating if needed) the store, installs the
// event-log triggers and returns the current high-water marks.
func localBeginSyncEventLog(ctx context.Context, dbPath string) (localSyncMark, error) {
	var m localSyncMark
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return m, err
	}
	defer db.Close()
	for _, ddl := range localSyncEventDDL {
		if _, err := db.DB().Exec(ddl); err != nil {
			return m, err
		}
	}
	if err := localAbsorbUnattributedEvents(db); err != nil {
		return m, err
	}
	var seq sql.NullInt64
	if err := db.DB().QueryRow(`SELECT MAX(seq) FROM zoom_sync_events`).Scan(&seq); err != nil {
		return m, err
	}
	m.stateSeq = seq.Int64
	seq = sql.NullInt64{}
	if err := db.DB().QueryRow(`SELECT MAX(seq) FROM zoom_row_events`).Scan(&seq); err != nil {
		return m, err
	}
	m.rowSeq = seq.Int64
	var ts sql.NullString
	if db.DB().QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = 'numbers'`).Scan(&ts) == nil {
		m.prevNumbers = ts.String
	}
	return m, nil
}

// localSyncFilterParams returns the user-supplied query params a sync of
// resource sent, normalized as sorted key=value pairs joined by "&". It
// reuses the generated scoping rules: --param reaches flat resources only,
// --global-param and --resource-param reach their targets everywhere.
func localSyncFilterParams(c *cobra.Command, resource string) (string, error) {
	flat, _ := c.Flags().GetStringArray("param")
	perResource, _ := c.Flags().GetStringArray("resource-param")
	global, _ := c.Flags().GetStringArray("global-param")
	p, err := parseSyncUserParams(flat, perResource, global)
	if err != nil {
		return "", err
	}
	params := map[string]string{}
	p.applyTo(resource, params, dependentSyncResourceNames()[resource])
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&"), nil
}

// localSyncSelection returns the resources this sync invocation runs,
// resolved the way the generated sync resolves them: --resources (or the
// default set when empty) narrowed to flat resources, plus every dependent
// whose parent table or own name was requested (all dependents when
// --resources is empty).
func localSyncSelection(c *cobra.Command) map[string]bool {
	requested, _ := c.Flags().GetStringSlice("resources")
	resources := append([]string(nil), requested...)
	if len(resources) == 0 {
		resources = defaultSyncResources()
	}
	sel := map[string]bool{}
	for _, r := range flatSyncResources(resources) {
		sel[r] = true
	}
	allow := map[string]bool{}
	for _, r := range requested {
		allow[r] = true
	}
	for _, dep := range dependentResourceDefs() {
		if len(allow) > 0 && !allow[dep.ParentTable] && !allow[dep.Name] {
			continue
		}
		sel[dep.Name] = true
	}
	return sel
}

// localSyncWalkComplete reports a walk not narrowed by --latest-only,
// --max-pages, --since or the dogfood one-page cap.
func localSyncWalkComplete(c *cobra.Command) bool {
	latestOnly, _ := c.Flags().GetBool("latest-only")
	maxPages, _ := c.Flags().GetInt("max-pages")
	since, _ := c.Flags().GetString("since")
	if cliutil.IsDogfoodEnv() && !c.Flags().Changed("max-pages") {
		maxPages = 1
	}
	return !latestOnly && maxPages == 0 && strings.TrimSpace(since) == ""
}

// localSyncWasFullWalk reports whether this invocation was a full
// reconciled walk: --full (which prunes rows the API no longer returns)
// without --no-prune, on a complete walk.
func localSyncWasFullWalk(c *cobra.Command) bool {
	full, _ := c.Flags().GetBool("full")
	noPrune, _ := c.Flags().GetBool("no-prune")
	return full && !noPrune && localSyncWalkComplete(c)
}

// localSyncWalkScope returns the walk scope a filtered sync of resource
// records separately: "site_id=X" for locations, "type=byoc" for numbers.
func localSyncWalkScope(resource, params string) (string, bool) {
	switch resource {
	case "locations":
		if strings.HasPrefix(params, "site_id=") && !strings.Contains(params, "&") && len(params) > len("site_id=") {
			return params, true
		}
	case "numbers":
		if localByocOnly(params) {
			return params, true
		}
	}
	return "", false
}

// localRecordSyncScope records, for every resource this invocation selected
// AND completed, whether that sync was filtered and whether it was a full
// reconciled walk. Completion is read from the zoom_sync_events rows this
// run wrote (seq > mark.stateSeq): the resource's last checkpoint write in
// this run must be completed with a non-zero watermark. Nothing else counts,
// so a preflight failure, a skipped resource or another resource's
// checkpoint can never be re-attributed to this run's flags.
//
// Filtered walks with a scope of their own (locations site_id=X, numbers
// type=byoc) are also recorded in zoom_sync_walks with the row ids this run
// wrote. A pruning (--full) walk deletes every row it did not return, so it
// drops every other walk of that resource. An additive (non --full) byoc
// walk deletes nothing, so it leaves the unfiltered numbers record in force:
// that record is re-pointed at the new checkpoint, but only if it was valid
// for the checkpoint this run started from.
func localRecordSyncScope(c *cobra.Command, dbPath string, mark localSyncMark) error {
	ctx := localCmdContext(c)
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT seq, resource, complete, synced_at FROM zoom_sync_events WHERE seq > ? ORDER BY seq`, mark.stateSeq)
	if err != nil {
		return err
	}
	type last struct {
		complete bool
		syncedAt string
	}
	lastWrite := map[string]last{}
	maxSeq := mark.stateSeq
	for rows.Next() {
		var seq int64
		var rt, ts sql.NullString
		var complete sql.NullInt64
		if err := rows.Scan(&seq, &rt, &complete, &ts); err != nil {
			_ = rows.Close()
			return err
		}
		maxSeq = seq
		lastWrite[rt.String] = last{complete: complete.Valid && complete.Int64 == 1, syncedAt: ts.String}
	}
	// An iteration error leaves lastWrite short: a resource this run did
	// write would look untouched and get its prepared walk restored. Fail
	// instead, so every prepared walk keeps its pending mark.
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	written, maxRowSeq, err := localRowsWrittenSince(db, mark.rowSeq)
	if err != nil {
		return err
	}
	if err := localEnsureSyncScopeTable(db); err != nil {
		return err
	}
	// A numbers run other than the BYOC walk that rewrote any row of the
	// BYOC walk replaced those rows without BYOC fields (completed or not):
	// the walk no longer describes them.
	if ids := written["numbers"]; len(ids) > 0 {
		params, err := localSyncFilterParams(c, "numbers")
		if err != nil {
			return err
		}
		if !localByocOnly(params) {
			if err := localSupersedeWalk(db, "numbers", "type=byoc", ids); err != nil {
				return err
			}
		}
	}
	// A prepared walk whose resource this run never touched (e.g. a usage
	// error before fetching) did not rewrite anything: restore its mark. A
	// walk that started and did not complete stays pending.
	for _, pw := range mark.prepared {
		if _, attempted := lastWrite[pw.resource]; attempted {
			continue
		}
		var err error
		if !pw.existed {
			_, err = db.DB().Exec(`DELETE FROM zoom_sync_walks WHERE resource = ? AND scope = ?`, pw.resource, pw.scope)
		} else if !pw.prevPending {
			_, err = db.DB().Exec(`UPDATE zoom_sync_walks SET pending = 0 WHERE resource = ? AND scope = ?`, pw.resource, pw.scope)
		}
		if err != nil {
			return err
		}
	}
	selected := localSyncSelection(c)
	fullWalk := localSyncWasFullWalk(c)
	walkComplete := localSyncWalkComplete(c)
	now := time.Now().UTC().Format(time.RFC3339)
	for rt, lw := range lastWrite {
		if !selected[rt] || !lw.complete || !localNonZeroTimestamp(sql.NullString{String: lw.syncedAt, Valid: true}) {
			continue
		}
		params, err := localSyncFilterParams(c, rt)
		if err != nil {
			return err
		}
		scope, isWalk := localSyncWalkScope(rt, params)
		// Press 4.34.2 never prunes a filtered walk (syncPruneSkipReason:
		// resource, flat or global params) nor a resource with a required
		// date window, so such a run deleted nothing and is additive.
		fullWalk := fullWalk && params == "" && !syncResourceHasDateWindow(rt)
		if fullWalk {
			// A prune deleted every row of rt this walk did not return, so
			// every other walk of rt lost its rows.
			if _, err := db.DB().Exec(`DELETE FROM zoom_sync_walks WHERE resource = ? AND scope <> ?`, rt, scope); err != nil {
				return err
			}
			if _, err := db.DB().Exec(`DELETE FROM zoom_sync_walk_rows WHERE resource = ? AND scope <> ?`, rt, scope); err != nil {
				return err
			}
		}
		if isWalk {
			if err := localWriteWalk(db, rt, scope, walkComplete, fullWalk, lw.syncedAt, now, written[rt]); err != nil {
				return err
			}
		}
		if rt == "numbers" && isWalk && !fullWalk {
			if _, err := db.DB().Exec(`UPDATE zoom_sync_scope SET synced_at = ?, recorded_at = ?
				WHERE resource = 'numbers' AND synced_at = ? AND filtered = 0 AND full = 1`, lw.syncedAt, now, mark.prevNumbers); err != nil {
				return err
			}
			continue
		}
		filtered := 0
		if params != "" {
			filtered = 1
		}
		if err := localWriteSyncScope(db, rt, filtered, params, lw.syncedAt, now, fullWalk); err != nil {
			return err
		}
	}
	// Processed events are no longer needed; keep the logs bounded.
	if _, err := db.DB().Exec(`DELETE FROM zoom_sync_events WHERE seq <= ?`, maxSeq); err != nil {
		return err
	}
	if _, err := db.DB().Exec(`DELETE FROM zoom_row_events WHERE seq <= ?`, maxRowSeq); err != nil {
		return err
	}
	return nil
}

// localAbsorbUnattributedEvents handles the logged writes made since the
// last recorded run by something other than a recorded sync (the pre-read
// auto-refresh, workflow archive, write-through caching). They cannot be
// attributed to any walk, so they are never adopted: every walk whose rows
// they rewrote is marked superseded, then the events are discarded.
func localAbsorbUnattributedEvents(db *store.Store) error {
	written, maxRow, err := localRowsWrittenSince(db, 0)
	if err != nil {
		return err
	}
	if err := localEnsureSyncScopeTable(db); err != nil {
		return err
	}
	for rt, ids := range written {
		// Strict read: the events are deleted below, so a walk missed here
		// would never be marked superseded.
		walks, err := localReadWalksErr(db.DB(), rt)
		if err != nil {
			return err
		}
		for scope := range walks {
			if err := localSupersedeWalk(db, rt, scope, ids); err != nil {
				return err
			}
		}
	}
	if _, err := db.DB().Exec(`DELETE FROM zoom_row_events WHERE seq <= ?`, maxRow); err != nil {
		return err
	}
	_, err = db.DB().Exec(`DELETE FROM zoom_sync_events`)
	return err
}

// localRowsWrittenSince returns, per resource type, the row ids written
// after seq (from the zoom_row_events trigger log), and the last seq read.
func localRowsWrittenSince(db *store.Store, seq int64) (map[string][]string, int64, error) {
	out := map[string][]string{}
	maxSeq := seq
	rows, err := db.DB().Query(`SELECT seq, resource_type, id FROM zoom_row_events WHERE seq > ? ORDER BY seq`, seq)
	if err != nil {
		return nil, seq, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var s int64
		var rt, id string
		if err := rows.Scan(&s, &rt, &id); err != nil {
			return nil, seq, err
		}
		maxSeq = s
		if key := rt + "\x00" + id; !seen[key] {
			seen[key] = true
			out[rt] = append(out[rt], id)
		}
	}
	return out, maxSeq, rows.Err()
}

// localWalkRowInsertHook lets tests inject a failure while a walk's row set
// is written; nil in production.
var localWalkRowInsertHook func(id string) error

// localWriteWalk publishes one filtered walk: the new row set and the
// completion metadata (walk_complete, pending = 0) commit in ONE transaction.
// On any error nothing is published, so the walk keeps its pending mark and
// its previous row set.
func localWriteWalk(db *store.Store, resource, scope string, walkComplete, fullWalk bool, syncedAt, now string, ids []string) error {
	if err := localEnsureSyncScopeTable(db); err != nil {
		return err
	}
	wc, full := 0, 0
	if walkComplete {
		wc = 1
	}
	if fullWalk {
		full = 1
	}
	tx, err := db.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM zoom_sync_walk_rows WHERE resource = ? AND scope = ?`, resource, scope); err != nil {
		return err
	}
	for _, id := range ids {
		if localWalkRowInsertHook != nil {
			if err := localWalkRowInsertHook(id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO zoom_sync_walk_rows (resource, scope, id) VALUES (?, ?, ?)`, resource, scope, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO zoom_sync_walks (resource, scope, walk_complete, full, synced_at, recorded_at, superseded, pending)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0)
		ON CONFLICT(resource, scope) DO UPDATE SET walk_complete = excluded.walk_complete, full = excluded.full,
		synced_at = excluded.synced_at, recorded_at = excluded.recorded_at, superseded = 0, pending = 0`,
		resource, scope, wc, full, syncedAt, now); err != nil {
		return err
	}
	return tx.Commit()
}

func localWriteSyncScope(db *store.Store, resource string, filtered int, params, syncedAt, recordedAt string, fullWalk bool) error {
	if err := localEnsureSyncScopeTable(db); err != nil {
		return err
	}
	full := 0
	if fullWalk {
		full = 1
	}
	_, err := db.DB().Exec(`INSERT INTO zoom_sync_scope (resource, filtered, params, synced_at, recorded_at, full)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource) DO UPDATE SET filtered = excluded.filtered, params = excluded.params,
		synced_at = excluded.synced_at, recorded_at = excluded.recorded_at, full = excluded.full`,
		resource, filtered, params, syncedAt, recordedAt, full)
	return err
}

// localSyncScopeRow is the recorded filter of a resource's latest
// completed sync.
type localSyncScopeRow struct {
	filtered bool
	full     bool
	params   string
	syncedAt string
}

// localReadSyncScope reads the scope rows; a store without the table (no
// sync since this feature, or a read-only legacy store) has none.
func localReadSyncScope(q localQuerier) map[string]localSyncScopeRow {
	out := map[string]localSyncScopeRow{}
	if q == nil {
		return out
	}
	rows, err := q.Query(`SELECT resource, filtered, params, synced_at, full FROM zoom_sync_scope`)
	if err != nil {
		// A table written before the full column existed: no row can be
		// trusted as a full walk.
		rows, err = q.Query(`SELECT resource, filtered, params, synced_at, 0 FROM zoom_sync_scope`)
		if err != nil {
			return out
		}
	}
	defer rows.Close()
	for rows.Next() {
		var r, params, syncedAt sql.NullString
		var filtered, full sql.NullInt64
		if err := rows.Scan(&r, &filtered, &params, &syncedAt, &full); err != nil {
			// A row that cannot be read: trust no row rather than a partial set.
			return map[string]localSyncScopeRow{}
		}
		if r.Valid {
			out[r.String] = localSyncScopeRow{filtered: filtered.Int64 == 1, full: full.Int64 == 1, params: params.String, syncedAt: syncedAt.String}
		}
	}
	if rows.Err() != nil {
		return map[string]localSyncScopeRow{}
	}
	return out
}

// localSupersedeWalk marks a walk superseded when any of ids is one of its
// rows (another sync rewrote them).
func localSupersedeWalk(db *store.Store, resource, scope string, ids []string) error {
	walks, err := localReadWalksErr(db.DB(), resource)
	if err != nil {
		return err
	}
	walk, ok := walks[scope]
	if !ok {
		return nil
	}
	for _, id := range ids {
		if walk.rows[id] {
			_, err := db.DB().Exec(`UPDATE zoom_sync_walks SET superseded = 1 WHERE resource = ? AND scope = ?`, resource, scope)
			return err
		}
	}
	return nil
}

// localWalk is one recorded filtered walk and the row ids it wrote.
type localWalk struct {
	walkComplete bool
	superseded   bool
	// pending: a newer attempt of this walk started and has not completed
	// (failed, interrupted or still running); the record must not be used.
	pending  bool
	syncedAt string
	rows     map[string]bool
}

// usable is a completed walk with no newer attempt pending and no later
// sync having rewritten its rows.
func (w localWalk) usable() bool { return w.walkComplete && !w.pending && !w.superseded }

// localReadWalks returns resource's recorded walks keyed by scope. Any read
// error yields no walks at all, never a partial set: a walk whose row set or
// supersession events were only half read could otherwise look usable, so a
// reader treats the resource as not covered instead.
func localReadWalks(q localQuerier, resource string) map[string]localWalk {
	out, err := localReadWalksErr(q, resource)
	if err != nil {
		return map[string]localWalk{}
	}
	return out
}

// localReadWalksErr is localReadWalks that reports read errors. Writers that
// act on the walks (marking them superseded, then discarding the events that
// justified it) use it so a failed read aborts instead of skipping a walk.
// A store without the walk tables (no recorded walk yet) has no walks, and
// one without the row-event log has no supersession events.
func localReadWalksErr(q localQuerier, resource string) (map[string]localWalk, error) {
	out := map[string]localWalk{}
	if q == nil {
		return out, nil
	}
	rows, err := q.Query(`SELECT scope, walk_complete, synced_at, superseded, pending FROM zoom_sync_walks WHERE resource = ?`, resource)
	if err != nil {
		if localMissingTable(err) {
			return out, nil
		}
		return nil, fmt.Errorf("reading %s walks: %w", resource, err)
	}
	for rows.Next() {
		var scope, syncedAt sql.NullString
		var wc, sup, pend sql.NullInt64
		if err := rows.Scan(&scope, &wc, &syncedAt, &sup, &pend); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning %s walk: %w", resource, err)
		}
		if scope.Valid {
			out[scope.String] = localWalk{walkComplete: wc.Int64 == 1, superseded: sup.Int64 == 1, pending: pend.Int64 == 1, syncedAt: syncedAt.String, rows: map[string]bool{}}
		}
	}
	if err := localCloseRows(rows); err != nil {
		return nil, fmt.Errorf("reading %s walks: %w", resource, err)
	}
	ids, err := q.Query(`SELECT scope, id FROM zoom_sync_walk_rows WHERE resource = ?`, resource)
	if err != nil {
		return nil, fmt.Errorf("reading %s walk rows: %w", resource, err)
	}
	for ids.Next() {
		var scope, id string
		if err := ids.Scan(&scope, &id); err != nil {
			_ = ids.Close()
			return nil, fmt.Errorf("scanning %s walk row: %w", resource, err)
		}
		if w, ok := out[scope]; ok {
			w.rows[id] = true
		}
	}
	if err := localCloseRows(ids); err != nil {
		return nil, fmt.Errorf("reading %s walk rows: %w", resource, err)
	}
	// Logged row writes not yet absorbed by a recorded sync were made by
	// something the recorder cannot attribute (or by a sync still running):
	// a walk whose rows they rewrote is treated as superseded, never as
	// still describing them.
	ev, err := q.Query(`SELECT DISTINCT id FROM zoom_row_events WHERE resource_type = ?`, resource)
	if err != nil {
		if localMissingTable(err) {
			return out, nil
		}
		return nil, fmt.Errorf("reading %s row events: %w", resource, err)
	}
	for ev.Next() {
		var id string
		if err := ev.Scan(&id); err != nil {
			_ = ev.Close()
			return nil, fmt.Errorf("scanning %s row event: %w", resource, err)
		}
		for scope, w := range out {
			if w.rows[id] {
				w.superseded = true
				out[scope] = w
			}
		}
	}
	if err := localCloseRows(ev); err != nil {
		return nil, fmt.Errorf("reading %s row events: %w", resource, err)
	}
	return out, nil
}

// localLegacyWalkSchema reports a walk read that failed only because the store
// predates part of the walk schema (a missing table or column). A read-only
// reader cannot migrate it, so it treats the store as having no usable walks;
// writers migrate first (localEnsureSyncScopeTable) and keep the error.
func localLegacyWalkSchema(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		msg := e.Error()
		if strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column") {
			return true
		}
	}
	return false
}

// localCloseRows closes a drained result set and reports the iteration error
// first (rows.Close does not return it).
func localCloseRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

// localByocOnly is a numbers sync narrowed only by type=byoc: the BYOC walk.
// It never covers the account for numbers inventory or orphan checks; e911
// reads the rows it wrote for the BYOC carrier-update check.
func localByocOnly(params string) bool { return params == "type=byoc" }

// localByocSyncHint is the additive BYOC walk (a filtered sync never prunes).
func localByocSyncHint() string {
	return localCLIName + " sync --resources numbers --resource-param numbers:type=byoc (additive: a filtered sync never prunes)"
}
