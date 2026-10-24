// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written snapshot schema for Cove fleet history. Lives beside the
// generated store.go (separate file, lazy init) so regeneration never
// touches it. The vendor console keeps no history; these tables are what
// make `storage growth` and `devices changes` possible.
package store

import (
	"context"
	"cove-pp-cli/internal/coverpc"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CoveSnapshot is one timestamped capture of fleet statistics.
//
// Endpoint and Complete make two snapshots comparable (hand-fix code:
// cove-snapshot-scope-and-completeness): a diff is only meaningful between
// captures of the same partner tree at the same API endpoint, and a device
// missing from a capture that stopped at its scan cap was not removed.
// Complete is nil for a snapshot written before completeness was recorded.
type CoveSnapshot struct {
	ID            int64     `json:"id"`
	TakenAt       time.Time `json:"taken_at"`
	RootPartnerID int64     `json:"root_partner_id"`
	DeviceCount   int       `json:"device_count"`
	Endpoint      string    `json:"endpoint,omitempty"`
	Complete      *bool     `json:"complete,omitempty"`
}

// KnownComplete reports whether the snapshot is recorded as a complete scan.
func (s CoveSnapshot) KnownComplete() bool { return s.Complete != nil && *s.Complete }

// KnownScope reports whether the snapshot records what it captured: a partner
// tree and an API endpoint. A snapshot written before endpoint tracking reads
// an empty endpoint and its scope is unknown.
// An endpoint recorded in an older binding form is unknown too: that form
// aliased distinct destinations, so it cannot prove two snapshots share one.
func (s CoveSnapshot) KnownScope() bool {
	return coverpc.EndpointBindingIsCurrent(s.Endpoint) && s.RootPartnerID > 0
}

// CoveSnapshotsComparable is the one rule every snapshot comparison applies
// (hand-fix code: cove-snapshot-scope-and-completeness): two distinct
// snapshots are diffed only when both carry a known scope and it is the same
// partner tree at the same endpoint. A snapshot of unknown scope is never
// compared with anything, since it may come from another endpoint or tenant.
func CoveSnapshotsComparable(a, b CoveSnapshot) bool {
	return a.ID != b.ID && a.KnownScope() && b.KnownScope() &&
		a.Endpoint == b.Endpoint && a.RootPartnerID == b.RootPartnerID
}

// CoveSnapshotScope is what a snapshot was a capture of.
type CoveSnapshotScope struct {
	RootPartnerID int64
	Endpoint      string
	// Complete is true when the scan exhausted the partner tree (no scan cap).
	Complete bool
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
	// Databases written before scope and completeness were recorded gain the
	// columns in place; their old rows read as endpoint '' and complete NULL
	// (unknown), so they never pass for a verified complete capture.
	have := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(cove_snapshots)`)
	if err != nil {
		return fmt.Errorf("inspecting cove snapshot table: %w", err)
	}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("inspecting cove snapshot table: %w", err)
		}
		have[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, col := range []struct{ name, ddl string }{
		{"endpoint", `ALTER TABLE cove_snapshots ADD COLUMN endpoint TEXT NOT NULL DEFAULT ''`},
		{"complete", `ALTER TABLE cove_snapshots ADD COLUMN complete INTEGER`},
	} {
		if have[col.name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, col.ddl); err != nil {
			return fmt.Errorf("adding cove_snapshots.%s: %w", col.name, err)
		}
	}
	return nil
}

// InsertCoveSnapshot writes a snapshot whose endpoint and completeness are
// unknown (they read back as ” and nil). The snapshot command records both
// through InsertCoveSnapshotScoped.
func (s *Store) InsertCoveSnapshot(ctx context.Context, takenAt time.Time, rootPartnerID int64, rows []CoveDeviceStat) (int64, error) {
	return s.insertCoveSnapshot(ctx, takenAt, rootPartnerID, "", nil, rows)
}

// InsertCoveSnapshotScoped writes a snapshot header (with what it captured:
// partner tree, endpoint, and whether the scan was complete) plus its device
// rows in one transaction and returns the snapshot id.
func (s *Store) InsertCoveSnapshotScoped(ctx context.Context, takenAt time.Time, scope CoveSnapshotScope, rows []CoveDeviceStat) (int64, error) {
	complete := scope.Complete
	return s.insertCoveSnapshot(ctx, takenAt, scope.RootPartnerID, scope.Endpoint, &complete, rows)
}

func (s *Store) insertCoveSnapshot(ctx context.Context, takenAt time.Time, rootPartnerID int64, endpoint string, complete *bool, rows []CoveDeviceStat) (int64, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return 0, err
	}
	var completeCol any
	if complete != nil {
		completeCol = *complete
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO cove_snapshots (taken_at, root_partner_id, device_count, endpoint, complete) VALUES (?, ?, ?, ?, ?)`,
		takenAt.UTC().Unix(), rootPartnerID, len(rows), endpoint, completeCol)
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
		`SELECT `+coveSnapshotCols+` FROM cove_snapshots ORDER BY taken_at DESC, id DESC LIMIT 1`)
	return scanCoveSnapshot(row)
}

