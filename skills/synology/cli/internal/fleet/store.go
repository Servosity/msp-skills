// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package fleet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"synology-pp-cli/internal/cliutil"
)

// DBPath is <data dir>/fleet.db: one mirror across every NAS, independent of
// the per-credential store the generated commands use.
func DBPath() (string, error) {
	dir, err := cliutil.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fleet"+dbExt), nil
}

const dbExt = ".db"

const schema = `
CREATE TABLE IF NOT EXISTS fleet_sync (
  nas TEXT PRIMARY KEY, url TEXT, client TEXT,
  attempted_at INTEGER, succeeded_at INTEGER, error TEXT,
  model TEXT, dsm_version TEXT, abb_installed INTEGER,
  abb_seen INTEGER,                   -- 1 once any sync found ABB installed
  history_from INTEGER, history_to INTEGER, -- results are complete over [from, to]
  mirror_format INTEGER
);
CREATE TABLE IF NOT EXISTS abb_tasks (
  nas TEXT, task_id INTEGER, task_name TEXT, backup_type INTEGER,
  running INTEGER, scheduled INTEGER, next_trigger_time INTEGER,
  last_result_id INTEGER, last_status INTEGER, last_job_action INTEGER,
  last_time_start INTEGER, last_time_end INTEGER, synced_at INTEGER, run_start INTEGER,
  PRIMARY KEY (nas, task_id)
);
CREATE TABLE IF NOT EXISTS abb_devices (
  nas TEXT, task_id INTEGER, device_id INTEGER, host_name TEXT, os_name TEXT,
  agent_status TEXT, backup_type INTEGER, synced_at INTEGER,
  PRIMARY KEY (nas, task_id, device_id)
);
CREATE TABLE IF NOT EXISTS abb_results (
  nas TEXT, result_id INTEGER, task_id INTEGER, task_name TEXT, status INTEGER,
  job_action INTEGER, backup_type INTEGER, time_start INTEGER, time_end INTEGER,
  error_count INTEGER, warning_count INTEGER, transfered_bytes INTEGER,
  hosts TEXT, first_error TEXT, device_ids TEXT,
  PRIMARY KEY (nas, result_id)
);
CREATE TABLE IF NOT EXISTS abb_device_runs (
  nas TEXT, device_result_id INTEGER, result_id INTEGER, device_id INTEGER,
  device_name TEXT, status INTEGER, time_start INTEGER, time_end INTEGER,
  transfered_bytes INTEGER,
  PRIMARY KEY (nas, device_result_id)
);
CREATE TABLE IF NOT EXISTS storage_snapshots (
  nas TEXT, taken_at INTEGER, kind TEXT, item_id TEXT, name TEXT, status TEXT,
  smart_status TEXT, temp INTEGER, size_total INTEGER, size_used INTEGER, detail TEXT,
  PRIMARY KEY (nas, taken_at, kind, item_id)
);
CREATE INDEX IF NOT EXISTS abb_results_task ON abb_results (nas, task_id, job_action, time_start);
CREATE INDEX IF NOT EXISTS abb_device_runs_dev ON abb_device_runs (nas, device_id, status);
`

// snapshotRetention bounds storage history kept for days-to-full slopes.
const snapshotRetention = 90 * 24 * time.Hour

// addedColumns are columns later than the first fleet.db layout; EnsureSchema
// adds any that an older mirror lacks.
var addedColumns = [][3]string{
	{"fleet_sync", "abb_seen", "INTEGER"}, {"fleet_sync", "history_from", "INTEGER"},
	{"fleet_sync", "history_to", "INTEGER"}, {"abb_tasks", "run_start", "INTEGER"},
	{"fleet_sync", "mirror_format", "INTEGER"}, {"abb_results", "device_ids", "TEXT"},
}

// MirrorFormat versions what a successful sync writes. A NAS whose last sync
// was written by an older version (smaller or NULL mirror_format) reads as
// not synced until it syncs again, so data written under older, looser rules
// is never reported as current.
const MirrorFormat = 11 // 3: results record protected device ids; 4: transfers checked against them; 5: transfer parents must be backups; 6-8: every last_result identity mirrored and checked; 9: unfinished task runs stored as running; 10-11: transfers within their run's time (checked after merging)

