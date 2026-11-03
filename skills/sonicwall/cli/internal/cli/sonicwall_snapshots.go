// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"sonicwall-pp-cli/internal/store"
)

// Posture snapshots live in their own table beside the generated resources
// table, so `posture changes` can diff runs. One row per firewall per run; each
// run has a unique id and records the scope it covered (fleet, tenant:<id>,
// serial:<sn>) so only like-for-like runs are compared.
const postureRunsDDL = `CREATE TABLE IF NOT EXISTS posture_runs (
	run_id   TEXT PRIMARY KEY,
	taken_at TEXT NOT NULL,
	scope    TEXT NOT NULL,
	complete INTEGER NOT NULL,
	devices  INTEGER NOT NULL
)`

const postureSnapshotDDL = `CREATE TABLE IF NOT EXISTS posture_snapshot_rows (
	run_id   TEXT NOT NULL,
	taken_at TEXT NOT NULL,
	scope    TEXT NOT NULL,
	complete INTEGER NOT NULL,
	serial   TEXT NOT NULL,
	name     TEXT,
	checks   TEXT NOT NULL,
	PRIMARY KEY (run_id, serial)
)`

const snapshotTimeLayout = "2006-01-02T15:04:05.000000000Z"

func newRunID(t time.Time) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return strconv.FormatInt(t.UnixNano(), 10) + "-" + hex.EncodeToString(b)
}

func savePostureSnapshot(ctx context.Context, dbPath string, takenAt time.Time, scope string, complete bool, rows []postureRow) error {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()
	for _, ddl := range []string{postureRunsDDL, postureSnapshotDDL} {
		if _, err := db.DB().ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating posture tables: %w", err)
		}
	}
	if len(rows) == 0 {
		// An empty run is history only when it empties a scope whose last run
		// had firewalls. A filter that never matched is not a run: saving it
		// would let 'posture changes' report a clean comparison of nothing.
		var last int
		err := db.DB().QueryRowContext(ctx, `SELECT devices FROM posture_runs WHERE scope = ? ORDER BY taken_at DESC LIMIT 1`, scope).Scan(&last)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && last == 0) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading posture runs: %w", err)
		}
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	runID := newRunID(takenAt)
	stamp := takenAt.UTC().Format(snapshotTimeLayout) // fixed width, so text order is time order
	done := 0
	if complete {
		done = 1
	}
	// The run row is written even for a zero-firewall run that empties a scope,
	// so an emptied fleet shows up in 'posture changes' instead of disappearing.
	if _, err := tx.ExecContext(ctx, `INSERT INTO posture_runs (run_id, taken_at, scope, complete, devices) VALUES (?, ?, ?, ?, ?)`,
		runID, stamp, scope, done, len(rows)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("writing posture run: %w", err)
	}
	for _, r := range rows {
		checks, err := json.Marshal(r.checks())
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO posture_snapshot_rows (run_id, taken_at, scope, complete, serial, name, checks) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runID, stamp, scope, done, r.Serial, r.Name, string(checks)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("writing snapshot: %w", err)
		}
	}
	return tx.Commit()
}

type postureRun struct {
	RunID    string
	TakenAt  time.Time
	Scope    string
	Complete bool
}

type postureSnapshot struct {
	Run    postureRun
	Checks map[string]map[string]string
	Names  map[string]string
}

// loadPostureRuns lists saved runs, newest first. A store without the runs table
// (posture never ran) yields no runs and no error.
func loadPostureRuns(ctx context.Context, db *sql.DB) ([]postureRun, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='posture_runs'`).Scan(&n); err != nil || n == 0 {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT run_id, taken_at, scope, complete FROM posture_runs ORDER BY taken_at DESC`)
	if err != nil {
		return nil, err
	}
	type raw struct {
		id, at, scope string
		complete      int
	}
	collected := make([]raw, 0)
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.id, &r.at, &r.scope, &r.complete); err != nil {
			_ = rows.Close()
			return nil, err
		}
		collected = append(collected, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	out := make([]postureRun, 0, len(collected))
	for _, r := range collected {
		t, err := time.Parse(time.RFC3339Nano, r.at)
		if err != nil {
			return nil, fmt.Errorf("posture run %s has an unreadable timestamp %q", r.id, r.at)
		}
		out = append(out, postureRun{RunID: r.id, TakenAt: t, Scope: r.scope, Complete: r.complete == 1})
	}
	return out, nil
}

func loadPostureSnapshot(ctx context.Context, db *sql.DB, run postureRun) (postureSnapshot, error) {
	snap := postureSnapshot{Run: run, Checks: map[string]map[string]string{}, Names: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT serial, COALESCE(name, ''), checks FROM posture_snapshot_rows WHERE run_id = ?`, run.RunID)
	if err != nil {
		return snap, err
	}
	defer rows.Close()
	for rows.Next() {
		var serial, name, checks string
		if err := rows.Scan(&serial, &name, &checks); err != nil {
			return snap, err
		}
		m := map[string]string{}
		if err := json.Unmarshal([]byte(checks), &m); err != nil {
			return snap, fmt.Errorf("posture run %s, firewall %s: corrupt checks JSON: %w", run.RunID, serial, err)
		}
		snap.Checks[serial] = m
		snap.Names[serial] = name
	}
	return snap, rows.Err()
}