// CoveSnapshotBefore returns the newest snapshot taken at or before cutoff,
// or ok=false when none qualifies. It ignores scope, so it never picks a
// comparison baseline: comparisons use CoveSnapshotBeforeInScope and
// CoveSnapshotsComparable.
func (s *Store) CoveSnapshotBefore(ctx context.Context, cutoff time.Time) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+coveSnapshotCols+` FROM cove_snapshots WHERE taken_at <= ? ORDER BY taken_at DESC, id DESC LIMIT 1`,
		cutoff.UTC().Unix())
	return scanCoveSnapshot(row)
}

// OldestCoveSnapshotSince returns the oldest snapshot taken at or after the
// cutoff, or ok=false when none. It ignores scope, so it never picks a
// comparison baseline: comparisons use CoveSnapshotBeforeInScope and
// CoveSnapshotsComparable.
func (s *Store) OldestCoveSnapshotSince(ctx context.Context, cutoff time.Time) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+coveSnapshotCols+` FROM cove_snapshots WHERE taken_at >= ? ORDER BY taken_at ASC, id ASC LIMIT 1`,
		cutoff.UTC().Unix())
	return scanCoveSnapshot(row)
}

// CoveSnapshotBeforeInScope returns the newest snapshot taken at or before
// cutoff of the same partner tree at the same endpoint, or ok=false when
// none qualifies. Comparisons pick their baseline here, never across scopes:
// an unknown scope (an empty endpoint or partner 0) matches nothing.
func (s *Store) CoveSnapshotBeforeInScope(ctx context.Context, cutoff time.Time, rootPartnerID int64, endpoint string) (CoveSnapshot, bool, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return CoveSnapshot{}, false, err
	}
	if !(CoveSnapshot{RootPartnerID: rootPartnerID, Endpoint: endpoint}).KnownScope() {
		return CoveSnapshot{}, false, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+coveSnapshotCols+` FROM cove_snapshots WHERE taken_at <= ? AND root_partner_id = ? AND endpoint = ? ORDER BY taken_at DESC, id DESC LIMIT 1`,
		cutoff.UTC().Unix(), rootPartnerID, endpoint)
	return scanCoveSnapshot(row)
}

const coveSnapshotCols = `id, taken_at, root_partner_id, device_count, endpoint, complete`

func scanCoveSnapshot(row *sql.Row) (CoveSnapshot, bool, error) {
	var snap CoveSnapshot
	var takenAt int64
	var complete sql.NullBool
	err := row.Scan(&snap.ID, &takenAt, &snap.RootPartnerID, &snap.DeviceCount, &snap.Endpoint, &complete)
	if complete.Valid {
		v := complete.Bool
		snap.Complete = &v
	}
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
// SettingsJSON is loaded too: the hot columns store 0 for a column the
// server did not report, and only the settings map tells the two apart.
func (s *Store) CoveDeviceStats(ctx context.Context, snapshotID int64) (map[int64]CoveDeviceStat, error) {
	if err := s.EnsureCoveSnapshotTables(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_id, account_id, partner_id, device_name, customer,
used_storage, last_status, last_success_at, sku, prev_sku, settings_json FROM cove_device_stats WHERE snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]CoveDeviceStat{}
	for rows.Next() {
		var r CoveDeviceStat
		if err := rows.Scan(&r.SnapshotID, &r.AccountID, &r.PartnerID, &r.DeviceName, &r.Customer,
			&r.UsedStorage, &r.LastStatus, &r.LastSuccessAt, &r.SKU, &r.PrevSKU, &r.SettingsJSON); err != nil {
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
