// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlite "modernc.org/sqlite"
)

// snapshotTimeFormat is the canonical on-disk stamp for fleet_snapshots.
// RFC3339 sorts lexicographically in chronological order, so string
// comparisons in SQL are valid chronological comparisons.
const snapshotTimeFormat = time.RFC3339

// snapshotKeepCaptures bounds history growth: only the most recent N distinct
// capture timestamps per entity are retained.
const snapshotKeepCaptures = 60

// SnapshotEntities are the high-gravity entities captured into history on each
// sync so the time-travel novel features have something to diff against.
var SnapshotEntities = []string{"agents", "threats"}

// CaptureFleetSnapshot appends the current agents/threats state from the live
// resources table into fleet_snapshots, stamped with capturedAt (UTC). It is a
// best-effort history capture invoked at the end of `sync`; callers should log
// but not fail the operation on error. Returns per-entity row counts.
func (s *Store) CaptureFleetSnapshot(capturedAt time.Time) (map[string]int, error) {
	return s.CaptureFleetSnapshotFor(capturedAt, SnapshotEntities)
}

// CaptureFleetSnapshotFor is CaptureFleetSnapshot limited to the named
// entities: a sync that refreshed only agents must not stamp a "new" threats
// capture that is really the previous sync's data under a later time, or a
// history diff would read stale rows as the current state.
func (s *Store) CaptureFleetSnapshotFor(capturedAt time.Time, entities []string) (map[string]int, error) {
	return s.captureFleetSnapshot(capturedAt, entities)
}

