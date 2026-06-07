// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written snapshot schema for Cove fleet history. Lives beside the
// generated store.go (separate file, lazy init) so regeneration never
// touches it. The vendor console keeps no history; these tables are what
// make `storage growth` and `devices changes` possible.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CoveSnapshot is one timestamped capture of fleet statistics.
type CoveSnapshot struct {
	ID            int64     `json:"id"`
	TakenAt       time.Time `json:"taken_at"`
	RootPartnerID int64     `json:"root_partner_id"`
	DeviceCount   int       `json:"device_count"`
}

// CoveDeviceStat is one device row inside a snapshot. Hot columns are
// extracted for SQL-side filtering; the full column-code map is preserved in
// SettingsJSON.
type CoveDeviceStat struct {
	SnapshotID    int64  `json:"snapshot_id"`
	AccountID     int64  `json:"account_id"`
	PartnerID     int64  `json:"partner_id"`
	DeviceName    string `json:"device_name"`
	Customer      string `json:"customer"`
	UsedStorage   int64  `json:"used_storage"`
	LastStatus    int64  `json:"last_status"`
	LastSuccessAt int64  `json:"last_success_at"`
	SKU           string `json:"sku"`
	PrevSKU       string `json:"prev_sku"`
	SettingsJSON  string `json:"settings_json,omitempty"`
}

// EnsureCoveSnapshotTables creates the snapshot tables when missing. Called
// lazily by every command that reads or writes snapshots.
func (s *Store) EnsureCoveSnapshotTables(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS cove_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  taken_at INTEGER NOT NULL,
  root_partner_id INTEGER NOT NULL DEFAULT 0,
  device_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS cove_device_stats (
  snapshot_id INTEGER NOT NULL REFERENCES cove_snapshots(id) ON DELETE CASCADE,
  account_id INTEGER NOT NULL,
  partner_id INTEGER NOT NULL DEFAULT 0,
  device_name TEXT NOT NULL DEFAULT '',
  customer TEXT NOT NULL DEFAULT '',
  used_storage INTEGER NOT NULL DEFAULT 0,
  last_status INTEGER NOT NULL DEFAULT 0,
  last_success_at INTEGER NOT NULL DEFAULT 0,
  sku TEXT NOT NULL DEFAULT '',
  prev_sku TEXT NOT NULL DEFAULT '',
  settings_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (snapshot_id, account_id)
);
CREATE INDEX IF NOT EXISTS idx_cove_device_stats_account ON cove_device_stats(account_id, snapshot_id);
`)
	if err != nil {
		return fmt.Errorf("ensuring cove snapshot tables: %w", err)
	}
	return nil
}

// InsertCoveSnapshot writes a snapshot header plus its device rows in one
// transaction and returns the snapshot id.
func (s *Store) InsertCoveSnapshot(ctx context.Context, takenAt time.Time, rootPartnerID int64, rows []CoveDeviceStat) (int64, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO cove_snapshots (taken_at, root_partner_id, device_count) VALUES (?, ?, ?)`,
		takenAt.UTC().Unix(), rootPartnerID, len(rows))
	if err != nil {
		return 0, fmt.Errorf("inserting snapshot header: %w", err)
	}
	snapID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO cove_device_stats
(snapshot_id, account_id, partner_id, device_name, customer, used_storage, last_status, last_success_at, sku, prev_sku, settings_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, r := range rows {
		settings := r.SettingsJSON
		if settings == "" {
			settings = "{}"
		}
		if _, err := stmt.ExecContext(ctx, snapID, r.AccountID, r.PartnerID, r.DeviceName, r.Customer,
			r.UsedStorage, r.LastStatus, r.LastSuccessAt, r.SKU, r.PrevSKU, settings); err != nil {
			return 0, fmt.Errorf("inserting device %d: %w", r.AccountID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return snapID, nil
}

// LatestCoveSnapshot returns the most recent snapshot header, or ok=false
// when none exist.
func (s *Store) LatestCoveSnapshot(ctx context.Context) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, taken_at, root_partner_id, device_count FROM cove_snapshots ORDER BY taken_at DESC, id DESC LIMIT 1`)
	return scanCoveSnapshot(row)
}

// CoveSnapshotBefore returns the newest snapshot taken at or before cutoff,
// or ok=false when none qualifies.
func (s *Store) CoveSnapshotBefore(ctx context.Context, cutoff time.Time) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, taken_at, root_partner_id, device_count FROM cove_snapshots WHERE taken_at <= ? ORDER BY taken_at DESC, id DESC LIMIT 1`,
		cutoff.UTC().Unix())
	return scanCoveSnapshot(row)
}

// OldestCoveSnapshotSince returns the oldest snapshot taken at or after the
// cutoff — the natural "baseline" for a --since window. ok=false when none.
func (s *Store) OldestCoveSnapshotSince(ctx context.Context, cutoff time.Time) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, taken_at, root_partner_id, device_count FROM cove_snapshots WHERE taken_at >= ? ORDER BY taken_at ASC, id ASC LIMIT 1`,
		cutoff.UTC().Unix())
	return scanCoveSnapshot(row)
}

func scanCoveSnapshot(row *sql.Row) (CoveSnapshot, bool, error) {
	var snap CoveSnapshot
	var takenAt int64
	err := row.Scan(&snap.ID, &takenAt, &snap.RootPartnerID, &snap.DeviceCount)
	if err == sql.ErrNoRows {
		return snap, false, nil
	}
	if err != nil {
		return snap, false, err
	}
	snap.TakenAt = time.Unix(takenAt, 0).UTC()
	return snap, true, nil
}

// CoveDeviceStats loads all device rows for a snapshot, keyed by account id.
func (s *Store) CoveDeviceStats(ctx context.Context, snapshotID int64) (map[int64]CoveDeviceStat, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_id, account_id, partner_id, device_name, customer,
used_storage, last_status, last_success_at, sku, prev_sku FROM cove_device_stats WHERE snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]CoveDeviceStat{}
	for rows.Next() {
		var r CoveDeviceStat
		if err := rows.Scan(&r.SnapshotID, &r.AccountID, &r.PartnerID, &r.DeviceName, &r.Customer,
			&r.UsedStorage, &r.LastStatus, &r.LastSuccessAt, &r.SKU, &r.PrevSKU); err != nil {
			return nil, err
		}
		out[r.AccountID] = r
	}
	return out, rows.Err()
}

// CoveSnapshotCount reports how many snapshots exist.
func (s *Store) CoveSnapshotCount(ctx context.Context) (int, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cove_snapshots`).Scan(&n)
	return n, err
}

// MarshalSettings renders a flattened column map as canonical JSON for the
// settings_json column.
func MarshalSettings(settings map[string]string) string {
	if len(settings) == 0 {
		return "{}"
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return "{}"
	}
	return string(data)
}
