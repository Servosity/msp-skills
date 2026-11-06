// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): health runs and inventory snapshots for the
// Windows Server checks. Recorded in handfixes.json (health-events). Created
// from migrateExtras (extras.go).

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

var winsrvMigrations = []string{
	`CREATE TABLE IF NOT EXISTS health_runs (
		run_id INTEGER PRIMARY KEY AUTOINCREMENT,
		server TEXT NOT NULL,
		ran_at TEXT NOT NULL,
		verdict TEXT NOT NULL,
		checks TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_health_runs_server ON health_runs(server, run_id)`,
	`CREATE TABLE IF NOT EXISTS inventory_runs (
		run_id INTEGER PRIMARY KEY AUTOINCREMENT,
		server TEXT NOT NULL,
		taken_at TEXT NOT NULL,
		errors TEXT NOT NULL DEFAULT '{}',
		absent TEXT NOT NULL DEFAULT '[]'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_inventory_runs_server ON inventory_runs(server, run_id)`,
	`CREATE TABLE IF NOT EXISTS inventory_rows (
		run_id INTEGER NOT NULL,
		section TEXT NOT NULL,
		row_key TEXT NOT NULL,
		data TEXT NOT NULL,
		PRIMARY KEY (run_id, section, row_key)
	)`,
}

// Every statement reaches SQLite through the kit's SQL driver (kitsql), which
// withholds a statement whose text or bound values repeat a held credential:
// the write fails whole, nothing of it is stored.

// HealthRun is one stored health result for one server.
type HealthRun struct {
	RunID   int64           `json:"run_id"`
	Server  string          `json:"server"`
	RanAt   string          `json:"ran_at"`
	Verdict string          `json:"verdict"`
	Checks  json.RawMessage `json:"checks"`
}

func missingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// LastHealthRun returns the newest stored run for server.
func (s *Store) LastHealthRun(ctx context.Context, server string) (HealthRun, bool, error) {
	var h HealthRun
	var checks string
	err := s.db.QueryRowContext(ctx, `SELECT run_id, server, ran_at, verdict, checks FROM health_runs
		WHERE server = ? ORDER BY run_id DESC LIMIT 1`, server).Scan(&h.RunID, &h.Server, &h.RanAt, &h.Verdict, &checks)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return HealthRun{}, false, nil
	}
	if err != nil {
		return HealthRun{}, false, err
	}
	h.Checks = json.RawMessage(checks)
	return h, true, nil
}

// RecordHealth stores a run and, in the same transaction, the events it
// produced, so a run is never recorded without its events or the reverse.
// It returns the run id and the seq of each event written.
func (s *Store) RecordHealth(ctx context.Context, h HealthRun, events []VendorEvent) (int64, []int64, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO health_runs (server, ran_at, verdict, checks) VALUES (?, ?, ?, ?)`,
		h.Server, h.RanAt, h.Verdict, string(h.Checks))
	if err != nil {
		return 0, nil, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, nil, err
	}
	var seqs []int64
	for _, e := range events {
		e.EventID = strings.ReplaceAll(e.EventID, "{run}", strconv.FormatInt(runID, 10))
		data := string(e.Data)
		r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO vendor_events
			(event_id, type, source, occurred_at, received_at, resource_type, resource_id, delivery_id, verified, data)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
			e.EventID, e.Type, e.Source, e.OccurredAt, e.ReceivedAt, e.ResourceType, e.ResourceID, e.Verified, data)
		if err != nil {
			return 0, nil, err
		}
		if n, _ := r.RowsAffected(); n > 0 {
			seq, _ := r.LastInsertId()
			seqs = append(seqs, seq)
		}
	}
	return runID, seqs, tx.Commit()
}

// InventoryRun is one stored snapshot header.
type InventoryRun struct {
	RunID   int64             `json:"run_id"`
	Server  string            `json:"server"`
	TakenAt string            `json:"taken_at"`
	Errors  map[string]string `json:"errors"`
	Absent  []string          `json:"absent"`
}

// RecordInventory stores one snapshot. sections maps section -> row key ->
// kept fields; a section absent from the map was not read.
func (s *Store) RecordInventory(ctx context.Context, run InventoryRun, sections map[string]map[string]map[string]any) (int64, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	errs, _ := json.Marshal(run.Errors)
	absent, _ := json.Marshal(run.Absent)
	if run.Absent == nil {
		absent = []byte("[]")
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO inventory_runs (server, taken_at, errors, absent) VALUES (?, ?, ?, ?)`,
		run.Server, run.TakenAt, string(errs), string(absent))
	if err != nil {
		return 0, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for section, rows := range sections {
		// A marker row records that the section was read even when it is empty.
		if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_rows (run_id, section, row_key, data) VALUES (?, ?, '', 'null')`, runID, section); err != nil {
			return 0, err
		}
		for key, fields := range rows {
			data, err := json.Marshal(fields)
			if err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_rows (run_id, section, row_key, data) VALUES (?, ?, ?, ?)`, runID, section, key, string(data)); err != nil {
				return 0, err
			}
		}
	}
	return runID, tx.Commit()
}

// InventoryRuns returns up to limit snapshot headers for server, newest first.
func (s *Store) InventoryRuns(ctx context.Context, server string, limit int) ([]InventoryRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, server, taken_at, errors, absent FROM inventory_runs
		WHERE server = ? ORDER BY run_id DESC LIMIT ?`, server, limit)
	if missingTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InventoryRun
	for rows.Next() {
		var r InventoryRun
		var errs, absent string
		if err := rows.Scan(&r.RunID, &r.Server, &r.TakenAt, &errs, &absent); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(errs), &r.Errors)
		_ = json.Unmarshal([]byte(absent), &r.Absent)
		out = append(out, r)
	}
	return out, rows.Err()
}

// InventorySections returns the stored sections of one snapshot.
func (s *Store) InventorySections(ctx context.Context, runID int64) (map[string]map[string]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT section, row_key, data FROM inventory_rows WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]map[string]any{}
	for rows.Next() {
		var section, key, data string
		if err := rows.Scan(&section, &key, &data); err != nil {
			return nil, err
		}
		if out[section] == nil {
			out[section] = map[string]map[string]any{}
		}
		if key == "" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(data), &fields); err != nil {
			return nil, err
		}
		out[section][key] = fields
	}
	return out, rows.Err()
}

// InventoryServers lists the servers with at least one snapshot.
func (s *Store) InventoryServers(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT server FROM inventory_runs ORDER BY server`)
	if missingTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