// EnsureSchema creates the fleet tables and migrates an older mirror.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("creating fleet tables: %w", err)
	}
	for _, c := range addedColumns {
		has, err := hasColumn(ctx, db, c[0], c[1])
		if err != nil {
			return err
		}
		if !has {
			if err := addColumn(ctx, db, c[0], c[1], c[2]); err != nil {
				return fmt.Errorf("migrating fleet tables: %w", err)
			}
		}
	}
	return nil
}

// addColumn adds one column and backfills it in one transaction. A
// concurrent process that added it first wins ("duplicate column name").
func addColumn(ctx context.Context, db *sql.DB, table, column, typ string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+typ); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		}
		return err
	}
	if table == "fleet_sync" && column == "abb_seen" {
		// An older mirror's evidence that ABB answered must survive the next
		// DSM 102, or a removed ABB would read as never installed.
		if _, err := tx.ExecContext(ctx, `UPDATE fleet_sync SET abb_seen = 1 WHERE abb_installed = 1
  OR EXISTS (SELECT 1 FROM abb_tasks t WHERE t.nas = fleet_sync.nas)
  OR EXISTS (SELECT 1 FROM abb_devices d WHERE d.nas = fleet_sync.nas)
  OR EXISTS (SELECT 1 FROM abb_results r WHERE r.nas = fleet_sync.nas)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NeedsMigration reports whether a mirror lacks a column this version reads.
func NeedsMigration(ctx context.Context, db Querier) (bool, error) {
	for _, c := range addedColumns {
		has, err := hasColumn(ctx, db, c[0], c[1])
		if err != nil || !has {
			return true, err
		}
	}
	return false, nil
}

func hasColumn(ctx context.Context, db Querier, table, column string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	return n > 0, err
}

// execer is the write surface shared by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ErrSuperseded: a sync of the same NAS that started later has already
// committed, so this older fetch is discarded rather than overwrite it.
var ErrSuperseded = errors.New("a later sync of this NAS committed first; this fetch was discarded")

// CommitSync writes a NAS's snapshot (when non-nil) and the attempt's outcome
// in one transaction, so a report never sees new data under an old freshness
// marker or a fresh marker over partial data.
func CommitSync(ctx context.Context, db *sql.DB, n NAS, at time.Time, snap *Snapshot, info SystemInfo, syncErr error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := claim(ctx, tx, n, at); err != nil {
		return err
	}
	var from, to int64
	if snap != nil {
		if err := writeSnapshot(ctx, tx, n.Name, snap); err != nil {
			return err
		}
		if snap.HistoryFrom > 0 {
			from, to = snap.HistoryFrom, snap.TakenAt.Unix()
		}
	}
	if err := recordAttempt(ctx, tx, n, at, info, syncErr, from, to); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordAttempt stamps a sync attempt with no data; err == nil marks it
// successful.
func RecordAttempt(ctx context.Context, db *sql.DB, n NAS, at time.Time, info SystemInfo, syncErr error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := claim(ctx, tx, n, at); err != nil {
		return err
	}
	if err := recordAttempt(ctx, tx, n, at, info, syncErr, 0, 0); err != nil {
		return err
	}
	return tx.Commit()
}

// claim makes the NAS's rows safe to write for a sync that started at at.
// Rows under this name with no sync record, or recorded against another URL,
// belong to some other box (task and device ids are small integers that
// collide across NAS units), so they are purged. A sync that started later
// and already committed wins: ErrSuperseded.
// ponytail: second resolution; two syncs of one NAS started in the same
// second both commit, last writer wins.
func claim(ctx context.Context, tx *sql.Tx, n NAS, at time.Time) error {
	var prevURL string
	var prevAt, prevFormat int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(url,''), COALESCE(attempted_at,0), COALESCE(mirror_format,0) FROM fleet_sync WHERE nas = ?`, n.Name).Scan(&prevURL, &prevAt, &prevFormat)
	switch {
	case err == sql.ErrNoRows:
		return purge(ctx, tx, n.Name)
	case err != nil:
		return err
	case prevAt > at.Unix():
		// Checked before the origin: an old sync of a replaced box must not
		// purge the replacement's newer history.
		return ErrSuperseded
	case prevURL != n.URL:
		return purge(ctx, tx, n.Name) // another box's rows
	case prevFormat < MirrorFormat:
		// Rows written under older rules: drop the data and its coverage and
		// start over, but keep the sync record, whose ABB evidence still holds.
		if _, err := tx.ExecContext(ctx, `UPDATE fleet_sync SET history_from = NULL, history_to = NULL WHERE nas = ?`, n.Name); err != nil {
			return err
		}
		return purgeData(ctx, tx, n.Name)
	}
	return nil
}