func (s *Store) captureFleetSnapshot(capturedAt time.Time, entities []string) (map[string]int, error) {
	stamp := capturedAt.UTC().Format(snapshotTimeFormat)
	counts := map[string]int{}

	tx, err := s.db.Begin()
	if err != nil {
		return counts, err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, entity := range entities {
		recs, err := querySnapshotRecs(tx, `SELECT id, data FROM resources WHERE resource_type = ?`, entity)
		if err != nil {
			return counts, err
		}
		if err := writeCaptureTx(tx, entity, stamp, recs, sql.NullInt64{}); err != nil {
			return counts, err
		}
		counts[entity] = len(recs)
	}

	if err := tx.Commit(); err != nil {
		return counts, err
	}
	return counts, nil
}

type snapshotRec struct{ id, data string }

func querySnapshotRecs(tx *sql.Tx, q string, args ...any) ([]snapshotRec, error) {
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	var recs []snapshotRec
	for rows.Next() {
		var r snapshotRec
		if err := rows.Scan(&r.id, &r.data); err != nil {
			_ = rows.Close() // returning the scan error; close error is not actionable on this path
			return nil, err
		}
		recs = append(recs, r)
	}
	_ = rows.Close() // rows.Err() below is the authoritative iteration-error check
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// writeCaptureTx stores one entity's capture at stamp, logs it (walkID is
// NULL for a capture not tied to a tracked walk), and prunes old captures.
func writeCaptureTx(tx *sql.Tx, entity, stamp string, recs []snapshotRec, walkID sql.NullInt64) error {
	// A capture at an existing stamp replaces it rather than double-inserting,
	// so snapshot-row consumers (versions rollout) never double-count. Walk
	// captures never collide (nextCaptureStampTx); an explicit-time caller can.
	if _, err := tx.Exec(
		`DELETE FROM fleet_snapshots WHERE entity = ? AND captured_at = ?`,
		entity, stamp,
	); err != nil {
		return err
	}
	for _, r := range recs {
		if _, err := tx.Exec(
			`INSERT INTO fleet_snapshots (captured_at, entity, entity_id, data) VALUES (?, ?, ?, ?)`,
			stamp, entity, r.id, r.data,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO fleet_capture_log (entity, captured_at, row_count, walk_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity, captured_at) DO UPDATE SET row_count = excluded.row_count, walk_id = excluded.walk_id`,
		entity, stamp, len(recs), walkID,
	); err != nil {
		return err
	}

	// Prune to the most recent snapshotKeepCaptures distinct capture times.
	keep := `SELECT captured_at FROM (
		SELECT captured_at FROM (` + snapshotCaptureTimesSQL + `)
		ORDER BY captured_at DESC
		LIMIT ?
	)`
	for _, table := range []string{"fleet_snapshots", "fleet_capture_log"} {
		if _, err := tx.Exec(
			`DELETE FROM `+table+` WHERE entity = ? AND captured_at NOT IN (`+keep+`)`, // #nosec G202 -- table is one of two internal literals; values go through ? placeholders
			entity, entity, entity, snapshotKeepCaptures,
		); err != nil {
			return err
		}
	}
	return nil
}

// ---- per-walk capture membership ----
//
// A walk capture claims "these rows are what this sync walk returned". The
// generated upsert stamps synced_at with the wall clock at second resolution,
// so no timestamp test can tell a row this walk wrote from one an earlier walk
// wrote in the same second, or before the clock stepped back; two walks can
// even share a clock tick. Membership is therefore recorded, never inferred:
//
//   - BeginFleetWalk registers the walk (fleet_walks, a never-reused id) and
//     arms a recorder for this store's file in this process.
//   - While armed, every read-write SQLite connection this process opens to
//     that file gets TEMP triggers (installFleetWalkRecorder, a driver
//     connection hook) that copy each agents/threats row it writes, with the
//     data written, into fleet_walk_rows, and each sync_state write for those
//     entities into fleet_walk_state, under the walk id. TEMP triggers belong
//     to the connection that created them: a write by any other process (a
//     concurrent sync, an MCP tool call caching a read) is never attributed
//     to this walk. No generated file is edited.
//   - CaptureFleetWalk records a capture for an entity only when this walk's
//     own sync_state writes show it started and finished the read, and only
//     from fleet_walk_rows. It fails closed (records nothing, returns
//     ErrFleetWalkUnproven) when the record cannot be proven complete: no
//     connection carried the recorder, one failed to install it, the walk is
//     no longer registered, or the store schema changed mid-walk.

// ErrFleetWalkUnproven is wrapped by CaptureFleetWalk when a walk cannot prove
// which rows it wrote. Nothing is recorded: no capture beats a wrong capture.
var ErrFleetWalkUnproven = errors.New("cannot prove which rows this sync walk wrote")

// fleetWalkReapAfter is how long a walk a crashed process left registered keeps
// its rows. A capture re-checks its own walk row, so reaping a walk that was
// somehow still running costs it its capture, never a wrong one.
const fleetWalkReapAfter = 24 * time.Hour

// fleetWalkTimeFormat is fixed-width (lexicographic = chronological). It is
// used only to reap abandoned walks, never to decide membership.
const fleetWalkTimeFormat = "2006-01-02T15:04:05.000000000Z"

type fleetWalkRecorder struct {
	walkID    int64
	installed int
	failed    int
	firstErr  error
}

var fleetWalkRecorders = struct {
	mu     sync.Mutex
	byPath map[string]*fleetWalkRecorder
}{byPath: map[string]*fleetWalkRecorder{}}

func init() {
	sqlite.RegisterConnectionHook(installFleetWalkRecorder)
}

// FleetWalk is one registered sync walk. See BeginFleetWalk.
type FleetWalk struct {
	id            int64
	path          string
	schemaVersion int64

	mu       sync.Mutex
	stopped  bool
	proofErr error
}

// ID is the walk's membership key (fleet_walks.walk_id).
func (w *FleetWalk) ID() int64 { return w.id }

// BeginFleetWalk registers a walk and arms its recorder: every read-write
// connection this process opens to this store's file from now until Stop
// records the agents/threats rows and sync_state it writes under the walk's
// id. Open the store that performs the walk after this call; connections that
// already exist (including this Store's) record nothing.
func (s *Store) BeginFleetWalk() (*FleetWalk, error) {
	key, ok := fleetWalkPathKey(s.path)
	if !ok {
		return nil, fmt.Errorf("resolving store path %q", s.path)
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	now := time.Now().UTC()
	if _, err := tx.Exec(`DELETE FROM fleet_walks WHERE opened_at < ?`, now.Add(-fleetWalkReapAfter).Format(fleetWalkTimeFormat)); err != nil {
		return nil, err
	}
	for _, table := range []string{"fleet_walk_rows", "fleet_walk_state"} {
		if _, err := tx.Exec(`DELETE FROM ` + table + ` WHERE walk_id NOT IN (SELECT walk_id FROM fleet_walks)`); err != nil { // #nosec G202 -- table is one of two internal literals
			return nil, err
		}
	}
	var schemaVersion int64
	if err := tx.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion); err != nil {
		return nil, err
	}
	res, err := tx.Exec(`INSERT INTO fleet_walks (opened_at, schema_version) VALUES (?, ?)`, now.Format(fleetWalkTimeFormat), schemaVersion)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	fleetWalkRecorders.mu.Lock()
	if cur := fleetWalkRecorders.byPath[key]; cur != nil {
		fleetWalkRecorders.mu.Unlock()
		// Rolled back with the transaction: the walk was never registered.
		return nil, fmt.Errorf("walk %d of this store is already in progress in this process", cur.walkID)
	}
	if err := tx.Commit(); err != nil {
		fleetWalkRecorders.mu.Unlock()
		return nil, err
	}
	fleetWalkRecorders.byPath[key] = &fleetWalkRecorder{walkID: id}
	fleetWalkRecorders.mu.Unlock()
	return &FleetWalk{id: id, path: key, schemaVersion: schemaVersion}, nil
}

// Stop disarms the walk's recorder (connections opened afterwards record
// nothing) and settles whether the recorder covered every connection that
// could have carried the walk's writes. Idempotent; CaptureFleetWalk and
// AbandonFleetWalk call it.
func (w *FleetWalk) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	fleetWalkRecorders.mu.Lock()
	rec := fleetWalkRecorders.byPath[w.path]
	if rec != nil && rec.walkID == w.id {
		delete(fleetWalkRecorders.byPath, w.path)
	} else {
		rec = nil
	}
	fleetWalkRecorders.mu.Unlock()
	switch {
	case rec == nil:
		w.proofErr = fmt.Errorf("%w: walk %d had no armed recorder", ErrFleetWalkUnproven, w.id)
	case rec.failed > 0:
		w.proofErr = fmt.Errorf("%w: %d connection(s) could not record walk %d: %v", ErrFleetWalkUnproven, rec.failed, w.id, rec.firstErr)
	case rec.installed == 0:
		w.proofErr = fmt.Errorf("%w: no connection opened during walk %d recorded its writes", ErrFleetWalkUnproven, w.id)
	}
}

func (w *FleetWalk) stop() error {
	w.Stop()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.proofErr
}

// CaptureFleetWalk records, for each named entity this walk completed, a
// capture holding exactly the rows the walk wrote, with the data it wrote.
// An entity the walk did not start and finish is skipped silently (a partial,
// skipped or failed walk is not fleet state). When the walk cannot prove its
// record is complete, nothing is recorded and the returned error wraps
// ErrFleetWalkUnproven. The walk's working rows are deleted either way.
//
// The stamp is the capture's identity and its order: it is the wall-clock
// second, moved past the latest existing capture when the clock has not, so
// two walks in one second, or after the clock stepped back, still get two
// distinct, correctly ordered captures.
func (s *Store) CaptureFleetWalk(w *FleetWalk, capturedAt time.Time, entities []string) (map[string]int, error) {
	counts := map[string]int{}
	proofErr := w.stop()
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return counts, err
	}
	defer tx.Rollback() //nolint:errcheck
	if proofErr == nil {
		proofErr = s.verifyFleetWalkTx(tx, w)
	}
	if proofErr != nil {
		if err := deleteFleetWalkTx(tx, w.id); err != nil {
			return counts, err
		}
		if err := tx.Commit(); err != nil {
			return counts, err
		}
		return counts, proofErr
	}

	completed := map[string]bool{}
	recs, err := querySnapshotRecs(tx,
		`SELECT entity, '' FROM fleet_walk_state WHERE walk_id = ? AND started = 1 AND complete = 1`, w.id)
	if err != nil {
		return counts, err
	}
	for _, r := range recs {
		completed[r.id] = true
	}
	var stamp string
	for _, entity := range entities {
		if !completed[entity] {
			continue
		}
		if stamp == "" {
			if stamp, err = nextCaptureStampTx(tx, capturedAt); err != nil {
				return counts, err
			}
		}
		rows, err := querySnapshotRecs(tx,
			`SELECT entity_id, data FROM fleet_walk_rows WHERE walk_id = ? AND entity = ?`, w.id, entity)
		if err != nil {
			return counts, err
		}
		if err := writeCaptureTx(tx, entity, stamp, rows, sql.NullInt64{Int64: w.id, Valid: true}); err != nil {
			return counts, err
		}
		counts[entity] = len(rows)
	}
	if err := deleteFleetWalkTx(tx, w.id); err != nil {
		return counts, err
	}
	if err := tx.Commit(); err != nil {
		return map[string]int{}, err
	}
	return counts, nil
}

// AbandonFleetWalk disarms a walk and deletes its working rows without
// recording anything (the sync failed, or history was not wanted after all).
func (s *Store) AbandonFleetWalk(w *FleetWalk) error {
	w.Stop()
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := deleteFleetWalkTx(tx, w.id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) verifyFleetWalkTx(tx *sql.Tx, w *FleetWalk) error {
	if key, ok := fleetWalkPathKey(s.path); !ok || key != w.path {
		return fmt.Errorf("%w: walk %d belongs to %s, not %s", ErrFleetWalkUnproven, w.id, w.path, s.path)
	}
	var registered int64
	err := tx.QueryRow(`SELECT schema_version FROM fleet_walks WHERE walk_id = ?`, w.id).Scan(&registered)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: walk %d is no longer registered", ErrFleetWalkUnproven, w.id)
	}
	if err != nil {
		return err
	}
	var current int64
	if err := tx.QueryRow(`PRAGMA schema_version`).Scan(&current); err != nil {
		return err
	}
	if current != registered || registered != w.schemaVersion {
		// A table rebuild drops the TEMP triggers that recorded the walk.
		return fmt.Errorf("%w: the store schema changed during walk %d", ErrFleetWalkUnproven, w.id)
	}
	return nil
}

func deleteFleetWalkTx(tx *sql.Tx, walkID int64) error {
	for _, table := range []string{"fleet_walk_rows", "fleet_walk_state", "fleet_walks"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE walk_id = ?`, walkID); err != nil { // #nosec G202 -- table is one of three internal literals; the id goes through the ? placeholder
			return err
		}
	}
	return nil
}

// nextCaptureStampTx is capturedAt's second, or one second past the latest
// existing capture of any entity when capturedAt is not after it.
func nextCaptureStampTx(tx *sql.Tx, capturedAt time.Time) (string, error) {
	at := capturedAt.UTC().Truncate(time.Second)
	rows, err := tx.Query(`SELECT DISTINCT captured_at FROM (
		SELECT captured_at FROM fleet_capture_log UNION SELECT captured_at FROM fleet_snapshots
	) ORDER BY captured_at DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var stamp string
		if err := rows.Scan(&stamp); err != nil {
			return "", err
		}
		// Readers skip a stamp they cannot parse (FleetSnapshotTimes); so does
		// the ordering here. The first parseable one is the latest capture.
		latest, err := time.Parse(snapshotTimeFormat, stamp)
		if err != nil {
			continue
		}
		if !at.After(latest) {
			at = latest.UTC().Add(time.Second)
		}
		break
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return at.Format(snapshotTimeFormat), nil
}

// installFleetWalkRecorder is the driver connection hook. It never fails a
// connection: a connection that cannot carry the recorder only voids the
// walk's capture (counted as a failure, checked by Stop).
func installFleetWalkRecorder(conn sqlite.ExecQuerierContext, dsn string) error {
	path, readOnly, ok := sqliteDSNPath(dsn)
	if !ok || readOnly {
		return nil // a read-only connection cannot write rows
	}
	fleetWalkRecorders.mu.Lock()
	defer fleetWalkRecorders.mu.Unlock()
	rec := fleetWalkRecorders.byPath[path]
	if rec == nil {
		return nil
	}
	if err := armFleetWalkConn(conn, rec.walkID); err != nil {
		rec.failed++
		if rec.firstErr == nil {
			rec.firstErr = err
		}
		return nil
	}
	rec.installed++
	return nil
}

func armFleetWalkConn(conn sqlite.ExecQuerierContext, walkID int64) error {
	ctx := context.Background()
	rows, err := conn.QueryContext(ctx, `SELECT COUNT(*) FROM main.sqlite_master WHERE type = 'table'
		AND name IN ('resources', 'sync_state', 'fleet_walks', 'fleet_walk_rows', 'fleet_walk_state')`, nil)
	if err != nil {
		return err
	}
	dest := make([]driver.Value, 1)
	err = rows.Next(dest)
	_ = rows.Close() // the value is read; a close error is not actionable here
	if err != nil {
		return err
	}
	if n, _ := dest[0].(int64); n != 5 {
		return fmt.Errorf("store is missing walk tables (%v of 5 present)", dest[0])
	}
	for _, stmt := range fleetWalkRecorderSQL(walkID) {
		if _, err := conn.ExecContext(ctx, stmt, nil); err != nil {
			return err
		}
	}
	return nil
}

// fleetWalkRecorderSQL is the TEMP trigger set for one walk. Every value
// spliced in is internal: the integer walk id and the SnapshotEntities
// literals. Each trigger is inert once the walk row is deleted. SQLite
// forbids a qualified INSERT/DELETE target inside a trigger; a TEMP trigger
// resolves the bare name temp-then-main, and the connection is brand new (it
// holds no temp tables), so the targets are the main walk tables.
func fleetWalkRecorderSQL(walkID int64) []string {
	w := strconv.FormatInt(walkID, 10)
	quoted := make([]string, 0, len(SnapshotEntities))
	for _, e := range SnapshotEntities {
		quoted = append(quoted, "'"+strings.ReplaceAll(e, "'", "''")+"'")
	}
	in := "(" + strings.Join(quoted, ", ") + ")"
	armed := `EXISTS (SELECT 1 FROM main.fleet_walks WHERE walk_id = ` + w + `)`
	name := "fleet_walk_" + w
	upsertRow := `INSERT INTO fleet_walk_rows (walk_id, entity, entity_id, data)
			SELECT ` + w + `, NEW.resource_type, NEW.id, NEW.data WHERE NEW.resource_type IN ` + in + `
			ON CONFLICT (walk_id, entity, entity_id) DO UPDATE SET data = excluded.data;`
	upsertState := `INSERT INTO fleet_walk_state (walk_id, entity, started, complete)
			VALUES (` + w + `, NEW.resource_type,
				COALESCE(NEW.last_attempt_complete, 0) = 0,
				COALESCE(NEW.last_attempt_complete, 0) = 1)
			ON CONFLICT (walk_id, entity) DO UPDATE SET
				started = MAX(started, excluded.started),
				complete = excluded.complete;`
	return []string{
		`CREATE TEMP TRIGGER IF NOT EXISTS ` + name + `_rows_ins AFTER INSERT ON main.resources
		WHEN NEW.resource_type IN ` + in + ` AND ` + armed + `
		BEGIN ` + upsertRow + ` END`,
		`CREATE TEMP TRIGGER IF NOT EXISTS ` + name + `_rows_upd AFTER UPDATE ON main.resources
		WHEN (OLD.resource_type IN ` + in + ` OR NEW.resource_type IN ` + in + `) AND ` + armed + `
		BEGIN
			DELETE FROM fleet_walk_rows WHERE walk_id = ` + w + ` AND entity = OLD.resource_type AND entity_id = OLD.id;
			` + upsertRow + `
		END`,
		`CREATE TEMP TRIGGER IF NOT EXISTS ` + name + `_rows_del AFTER DELETE ON main.resources
		WHEN OLD.resource_type IN ` + in + ` AND ` + armed + `
		BEGIN
			DELETE FROM fleet_walk_rows WHERE walk_id = ` + w + ` AND entity = OLD.resource_type AND entity_id = OLD.id;
		END`,
		`CREATE TEMP TRIGGER IF NOT EXISTS ` + name + `_state_ins AFTER INSERT ON main.sync_state
		WHEN NEW.resource_type IN ` + in + ` AND ` + armed + `
		BEGIN ` + upsertState + ` END`,
		`CREATE TEMP TRIGGER IF NOT EXISTS ` + name + `_state_upd AFTER UPDATE ON main.sync_state
		WHEN NEW.resource_type IN ` + in + ` AND ` + armed + `
		BEGIN ` + upsertState + ` END`,
	}
}

// sqliteDSNPath returns the walk key of a DSN's database file and whether the
// DSN opens it read-only.
func sqliteDSNPath(dsn string) (path string, readOnly bool, ok bool) {
	p, query, _ := strings.Cut(dsn, "?")
	p = strings.TrimPrefix(p, "file:")
	if p == "" || strings.HasPrefix(p, ":memory:") {
		return "", false, false
	}
	if vals, err := url.ParseQuery(query); err == nil {
		readOnly = vals.Get("mode") == "ro" || vals.Get("immutable") == "1"
	}
	path, ok = fleetWalkPathKey(p)
	return path, readOnly, ok
}

func fleetWalkPathKey(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	return filepath.Clean(abs), true
}

// snapshotCaptureTimesSQL is every capture time of one entity (bound twice):
// the capture log, which also records zero-row captures, united with the rows'
// own stamps (captures written before the log existed).
const snapshotCaptureTimesSQL = `SELECT captured_at FROM fleet_capture_log WHERE entity = ?
	UNION SELECT captured_at FROM fleet_snapshots WHERE entity = ?`

// FleetSnapshotTimes returns every distinct capture time for an entity, oldest
// first.
func (s *Store) FleetSnapshotTimes(entity string) ([]time.Time, error) {
	rows, err := s.db.Query(
		`SELECT captured_at FROM (`+snapshotCaptureTimesSQL+`) ORDER BY captured_at ASC`,
		entity, entity,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var stamp string
		if err := rows.Scan(&stamp); err != nil {
			return nil, err
		}
		if t, err := time.Parse(snapshotTimeFormat, stamp); err == nil {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// FleetSnapshotRows returns the entity rows captured at exactly `at`.
func (s *Store) FleetSnapshotRows(entity string, at time.Time) ([]json.RawMessage, error) {
	stamp := at.UTC().Format(snapshotTimeFormat)
	rows, err := s.db.Query(
		`SELECT data FROM fleet_snapshots WHERE entity = ? AND captured_at = ?`,
		entity, stamp,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, rows.Err()
}

// FleetSnapshotRowsByID returns the entity rows captured at exactly `at`, keyed
// by entity_id.
func (s *Store) FleetSnapshotRowsByID(entity string, at time.Time) (map[string]json.RawMessage, error) {
	stamp := at.UTC().Format(snapshotTimeFormat)
	rows, err := s.db.Query(
		`SELECT entity_id, data FROM fleet_snapshots WHERE entity = ? AND captured_at = ?`,
		entity, stamp,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		out[id] = json.RawMessage(data)
	}
	return out, rows.Err()
}

// FleetSnapshotLatest returns the most recent capture time for an entity.
func (s *Store) FleetSnapshotLatest(entity string) (time.Time, bool, error) {
	return s.snapshotBound(entity, "DESC", nil)
}

// FleetSnapshotNearestBefore returns the most recent capture time at or before
// cutoff, used to establish a baseline "state as of N ago".
func (s *Store) FleetSnapshotNearestBefore(entity string, cutoff time.Time) (time.Time, bool, error) {
	c := cutoff.UTC().Format(snapshotTimeFormat)
	return s.snapshotBound(entity, "DESC", &boundFilter{op: "<=", val: c})
}

type boundFilter struct {
	op  string
	val string
}

func (s *Store) snapshotBound(entity, dir string, f *boundFilter) (time.Time, bool, error) {
	// dir and f.op are never user-controlled: dir is always the package-internal
	// literal "DESC" and f.op is always "<=" (see the two callers above). Every
	// value that could carry user input (entity, f.val) is bound via a ? placeholder.
	q := `SELECT captured_at FROM (` + snapshotCaptureTimesSQL + `) WHERE 1 = 1`
	args := []any{entity, entity}
	if f != nil {
		q += ` AND captured_at ` + f.op + ` ?` // #nosec G202 -- f.op is an internal literal ("<="); user data goes through the ? placeholder
		args = append(args, f.val)
	}
	q += ` ORDER BY captured_at ` + dir + ` LIMIT 1` // #nosec G202 -- dir is an internal literal ("DESC"), not user input
	row := s.db.QueryRow(q, args...)
	var stamp string
	if err := row.Scan(&stamp); err != nil {
		return time.Time{}, false, nil //nolint:nilerr // no rows => no snapshot
	}
	t, err := time.Parse(snapshotTimeFormat, stamp)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

// ResetSyncWatermark drops an entity's incremental watermark and resume cursor
// and clears its completion flag, so the next walk is unbounded. Whether that
// walk completed is proven by its own recorded sync_state writes (see
// CaptureFleetWalk), not by this flag. The generated incremental sync for
// agents and threats filters on createdAt__gte
// (records created since the last sync), which never refreshes the state of a
// record that already exists; fleet history needs every record re-read.
func (s *Store) ResetSyncWatermark(resourceType string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.Exec(
		`UPDATE sync_state SET last_cursor = '', last_synced_at = NULL, last_attempt_complete = 0 WHERE resource_type = ?`,
		resourceType,
	)
	return err
}
