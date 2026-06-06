// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"time"
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
	stamp := capturedAt.UTC().Format(snapshotTimeFormat)
	counts := map[string]int{}

	tx, err := s.db.Begin()
	if err != nil {
		return counts, err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, entity := range SnapshotEntities {
		// Two syncs starting within the same wall-clock second share a stamp
		// (RFC3339 is second-precision); replace rather than double-insert so
		// snapshot-row consumers (versions rollout) never double-count.
		if _, err := tx.Exec(
			`DELETE FROM fleet_snapshots WHERE entity = ? AND captured_at = ?`,
			entity, stamp,
		); err != nil {
			return counts, err
		}
		rows, err := tx.Query(`SELECT id, data FROM resources WHERE resource_type = ?`, entity)
		if err != nil {
			return counts, err
		}
		type rec struct{ id, data string }
		var recs []rec
		for rows.Next() {
			var id, data string
			if err := rows.Scan(&id, &data); err != nil {
				_ = rows.Close() // returning the scan error; close error is not actionable on this path
				return counts, err
			}
			recs = append(recs, rec{id, data})
		}
		_ = rows.Close() // rows.Err() below is the authoritative iteration-error check
		if err := rows.Err(); err != nil {
			return counts, err
		}
		for _, r := range recs {
			if _, err := tx.Exec(
				`INSERT INTO fleet_snapshots (captured_at, entity, entity_id, data) VALUES (?, ?, ?, ?)`,
				stamp, entity, r.id, r.data,
			); err != nil {
				return counts, err
			}
		}
		counts[entity] = len(recs)

		// Prune to the most recent snapshotKeepCaptures distinct capture times.
		if _, err := tx.Exec(
			`DELETE FROM fleet_snapshots
			 WHERE entity = ?
			   AND captured_at NOT IN (
			     SELECT captured_at FROM (
			       SELECT DISTINCT captured_at FROM fleet_snapshots
			       WHERE entity = ?
			       ORDER BY captured_at DESC
			       LIMIT ?
			     )
			   )`,
			entity, entity, snapshotKeepCaptures,
		); err != nil {
			return counts, err
		}
	}

	if err := tx.Commit(); err != nil {
		return counts, err
	}
	return counts, nil
}

// FleetSnapshotTimes returns every distinct capture time for an entity, oldest
// first.
func (s *Store) FleetSnapshotTimes(entity string) ([]time.Time, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT captured_at FROM fleet_snapshots WHERE entity = ? ORDER BY captured_at ASC`,
		entity,
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
	q := `SELECT captured_at FROM fleet_snapshots WHERE entity = ?`
	args := []any{entity}
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
