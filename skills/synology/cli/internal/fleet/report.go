// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package fleet

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Severity orders findings; the worst one drives a command's exit code.
type Severity int

const (
	SevOK Severity = iota
	SevWarning
	SevCritical
	SevUnknown // no trustworthy data: never reported as OK
)

func (s Severity) String() string {
	return [...]string{"ok", "warning", "critical", "unknown"}[s]
}

func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// StorageSeverity classifies a DSM storage status. Only "normal" is attested in
// real captures, so any other value is a warning, never OK, and a missing
// status is unknown (dumbmonit metrics.rs:60-95 maps unknown labels the same way).
func StorageSeverity(status string) Severity {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "":
		return SevUnknown
	case "normal":
		return SevOK
	case "crashed", "critical", "failing", "fail", "failed", "error", "damaged", "degrade", "degraded",
		"broken", "system_partition_failed", "unformatted", "not_initialized", "offline":
		return SevCritical
	}
	return SevWarning
}

// Querier is the read surface shared by *sql.DB and *sql.Tx. The CLI runs a
// report inside one read transaction so every query sees the same snapshot.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SyncState is a NAS's mirror freshness.
type SyncState struct {
	NAS, URL, Client, Error, Model, DSMVersion string
	AttemptedAt, SucceededAt                   int64
	ABBInstalled                               sql.NullBool
	ABBSeen                                    bool  // ABB answered on some sync, or history exists
	HistoryFrom                                int64 // results are complete from here on (0 = none)
	Format                                     int64 // mirror_format of the last attempt
}