// recordAttempt upserts the outcome. History coverage [from, to] extends the
// recorded interval when it overlaps it and replaces it after a gap, so a
// report never treats unfetched time as covered.
func recordAttempt(ctx context.Context, db execer, n NAS, at time.Time, info SystemInfo, syncErr error, from, to int64) error {
	errText := ""
	var succeeded, hFrom, hTo any
	if syncErr != nil {
		errText = syncErr.Error()
	} else {
		succeeded = at.Unix()
	}
	if from > 0 {
		hFrom, hTo = from, to
	}
	seen := 0
	if info.ABBInstalled != nil && *info.ABBInstalled {
		seen = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO fleet_sync (nas, url, client, attempted_at, succeeded_at, error, model, dsm_version, abb_installed, abb_seen, history_from, history_to, mirror_format)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(nas) DO UPDATE SET url=excluded.url, client=excluded.client, attempted_at=excluded.attempted_at, mirror_format=excluded.mirror_format,
  succeeded_at=COALESCE(excluded.succeeded_at, fleet_sync.succeeded_at), error=excluded.error,
  model=COALESCE(NULLIF(excluded.model,''), fleet_sync.model),
  dsm_version=COALESCE(NULLIF(excluded.dsm_version,''), fleet_sync.dsm_version),
  abb_installed=COALESCE(excluded.abb_installed, fleet_sync.abb_installed),
  abb_seen=MAX(COALESCE(fleet_sync.abb_seen,0), COALESCE(fleet_sync.abb_installed,0), excluded.abb_seen),
  history_from=CASE WHEN excluded.history_from IS NULL THEN fleet_sync.history_from
    WHEN fleet_sync.history_from IS NULL OR fleet_sync.history_to IS NULL OR excluded.history_from > fleet_sync.history_to THEN excluded.history_from
    ELSE MIN(fleet_sync.history_from, excluded.history_from) END,
  history_to=CASE WHEN excluded.history_to IS NULL THEN fleet_sync.history_to
    WHEN fleet_sync.history_to IS NULL OR excluded.history_from > fleet_sync.history_to THEN excluded.history_to
    ELSE MAX(fleet_sync.history_to, excluded.history_to) END`,
		n.Name, n.URL, n.Client, at.Unix(), succeeded, errText, info.Model, info.DSMVersion, info.ABBInstalled, seen, hFrom, hTo, MirrorFormat)
	return err
}

func writeSnapshot(ctx context.Context, tx *sql.Tx, nas string, s *Snapshot) error {
	now := s.TakenAt.Unix()
	if s.ABBAbsent {
		// The package is gone: its tasks and devices are no longer current.
		for _, q := range []string{`DELETE FROM abb_tasks WHERE nas = ?`, `DELETE FROM abb_devices WHERE nas = ?`} {
			if _, err := tx.ExecContext(ctx, q, nas); err != nil {
				return err
			}
		}
	}
	if s.ABBInstalled {
		for _, q := range []string{`DELETE FROM abb_tasks WHERE nas = ?`, `DELETE FROM abb_devices WHERE nas = ?`} {
			if _, err := tx.ExecContext(ctx, q, nas); err != nil {
				return err
			}
		}
		for _, t := range s.Tasks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO abb_tasks VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				nas, t.TaskID, t.Name, t.BackupType, b2i(t.Running), b2i(t.Scheduled), t.NextTrigger,
				t.LastResultID, t.LastStatus, t.LastJobAction, t.LastStart, t.LastEnd, now, t.RunStart); err != nil {
				return err
			}
			for _, d := range t.Devices {
				if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO abb_devices VALUES (?,?,?,?,?,?,?,?)`,
					nas, t.TaskID, d.DeviceID, d.HostName, d.OSName, d.AgentStatus, d.BackupType, now); err != nil {
					return err
				}
			}
		}
		for _, r := range s.Results {
			if r.FromTask {
				if err := writeTaskResult(ctx, tx, nas, r); err != nil {
					return err
				}
				continue
			}
			if err := writeListedResult(ctx, tx, nas, r); err != nil {
				return err
			}
		}
		for _, d := range s.DeviceRuns {
			if err := writeDeviceRun(ctx, tx, nas, d); err != nil {
				return err
			}
		}
	}
	if s.StorageOK {
		// A second fetch in the same second replaces that snapshot wholesale;
		// leftover rows would make an empty answer look like the old one.
		if _, err := tx.ExecContext(ctx, `DELETE FROM storage_snapshots WHERE nas = ? AND taken_at = ?`, nas, now); err != nil {
			return err
		}
	}
	for _, it := range s.Storage {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO storage_snapshots VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			nas, now, it.Kind, it.ID, it.Name, it.Status, it.SmartStatus, it.Temp, it.SizeTotal, it.SizeUsed, it.Detail); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM storage_snapshots WHERE nas = ? AND taken_at < ?`,
		nas, s.TakenAt.Add(-snapshotRetention).Unix())
	return err
}

// mergeRun applies the one rule for a run seen again: its identity never
// changes; an unfinished status (0 or running) gives way to a final one; a
// final status never goes back to running; two different final statuses are
// inconsistent evidence. A known end time is kept; two different ones are
// inconsistent too. ok=false means the commit must fail.
func mergeRun(oldStatus, newStatus, oldEnd, newEnd int64) (status, end int64, ok bool) {
	if oldEnd > 0 && newEnd > 0 && oldEnd != newEnd {
		return 0, 0, false
	}
	end = max(oldEnd, newEnd)
	unfinished := func(s int64) bool { return s == 0 || s == StatusRunning }
	switch {
	case oldStatus == newStatus, unfinished(oldStatus):
		return newStatus, end, true
	case unfinished(newStatus):
		return oldStatus, end, true
	}
	return 0, 0, false
}

// writeListedResult upserts a run from the result listing under mergeRun:
// measurements (hosts, counts) come from the listing, the outcome and end
// time may only move forward, and a contradiction fails the commit.
func writeListedResult(ctx context.Context, tx *sql.Tx, nas string, r Result) error {
	var taskID, status, action, start, end int64
	var deviceIDs string
	err := tx.QueryRowContext(ctx, `SELECT task_id, status, job_action, time_start, COALESCE(time_end,0), COALESCE(device_ids,'')
  FROM abb_results WHERE nas = ? AND result_id = ?`,
		nas, r.ResultID).Scan(&taskID, &status, &action, &start, &end, &deviceIDs)
	if err == sql.ErrNoRows {
		if err := checkTransfersFit(ctx, tx, nas, r.ResultID, r.JobAction, normalizeIDs(r.DeviceIDs), r.Start, r.End); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO abb_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			nas, r.ResultID, r.TaskID, r.TaskName, r.Status, r.JobAction, r.BackupType, r.Start, r.End,
			r.Errors, r.Warnings, r.Bytes, r.Hosts, r.FirstError, normalizeIDs(r.DeviceIDs))
		return err
	}
	if err != nil {
		return err
	}
	// The devices a run protected are part of its identity: a later copy
	// may omit them (kept) but never name different ones.
	deviceIDs, r.DeviceIDs = normalizeIDs(deviceIDs), normalizeIDs(r.DeviceIDs)
	if taskID != r.TaskID || action != r.JobAction || start != r.Start || (deviceIDs != "" && r.DeviceIDs != "" && deviceIDs != r.DeviceIDs) {
		return fmt.Errorf("result %d changed identity since it was mirrored", r.ResultID)
	}
	if r.DeviceIDs == "" {
		r.DeviceIDs = deviceIDs
	}
	st, en, ok := mergeRun(status, r.Status, end, r.End)
	if ok {
		if err := checkTransfersFit(ctx, tx, nas, r.ResultID, r.JobAction, r.DeviceIDs, r.Start, en); err != nil {
			return err
		}
	}
	if !ok {
		return fmt.Errorf("result %d was %s with end %d and is now %s with end %d", r.ResultID,
			StatusLabel(int(status)), end, StatusLabel(int(r.Status)), r.End)
	}
	_, err = tx.ExecContext(ctx, `UPDATE abb_results SET task_name = ?, status = ?, backup_type = ?, time_end = ?,
  error_count = ?, warning_count = ?, transfered_bytes = ?, hosts = ?, first_error = COALESCE(NULLIF(?,''), first_error),
  device_ids = ? WHERE nas = ? AND result_id = ?`,
		r.TaskName, st, r.BackupType, en, r.Errors, r.Warnings, r.Bytes, r.Hosts, r.FirstError, r.DeviceIDs, nas, r.ResultID)
	return err
}

// writeDeviceRun upserts a per-device transfer under the same mergeRun rule.
// A transfer may only name a device its parent result protected, when the
// mirror knows that result's device set.
func writeDeviceRun(ctx context.Context, tx *sql.Tx, nas string, d DeviceRun) error {
	var resultID, deviceID, status, start, end int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(result_id,0), device_id, status, time_start, COALESCE(time_end,0) FROM abb_device_runs
  WHERE nas = ? AND device_result_id = ?`, nas, d.DeviceResultID).Scan(&resultID, &deviceID, &status, &start, &end)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	exists := err == nil
	// Merge first: the parent checks below judge the row as it will be stored.
	st, en := d.Status, d.End
	if exists {
		if deviceID != d.DeviceID || start != d.Start || (resultID > 0 && d.ResultID > 0 && resultID != d.ResultID) {
			return fmt.Errorf("device transfer %d changed identity since it was mirrored", d.DeviceResultID)
		}
		var ok bool
		if st, en, ok = mergeRun(status, d.Status, end, d.End); !ok {
			return fmt.Errorf("device transfer %d was %s and is now %s", d.DeviceResultID, StatusLabel(int(status)), StatusLabel(int(d.Status)))
		}
	}
	// The parent is the transfer's own result_id or, when it omits one, the
	// one already mirrored for it.
	parent := d.ResultID
	if parent <= 0 {
		parent = resultID
	}
	if parent > 0 {
		var ids string
		var action, pStart, pEnd int64
		perr := tx.QueryRowContext(ctx, `SELECT COALESCE(device_ids,''), job_action, time_start, COALESCE(time_end,0) FROM abb_results
  WHERE nas = ? AND result_id = ?`, nas, parent).Scan(&ids, &action, &pStart, &pEnd)
		if perr != nil && perr != sql.ErrNoRows {
			return perr
		}
		if perr == nil && action != JobBackup {
			return fmt.Errorf("device transfer %d belongs to result %d, which is not a backup", d.DeviceResultID, parent)
		}
		if perr == nil && !withinRun(d.Start, en, pStart, pEnd) {
			return fmt.Errorf("device transfer %d falls outside the time of its result %d", d.DeviceResultID, parent)
		}
		if ids != "" && !strings.Contains(normalizeIDs(ids), ","+strconv.FormatInt(d.DeviceID, 10)+",") {
			return fmt.Errorf("device transfer %d names device %d, which result %d did not protect", d.DeviceResultID, d.DeviceID, parent)
		}
	}
	if !exists {
		_, err = tx.ExecContext(ctx, `INSERT INTO abb_device_runs VALUES (?,?,?,?,?,?,?,?,?)`,
			nas, d.DeviceResultID, d.ResultID, d.DeviceID, d.DeviceName, d.Status, d.Start, d.End, d.Bytes)
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE abb_device_runs SET result_id = MAX(COALESCE(result_id,0), ?), device_name = ?, status = ?, time_end = ?, transfered_bytes = ?
  WHERE nas = ? AND device_result_id = ?`, d.ResultID, d.DeviceName, st, en, d.Bytes, nas, d.DeviceResultID)
	return err
}

// withinRun reports whether a transfer [start, end] (end 0 = open) fits its
// run [pStart, pEnd] (pEnd 0 = still open), give or take transferSkew.
func withinRun(start, end, pStart, pEnd int64) bool {
	if start < pStart-transferSkew {
		return false
	}
	if pEnd > 0 && (start > pEnd+transferSkew || end > pEnd+transferSkew) {
		return false
	}
	return true
}

// transferSkew is how far (seconds) a device transfer may sit outside its
// parent run's start/end before the two are treated as contradictory.
const transferSkew = 3600

// checkTransfersFit fails when mirrored transfers of resultID contradict what
// is now known about it: a non-backup result has no device transfers, a
// known device set (ids) must contain every transferred device, and every
// transfer must fall within the run's [start, end] (end 0 = still open).
func checkTransfersFit(ctx context.Context, tx *sql.Tx, nas string, resultID, action int64, ids string, start, end int64) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM abb_device_runs WHERE nas = ? AND result_id = ?
  AND (? <> 1 OR (? <> '' AND instr(?, ',' || device_id || ',') = 0)
       OR time_start < ? - ? OR (? > 0 AND (time_end > ? + ? OR time_start > ? + ?)))`,
		nas, resultID, action, ids, ids, start, transferSkew, end, end, transferSkew, end, transferSkew).Scan(&n)
	if err == nil && n > 0 {
		err = fmt.Errorf("result %d (action %d, devices %q) contradicts a mirrored device transfer", resultID, action, strings.Trim(ids, ","))
	}
	return err
}