// LoadSyncStates returns fleet_sync rows keyed by NAS.
func LoadSyncStates(ctx context.Context, db Querier) (map[string]SyncState, error) {
	rows, err := db.QueryContext(ctx, `SELECT nas, COALESCE(url,''), COALESCE(client,''), COALESCE(attempted_at,0),
  COALESCE(succeeded_at,0), COALESCE(error,''), COALESCE(model,''), COALESCE(dsm_version,''), abb_installed,
  COALESCE(abb_seen,0) = 1 OR EXISTS (SELECT 1 FROM abb_results r WHERE r.nas = fleet_sync.nas),
  COALESCE(history_from,0), COALESCE(mirror_format,0) FROM fleet_sync`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SyncState{}
	for rows.Next() {
		var s SyncState
		if err := rows.Scan(&s.NAS, &s.URL, &s.Client, &s.AttemptedAt, &s.SucceededAt, &s.Error, &s.Model, &s.DSMVersion, &s.ABBInstalled, &s.ABBSeen, &s.HistoryFrom, &s.Format); err != nil {
			return nil, err
		}
		out[s.NAS] = s
	}
	return out, rows.Err()
}

// FreshFor reports whether n's mirror is current: the latest attempt
// succeeded with no error, against the URL n is registered with now, within
// maxAge.
func (s SyncState) FreshFor(n NAS, now time.Time, maxAge time.Duration) bool {
	return s.Error == "" && s.SucceededAt > 0 && s.SucceededAt >= s.AttemptedAt && s.URL == n.URL && s.Format >= MirrorFormat &&
		now.Sub(time.Unix(s.SucceededAt, 0)) <= maxAge
}

// ABBGone describes a NAS whose last sync got DSM error 102 for Active Backup
// for Business: abb_missing (a finding: ABB ran here before and its machines
// are no longer protected) or abb_not_installed (not a finding: this NAS never
// ran ABB). ok is false while ABB is installed or unknown.
func (s SyncState) ABBGone() (kind, note string, ok bool) {
	if !s.ABBInstalled.Valid || s.ABBInstalled.Bool {
		return "", "", false
	}
	if s.ABBSeen {
		return "abb_missing", "Active Backup for Business ran on this NAS but no longer answers (DSM error 102: uninstalled, stopped or needs repair); its devices are not being backed up", true
	}
	return "abb_not_installed", "Active Backup for Business is not installed on this NAS (DSM error 102)", true
}

// --- abb failures ---

// Failure is one task (or one NAS) needing attention.
type Failure struct {
	NAS        string `json:"nas"`
	Client     string `json:"client,omitempty"`
	Kind       string `json:"kind"` // run_failed, no_run_in_window, running_too_long, running_start_unknown, nas_not_synced, abb_missing, abb_not_installed
	Running    bool   `json:"running,omitempty"`
	TaskID     int64  `json:"task_id,omitempty"`
	TaskName   string `json:"task_name,omitempty"`
	Status     string `json:"status,omitempty"`
	ResultID   int64  `json:"result_id,omitempty"`
	TimeStart  int64  `json:"time_start,omitempty"`
	TimeEnd    int64  `json:"time_end,omitempty"`
	Errors     int64  `json:"error_count,omitempty"`
	Warnings   int64  `json:"warning_count,omitempty"`
	Hosts      string `json:"hosts,omitempty"`
	FirstError string `json:"first_error,omitempty"`
	LastRunAt  int64  `json:"last_run_at,omitempty"`
	Note       string `json:"note,omitempty"`
}

// Failures lists tasks whose newest backup run inside the window did not
// succeed, scheduled tasks with no backup run in the window (running tasks are
// skipped), and NAS units whose mirror is not fresh.
func Failures(ctx context.Context, db Querier, nasList []NAS, since time.Time, now time.Time, maxSyncAge time.Duration) ([]Failure, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Failure, 0)
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, Failure{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
			continue
		}
		if kind, note, gone := st.ABBGone(); gone {
			out = append(out, Failure{NAS: n.Name, Client: n.Client, Kind: kind, Note: note})
			continue
		}
		tasks, err := loadTasks(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		for _, t := range tasks {
			last, found, err := latestBackup(ctx, db, n.Name, t.TaskID, since.Unix())
			if err != nil {
				return nil, err
			}
			// A failed run inside the window stays a failure even while a newer
			// run is in progress: the new run has not succeeded yet.
			if found && last.Status != StatusSuccess {
				f := Failure{NAS: n.Name, Client: n.Client, Kind: "run_failed", TaskID: t.TaskID, TaskName: t.Name,
					Status: StatusLabel(int(last.Status)), ResultID: last.ResultID, TimeStart: last.Start, TimeEnd: last.End,
					Errors: last.Errors, Warnings: last.Warnings, Hosts: last.Hosts, FirstError: last.FirstError, Running: t.Running}
				if t.Running {
					f.Note = "a newer run is in progress"
				}
				out = append(out, f)
				continue
			}
			if t.Running {
				start := t.RunStart
				if start == 0 {
					if start, err = runningBackupStart(ctx, db, n.Name, t.TaskID); err != nil {
						return nil, err
					}
				}
				switch {
				case start == 0:
					out = append(out, Failure{NAS: n.Name, Client: n.Client, Kind: "running_start_unknown", TaskID: t.TaskID, Running: true,
						TaskName: t.Name, Note: "a backup is running but its start time is not known, so a hung run cannot be ruled out"})
				case start < since.Unix():
					out = append(out, Failure{NAS: n.Name, Client: n.Client, Kind: "running_too_long", TaskID: t.TaskID, Running: true,
						TaskName: t.Name, TimeStart: start, Note: "a backup has been running since before the window; it may be hung"})
				}
				continue // a backup in progress that started inside the window
			}
			if !found {
				if !t.Scheduled {
					continue
				}
				prev, _, err := latestBackup(ctx, db, n.Name, t.TaskID, 0)
				if err != nil {
					return nil, err
				}
				out = append(out, Failure{NAS: n.Name, Client: n.Client, Kind: "no_run_in_window", TaskID: t.TaskID,
					TaskName: t.Name, LastRunAt: prev.Start,
					Note: "no backup run since " + since.UTC().Format(time.RFC3339) + "; widen --since for tasks scheduled less often"})
				continue
			}
		}
	}
	return out, nil
}

func syncNote(st SyncState, ok bool, n NAS) string {
	switch {
	case !ok:
		return "never synced: run 'synology-cli fleet sync'"
	case st.Error != "":
		return "last sync failed: " + st.Error
	case st.URL != n.URL:
		return "the NAS's registered URL changed since its last sync; run 'synology-cli fleet sync'"
	case st.Format < MirrorFormat:
		return "the mirror was written by an older version of this CLI; run 'synology-cli fleet sync'"
	}
	return "mirror is older than --max-sync-age; run 'synology-cli fleet sync'"
}

// runningBackupStart is the start of the newest backup result for a task
// still mirrored as running (0 if none).
func runningBackupStart(ctx context.Context, db Querier, nas string, taskID int64) (int64, error) {
	var start sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT MAX(time_start) FROM abb_results
  WHERE nas = ? AND task_id = ? AND job_action = ? AND status IN (0, 1) AND time_start > 0`, nas, taskID, JobBackup).Scan(&start)
	return start.Int64, err
}

func loadTasks(ctx context.Context, db Querier, nas string) ([]Task, error) {
	rows, err := db.QueryContext(ctx, `SELECT task_id, task_name, running, scheduled, backup_type, COALESCE(last_time_start,0), COALESCE(run_start,0),
  COALESCE(last_result_id,0), COALESCE(last_status,0), COALESCE(last_job_action,0), COALESCE(last_time_end,0)
  FROM abb_tasks WHERE nas = ? ORDER BY task_name`, nas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		var running, scheduled int
		if err := rows.Scan(&t.TaskID, &t.Name, &running, &scheduled, &t.BackupType, &t.LastStart, &t.RunStart,
			&t.LastResultID, &t.LastStatus, &t.LastJobAction, &t.LastEnd); err != nil {
			return nil, err
		}
		t.Running, t.Scheduled = running == 1, scheduled == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func latestBackup(ctx context.Context, db Querier, nas string, taskID, since int64) (Result, bool, error) {
	var r Result
	err := db.QueryRowContext(ctx, `SELECT result_id, status, time_start, time_end, error_count, warning_count,
  COALESCE(hosts,''), COALESCE(first_error,'') FROM abb_results
  WHERE nas = ? AND task_id = ? AND job_action = ? AND status NOT IN (0, 1) AND (time_start >= ? OR time_end >= ?)
  ORDER BY time_start DESC, status = 2, result_id DESC LIMIT 1`, // at a tie, a non-success decides
		nas, taskID, JobBackup, since, since).Scan(&r.ResultID, &r.Status, &r.Start, &r.End, &r.Errors, &r.Warnings, &r.Hosts, &r.FirstError)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	return r, err == nil, err
}

// --- abb stale ---

// StaleDevice is a protected machine without a recent good backup.
type StaleDevice struct {
	NAS         string `json:"nas"`
	Client      string `json:"client,omitempty"`
	HostName    string `json:"host_name,omitempty"`
	DeviceID    int64  `json:"device_id,omitempty"`
	TaskName    string `json:"task_name,omitempty"`
	OSName      string `json:"os_name,omitempty"`
	AgentStatus string `json:"agent_status,omitempty"`
	LastSuccess int64  `json:"last_success,omitempty"` // 0 = never succeeded in the mirror
	AgeHours    int64  `json:"age_hours,omitempty"`
	Running     bool   `json:"running,omitempty"` // its task is backing up now; the age still counts
	Kind        string `json:"kind"`              // stale, never_succeeded, nas_not_synced, abb_missing, abb_not_installed
	Note        string `json:"note,omitempty"`
}

// Stale lists devices whose last successful (or partially successful, which
// still produced a restore point) backup is older than the threshold. A
// running task does not hide a device: the age decides, and running is shown.
func Stale(ctx context.Context, db Querier, nasList []NAS, olderThan time.Duration, now time.Time, maxSyncAge time.Duration) ([]StaleDevice, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]StaleDevice, 0)
	cutoff := now.Add(-olderThan).Unix()
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, StaleDevice{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
			continue
		}
		if kind, note, gone := st.ABBGone(); gone {
			out = append(out, StaleDevice{NAS: n.Name, Client: n.Client, Kind: kind, Note: note})
			continue
		}
		devs, err := deviceLastSuccess(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		for _, d := range devs {
			if d.LastSuccess > 0 && d.LastSuccess >= cutoff {
				continue
			}
			d.Running = d.running
			d.NAS, d.Client = n.Name, n.Client
			if d.LastSuccess == 0 {
				d.Kind = "never_succeeded"
			} else {
				d.Kind = "stale"
				d.AgeHours = int64(now.Sub(time.Unix(d.LastSuccess, 0)).Hours())
			}
			out = append(out, d.StaleDevice)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastSuccess < out[j].LastSuccess })
	return out, nil
}

type devRow struct {
	StaleDevice
	running bool
}

// lastSuccessSQL is the one rule for a device's last good backup (alias d =
// abb_devices; shared by abb stale and abb device). A device with no
// per-device runs falls back to a fully successful task result whose
// task_config named its device id; a host name never decides, since two
// machines (or a machine and its replacement) can share one.
const lastSuccessSQL = `MAX(
  COALESCE((SELECT MAX(r.time_end) FROM abb_device_runs r WHERE r.nas = d.nas AND r.device_id = d.device_id AND r.status IN (2,3) AND r.time_end > 0
     AND NOT EXISTS (SELECT 1 FROM abb_results p WHERE p.nas = r.nas AND p.result_id = r.result_id AND p.job_action <> 1)), 0),
  CASE WHEN EXISTS (SELECT 1 FROM abb_device_runs r WHERE r.nas = d.nas AND r.device_id = d.device_id) THEN 0
  ELSE COALESCE((SELECT MAX(x.time_end) FROM abb_results x WHERE x.nas = d.nas AND x.task_id = d.task_id AND x.job_action = 1
     AND x.status = 2 AND x.time_end > 0 AND instr(COALESCE(x.device_ids,''), ',' || d.device_id || ',') > 0), 0) END)`

// deviceLastSuccess joins each protected device to its newest good per-device
// run (success or partial: both left a restore point). Only a device with no
// per-device runs at all falls back to task-level results that name its host,
// and then only full successes count: a task-level partial does not say which
// device failed.
func deviceLastSuccess(ctx context.Context, db Querier, nas string) ([]devRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT d.device_id, COALESCE(d.host_name,''), COALESCE(t.task_name,''), COALESCE(d.os_name,''), COALESCE(d.agent_status,''),
  COALESCE(t.running,0), `+lastSuccessSQL+`
FROM abb_devices d LEFT JOIN abb_tasks t ON t.nas = d.nas AND t.task_id = d.task_id
WHERE d.nas = ? ORDER BY d.host_name`, nas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []devRow
	for rows.Next() {
		var d devRow
		var running int
		if err := rows.Scan(&d.DeviceID, &d.HostName, &d.TaskName, &d.OSName, &d.AgentStatus, &running, &d.LastSuccess); err != nil {
			return nil, err
		}
		d.running = running == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

// --- fleet health ---

// HealthIssue is one storage problem on one NAS.
type HealthIssue struct {
	NAS        string   `json:"nas"`
	Client     string   `json:"client,omitempty"`
	Kind       string   `json:"kind"` // volume, pool, disk, nas_not_synced
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name,omitempty"`
	Severity   Severity `json:"severity"`
	Problem    string   `json:"problem"`
	UsedPct    float64  `json:"used_pct,omitempty"`
	TempC      int64    `json:"temp_c,omitempty"`
	DaysToFull *float64 `json:"days_to_full,omitempty"`
	TakenAt    int64    `json:"taken_at,omitempty"`
}

// Health checks each NAS's newest storage snapshot against the thresholds.
func Health(ctx context.Context, db Querier, nasList []NAS, fullPct float64, diskTemp int64, now time.Time, maxSyncAge time.Duration) ([]HealthIssue, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]HealthIssue, 0)
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, HealthIssue{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Severity: SevUnknown, Problem: syncNote(st, ok, n)})
			continue
		}
		items, takenAt, err := latestStorage(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		volumes := 0
		for _, it := range items {
			if it.Kind == "volume" {
				volumes++
			}
		}
		if takenAt != st.SucceededAt || volumes == 0 {
			// The last good sync left no storage snapshot (or one without
			// volumes): an older snapshot is not today's answer.
			out = append(out, HealthIssue{NAS: n.Name, Client: n.Client, Kind: "storage_snapshot", Severity: SevUnknown, TakenAt: takenAt,
				Problem: "no current storage snapshot from the last successful sync"})
			continue
		}
		for _, it := range items {
			base := HealthIssue{NAS: n.Name, Client: n.Client, Kind: it.Kind, ID: it.ID, Name: it.Name, TakenAt: takenAt}
			if sev := StorageSeverity(it.Status); sev != SevOK {
				iss := base
				iss.Severity, iss.Problem = sev, "status "+it.Status
				if it.Status == "" {
					iss.Problem = "status not reported"
				}
				out = append(out, iss)
			}
			if it.Kind == "disk" {
				if sev := StorageSeverity(it.SmartStatus); sev != SevOK {
					iss := base
					iss.Severity, iss.Problem = sev, "SMART "+it.SmartStatus
					if it.SmartStatus == "" {
						iss.Severity, iss.Problem = SevWarning, "SMART status not reported"
					}
					out = append(out, iss)
				}
				if diskTemp > 0 && it.Temp <= 0 { // older mirrors stored a missing reading as 0
					iss := base
					iss.Severity, iss.Problem = SevWarning, "temperature not reported (--disk-temp 0 skips the check)"
					out = append(out, iss)
				}
				if diskTemp > 0 && it.Temp >= diskTemp {
					iss := base
					iss.Severity, iss.Problem, iss.TempC = SevWarning, fmt.Sprintf("temperature %dC >= %dC", it.Temp, diskTemp), it.Temp
					out = append(out, iss)
				}
				for _, flag := range []string{"below_remain_life_thr", "exceed_bad_sector_thr", "remain_life_danger"} {
					if strings.Contains(it.Detail, flag) {
						iss := base
						iss.Severity, iss.Problem = SevWarning, flag
						out = append(out, iss)
					}
				}
			}
			if it.Kind == "volume" && it.SizeTotal <= 0 {
				iss := base
				iss.Severity, iss.Problem = SevWarning, "capacity not reported"
				out = append(out, iss)
			}
			if it.Kind == "volume" && it.SizeTotal > 0 {
				pct := 100 * float64(it.SizeUsed) / float64(it.SizeTotal)
				if fullPct > 0 && pct >= fullPct {
					iss := base
					iss.Severity, iss.Problem, iss.UsedPct = SevWarning, fmt.Sprintf("%.1f%% used >= %.0f%%", pct, fullPct), round1(pct)
					if d, ok, err := daysToFull(ctx, db, n.Name, it.ID, it, takenAt); err != nil {
						return nil, err
					} else if ok {
						iss.DaysToFull = &d
					}
					out = append(out, iss)
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity > out[j].Severity })
	return out, nil
}

func latestStorage(ctx context.Context, db Querier, nas string) ([]StorageItem, int64, error) {
	var takenAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(taken_at) FROM storage_snapshots WHERE nas = ?`, nas).Scan(&takenAt); err != nil {
		return nil, 0, err
	}
	if !takenAt.Valid {
		return nil, 0, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT kind, item_id, COALESCE(name,''), COALESCE(status,''), COALESCE(smart_status,''),
  COALESCE(temp,0), COALESCE(size_total,0), COALESCE(size_used,0), COALESCE(detail,'')
  FROM storage_snapshots WHERE nas = ? AND taken_at = ? ORDER BY kind, item_id`, nas, takenAt.Int64)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []StorageItem
	for rows.Next() {
		var it StorageItem
		if err := rows.Scan(&it.Kind, &it.ID, &it.Name, &it.Status, &it.SmartStatus, &it.Temp, &it.SizeTotal, &it.SizeUsed, &it.Detail); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, takenAt.Int64, rows.Err()
}

// daysToFull projects the used-bytes slope between the oldest snapshot in the
// last 30 days and the newest one. It needs at least a day between them and a
// growing volume.
func daysToFull(ctx context.Context, db Querier, nas, volID string, cur StorageItem, takenAt int64) (float64, bool, error) {
	var oldAt, oldUsed sql.NullInt64
	// Only snapshots with known capacity (size_used >= 0) are measurements.
	err := db.QueryRowContext(ctx, `SELECT taken_at, size_used FROM storage_snapshots WHERE nas = ? AND kind = 'volume' AND item_id = ?
  AND taken_at >= ? AND size_used >= 0 AND size_total > 0 ORDER BY taken_at ASC LIMIT 1`, nas, volID, takenAt-30*24*3600).Scan(&oldAt, &oldUsed)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	dt := float64(takenAt - oldAt.Int64)
	grow := float64(cur.SizeUsed - oldUsed.Int64)
	if dt < 24*3600 || grow <= 0 {
		return 0, false, nil
	}
	perDay := grow / (dt / 86400)
	return round1(float64(cur.SizeTotal-cur.SizeUsed) / perDay), true, nil
}

// --- fleet status ---

// NASStatus is one row of the fleet rollup.
type NASStatus struct {
	NAS          string   `json:"nas"`
	Client       string   `json:"client,omitempty"`
	State        Severity `json:"state"`
	Model        string   `json:"model,omitempty"`
	DSMVersion   string   `json:"dsm_version,omitempty"`
	LastSyncAt   int64    `json:"last_sync_at,omitempty"`
	SyncError    string   `json:"sync_error,omitempty"`
	Tasks        int      `json:"tasks"`
	TasksOK      int      `json:"tasks_ok"`
	TasksPartial int      `json:"tasks_partial"`
	TasksFailed  int      `json:"tasks_failed"`
	TasksNoRun   int      `json:"tasks_no_run"`
	TasksRunning int      `json:"tasks_running"`
	TasksStuck   int      `json:"tasks_running_too_long"`
	TasksOff     int      `json:"tasks_unscheduled"`
	StaleDevices int      `json:"stale_devices"`
	Storage      Severity `json:"storage"`
	ABBInstalled *bool    `json:"abb_installed,omitempty"`
	Note         string   `json:"note,omitempty"`
}

// Status builds the one-row-per-NAS rollup from the other reports.
func Status(ctx context.Context, db Querier, nasList []NAS, since time.Time, staleAfter time.Duration, fullPct float64, diskTemp int64, now time.Time, maxSyncAge time.Duration) ([]NASStatus, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	failures, err := Failures(ctx, db, nasList, since, now, maxSyncAge)
	if err != nil {
		return nil, err
	}
	stale, err := Stale(ctx, db, nasList, staleAfter, now, maxSyncAge)
	if err != nil {
		return nil, err
	}
	health, err := Health(ctx, db, nasList, fullPct, diskTemp, now, maxSyncAge)
	if err != nil {
		return nil, err
	}
	out := make([]NASStatus, 0, len(nasList))
	for _, n := range nasList {
		st, ok := states[n.Name]
		row := NASStatus{NAS: n.Name, Client: n.Client, Model: st.Model, DSMVersion: st.DSMVersion, LastSyncAt: st.SucceededAt, SyncError: st.Error}
		if st.ABBInstalled.Valid {
			v := st.ABBInstalled.Bool
			row.ABBInstalled = &v
		}
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			row.State, row.Storage = SevUnknown, SevUnknown
			out = append(out, row)
			continue
		}
		tasks, err := loadTasks(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		row.Tasks = len(tasks)
		flagged := map[int64]Failure{}
		for _, f := range failures {
			if f.NAS == n.Name && f.TaskID != 0 {
				flagged[f.TaskID] = f
			}
		}
		// Each task lands in exactly one bucket, so the counts add up to Tasks.
		for _, t := range tasks {
			f, bad := flagged[t.TaskID]
			switch {
			case bad && (f.Kind == "running_too_long" || f.Kind == "running_start_unknown"):
				row.TasksStuck++
			case bad && f.Kind == "no_run_in_window":
				row.TasksNoRun++
			case bad && (f.Status == "partial" || f.Status == "canceled"):
				row.TasksPartial++
			case bad:
				row.TasksFailed++
			case t.Running:
				row.TasksRunning++
			case !t.Scheduled:
				row.TasksOff++
			default:
				row.TasksOK++
			}
		}
		for _, s := range stale {
			if s.NAS == n.Name && (s.Kind == "stale" || s.Kind == "never_succeeded") {
				row.StaleDevices++
			}
		}
		for _, h := range health {
			if h.NAS == n.Name && h.Severity > row.Storage {
				row.Storage = h.Severity
			}
		}
		row.State = row.Storage
		if row.TasksFailed > 0 {
			row.State = maxSev(row.State, SevCritical)
		}
		if row.TasksPartial+row.TasksNoRun+row.TasksStuck+row.StaleDevices > 0 {
			row.State = maxSev(row.State, SevWarning)
		}
		if kind, note, gone := st.ABBGone(); gone {
			row.Note = note
			if kind == "abb_missing" {
				row.State = maxSev(row.State, SevCritical)
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func maxSev(a, b Severity) Severity {
	if b > a {
		return b
	}
	return a
}

// --- abb device ---

// DeviceMatch is one protected machine found by host name, or a
// nas_not_synced row for a NAS whose mirror cannot answer.
type DeviceMatch struct {
	NAS          string      `json:"nas"`
	Client       string      `json:"client,omitempty"`
	Kind         string      `json:"kind"` // device, nas_not_synced, abb_missing, abb_not_installed
	Note         string      `json:"note,omitempty"`
	HostName     string      `json:"host_name,omitempty"`
	DeviceID     int64       `json:"device_id,omitempty"`
	TaskID       int64       `json:"task_id,omitempty"`
	TaskName     string      `json:"task_name,omitempty"`
	OSName       string      `json:"os_name,omitempty"`
	AgentStatus  string      `json:"agent_status,omitempty"`
	LastSuccess  int64       `json:"last_success,omitempty"`
	RecentRuns   []DeviceRun `json:"recent_runs"`
	LastAttempt  string      `json:"last_attempt_status,omitempty"`
	LastAttemptT int64       `json:"last_attempt_at,omitempty"`
}

// FindDevice returns protected devices on the given NAS units whose host name
// contains query (case-insensitive), with each one's last success and recent
// runs. A NAS whose mirror is not fresh gets a nas_not_synced row: "not found"
// from a stale mirror is not an answer.
func FindDevice(ctx context.Context, db Querier, nasList []NAS, query string, runs int, now time.Time, maxSyncAge time.Duration) ([]DeviceMatch, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	clients := map[string]string{}
	var unsynced []DeviceMatch
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			unsynced = append(unsynced, DeviceMatch{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n), RecentRuns: make([]DeviceRun, 0)})
		} else if kind, note, gone := st.ABBGone(); gone {
			unsynced = append(unsynced, DeviceMatch{NAS: n.Name, Client: n.Client, Kind: kind, Note: note, RecentRuns: make([]DeviceRun, 0)})
		} else {
			clients[n.Name] = n.Client
		}
	}
	// instr, not LIKE: '%' and '_' in a host name are literal.
	rows, err := db.QueryContext(ctx, `SELECT d.nas, d.host_name, d.device_id, d.task_id, COALESCE(t.task_name,''), COALESCE(d.os_name,''),
  COALESCE(d.agent_status,''), `+lastSuccessSQL+` FROM abb_devices d LEFT JOIN abb_tasks t ON t.nas = d.nas AND t.task_id = d.task_id
  WHERE instr(lower(d.host_name), lower(?)) > 0 ORDER BY d.nas, d.host_name`, query)
	if err != nil {
		return nil, err
	}
	var out []DeviceMatch
	for rows.Next() {
		var m DeviceMatch
		if err := rows.Scan(&m.NAS, &m.HostName, &m.DeviceID, &m.TaskID, &m.TaskName, &m.OSName, &m.AgentStatus, &m.LastSuccess); err != nil {
			_ = rows.Close()
			return nil, err
		}
		client, fresh := clients[m.NAS]
		if !fresh {
			continue // unregistered or not fresh: reported as nas_not_synced instead
		}
		m.Client, m.Kind = client, "device"
		out = append(out, m)
	}
	// rows.Err, not Close, reports an iteration that stopped early.
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Drain-first: follow-up queries run after the parent cursor is closed.
	for i := range out {
		m := &out[i]
		rr, err := db.QueryContext(ctx, `SELECT device_result_id, result_id, device_id, COALESCE(device_name,''), status, time_start, time_end,
  COALESCE(transfered_bytes,0) FROM abb_device_runs WHERE nas = ? AND device_id = ? ORDER BY time_start DESC LIMIT ?`, m.NAS, m.DeviceID, runs)
		if err != nil {
			return nil, err
		}
		m.RecentRuns = make([]DeviceRun, 0)
		for rr.Next() {
			var r DeviceRun
			if err := rr.Scan(&r.DeviceResultID, &r.ResultID, &r.DeviceID, &r.DeviceName, &r.Status, &r.Start, &r.End, &r.Bytes); err != nil {
				_ = rr.Close()
				return nil, err
			}
			m.RecentRuns = append(m.RecentRuns, r)
		}
		if err := rr.Err(); err != nil {
			_ = rr.Close()
			return nil, err
		}
		if err := rr.Close(); err != nil {
			return nil, err
		}
		if len(m.RecentRuns) > 0 {
			m.LastAttempt, m.LastAttemptT = StatusLabel(int(m.RecentRuns[0].Status)), m.RecentRuns[0].Start
		}
	}
	if out == nil {
		out = make([]DeviceMatch, 0)
	}
	return append(out, unsynced...), nil
}

// --- abb trend ---

// TrendRow summarizes backup runs for one task or device over a window, or is
// a nas_not_synced row for a NAS whose history cannot be trusted.
type TrendRow struct {
	NAS            string  `json:"nas"`
	Kind           string  `json:"kind"` // task, device, nas_not_synced, abb_missing, abb_not_installed, partial_history
	Note           string  `json:"note,omitempty"`
	HistoryFrom    int64   `json:"history_from,omitempty"` // set when the window starts before the mirrored history
	Key            string  `json:"key,omitempty"`          // task name or device host name
	ID             int64   `json:"id"`
	Runs           int     `json:"runs"`
	Successes      int     `json:"successes"`
	Partials       int     `json:"partials"`
	Failures       int     `json:"failures"`
	SuccessRatePct float64 `json:"success_rate_pct"`
	MedianSec      int64   `json:"median_duration_sec"`
	P90Sec         int64   `json:"p90_duration_sec"`
	LastSec        *int64  `json:"last_duration_sec"` // null: the newest completed run has no end time
	Overrun        bool    `json:"overrun"`
}

// Trend aggregates backup runs since a time, per task (byDevice=false, from
// task-level results) or per device (from per-device runs). A run overruns
// when the newest completed run took more than overrun x the median of at
// least 3 runs.
func Trend(ctx context.Context, db Querier, nasList []NAS, since time.Time, byDevice bool, overrun float64, now time.Time, maxSyncAge time.Duration) ([]TrendRow, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	fresh := map[string]bool{}
	partial := map[string]int64{} // NAS -> history_from, when the window starts before it
	var unsynced []TrendRow
	for _, n := range nasList {
		st, ok := states[n.Name]
		switch kind, note, gone := st.ABBGone(); {
		case !ok || !st.FreshFor(n, now, maxSyncAge):
			unsynced = append(unsynced, TrendRow{NAS: n.Name, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
		case gone:
			unsynced = append(unsynced, TrendRow{NAS: n.Name, Kind: kind, Note: note})
		default:
			fresh[n.Name] = true
			if st.HistoryFrom == 0 || since.Unix() < st.HistoryFrom {
				partial[n.Name] = st.HistoryFrom
			}
		}
	}
	kind := "task"
	if byDevice {
		kind = "device"
	}
	q := `SELECT nas, COALESCE(task_name,''), task_id, status, time_start, time_end FROM abb_results
  WHERE job_action = 1 AND time_start >= ? ORDER BY nas, task_id, time_start`
	if byDevice {
		q = `SELECT nas, COALESCE(device_name,''), device_id, status, time_start, time_end FROM abb_device_runs
  WHERE time_start >= ? ORDER BY nas, device_id, time_start`
	}
	rows, err := db.QueryContext(ctx, q, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type acc struct {
		row     TrendRow
		durs    []int64
		lastDur int64 // newest completed run's duration; -1 = no end time
	}
	groups := map[string]*acc{}
	var order []string
	for rows.Next() {
		var nas, key string
		var id, status, start, end int64
		if err := rows.Scan(&nas, &key, &id, &status, &start, &end); err != nil {
			return nil, err
		}
		if !fresh[nas] {
			continue
		}
		if status == 0 || status == StatusRunning {
			continue // not finished: neither a success nor a failure yet
		}
		k := fmt.Sprintf("%s\x00%d", nas, id)
		g, ok := groups[k]
		if !ok {
			g = &acc{row: TrendRow{NAS: nas, Kind: kind, Key: key, ID: id}}
			if from, ok := partial[nas]; ok {
				g.row.HistoryFrom, g.row.Note = from, partialNote(nas, from)
			}
			groups[k] = g
			order = append(order, k)
		}
		g.row.Runs++
		switch status {
		case StatusSuccess:
			g.row.Successes++
		case StatusPartial:
			g.row.Partials++
		default:
			g.row.Failures++
		}
		g.lastDur = -1
		if end > 0 && start > 0 && end >= start { // a run can finish in the second it started
			g.durs = append(g.durs, end-start)
			g.lastDur = end - start
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]TrendRow, 0, len(order))
	for _, k := range order {
		g := groups[k]
		r := g.row
		if r.Runs > 0 {
			r.SuccessRatePct = round1(100 * float64(r.Successes) / float64(r.Runs))
		}
		if n := len(g.durs); n > 0 {
			// LastSec and Overrun describe the newest completed run only; when
			// it has no end time they stay 0/false rather than borrow an older one.
			if g.lastDur >= 0 {
				r.LastSec = &g.lastDur
			} else {
				r.Note = strings.TrimSpace(r.Note + " The newest run has no end time: its duration and overrun are unknown.")
			}
			sorted := append([]int64(nil), g.durs...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			r.MedianSec = sorted[n/2]
			if n%2 == 0 {
				r.MedianSec = (sorted[n/2-1] + sorted[n/2]) / 2
			}
			r.P90Sec = sorted[int(math.Ceil(0.9*float64(n)))-1]
			r.Overrun = g.lastDur >= 0 && n >= 3 && overrun > 0 && float64(g.lastDur) > overrun*float64(r.MedianSec)
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Overrun != out[j].Overrun {
			return out[i].Overrun
		}
		return out[i].SuccessRatePct < out[j].SuccessRatePct
	})
	// A NAS with partial history but no runs in the window still says so.
	for _, n := range nasList {
		from, ok := partial[n.Name]
		if !ok {
			continue
		}
		listed := false
		for _, r := range out {
			listed = listed || r.NAS == n.Name
		}
		if !listed {
			out = append(out, TrendRow{NAS: n.Name, Kind: "partial_history", HistoryFrom: from, Note: partialNote(n.Name, from)})
		}
	}
	return append(out, unsynced...), nil
}

func partialNote(nas string, from int64) string {
	since := "no history is mirrored yet"
	if from > 0 {
		since = "history is mirrored only since " + time.Unix(from, 0).UTC().Format(time.RFC3339)
	}
	return since + "; runs before that are missing from these numbers. Backfill with 'synology-cli fleet sync --nas " + nas + " --full --since <window>'"
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