// writeTaskResult stores a run known only from its task's last_result. A run
// already mirrored keeps its hosts and counts; a mirrored run still marked
// running takes the final status; any other disagreement fails the commit.
func writeTaskResult(ctx context.Context, tx *sql.Tx, nas string, r Result) error {
	var taskID, status, action, start, end int64
	err := tx.QueryRowContext(ctx, `SELECT task_id, status, job_action, time_start, COALESCE(time_end,0) FROM abb_results WHERE nas = ? AND result_id = ?`,
		nas, r.ResultID).Scan(&taskID, &status, &action, &start, &end)
	switch {
	case err == sql.ErrNoRows:
		// A transfer mirrored earlier may already hang off this run.
		if err := checkTransfersFit(ctx, tx, nas, r.ResultID, r.JobAction, "", r.Start, r.End); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO abb_results VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			nas, r.ResultID, r.TaskID, r.TaskName, r.Status, r.JobAction, r.BackupType, r.Start, r.End, 0, 0, 0, "", "", "")
		return err
	case err != nil:
		return err
	case taskID != r.TaskID || action != r.JobAction || start != r.Start:
		return fmt.Errorf("result %d differs between its task and the mirror", r.ResultID)
	}
	// Only the outcome and end time may move forward; hosts and counts stay.
	st, en, ok := mergeRun(status, r.Status, end, r.End)
	if !ok {
		return fmt.Errorf("result %d is %s in the mirror but %s in its task", r.ResultID, StatusLabel(int(status)), StatusLabel(int(r.Status)))
	}
	if err := checkTransfersFit(ctx, tx, nas, r.ResultID, r.JobAction, "", start, en); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE abb_results SET status = ?, time_end = ? WHERE nas = ? AND result_id = ?`, st, en, nas, r.ResultID)
	return err
}

// ResultCursor is where the next result fetch should start for a NAS (0 if
// nothing is mirrored, or the mirror was recorded against another URL): the
// newest mirrored start, or the start of the oldest run still mirrored as
// running so its final status is fetched, whichever is earlier. Fetch
// subtracts a further day of overlap.
func ResultCursor(ctx context.Context, db *sql.DB, n NAS) (int64, error) {
	var url string
	var format int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(url,''), COALESCE(mirror_format,0) FROM fleet_sync WHERE nas = ?`, n.Name).Scan(&url, &format)
	if err == sql.ErrNoRows || (err == nil && (url != n.URL || format < MirrorFormat)) {
		return 0, nil // another box's or an older version's history: CommitSync purges it
	}
	if err != nil {
		return 0, err
	}
	nas := n.Name
	// A running row older than a week is one DSM dropped from its log; it
	// must not pin the cursor forever.
	var newest, oldestRunning sql.NullInt64
	err = db.QueryRowContext(ctx, `SELECT MAX(time_start),
  MIN(CASE WHEN status IN (0, 1) AND time_start >= (SELECT MAX(time_start) FROM abb_results WHERE nas = ?) - 7*86400 THEN time_start END)
  FROM abb_results WHERE nas = ?`, nas, nas).Scan(&newest, &oldestRunning)
	if err != nil || !newest.Valid {
		return 0, err
	}
	if oldestRunning.Valid && oldestRunning.Int64 > 0 && oldestRunning.Int64 < newest.Int64 {
		return oldestRunning.Int64, nil
	}
	return newest.Int64, nil
}

// PurgeNAS removes every mirrored row for a NAS in one transaction, so a sync
// committing meanwhile never sees half of them gone.
func PurgeNAS(ctx context.Context, db *sql.DB, nas string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := purge(ctx, tx, nas); err != nil {
		return err
	}
	return tx.Commit()
}

func purge(ctx context.Context, db execer, nas string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM fleet_sync WHERE nas = ?`, nas); err != nil {
		return err
	}
	return purgeData(ctx, db, nas)
}

// purgeData removes a NAS's mirrored data, keeping its fleet_sync record.
func purgeData(ctx context.Context, db execer, nas string) error {
	for _, t := range []string{"abb_tasks", "abb_devices", "abb_results", "abb_device_runs", "storage_snapshots"} {
		if _, err := db.ExecContext(ctx, `DELETE FROM `+t+` WHERE nas = ?`, nas); err != nil {
			return err
		}
	}
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
