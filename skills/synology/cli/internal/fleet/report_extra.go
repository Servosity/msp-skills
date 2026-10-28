// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Reports added by the 4.34.2 reprint: abb unprotected,
// fleet capacity and abb change-rate. Each reads only the fleet.db mirror and
// follows report.go's rules: a NAS whose mirror is not fresh is one
// nas_not_synced row, never an empty answer, and absence is never zero.

package fleet

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// goodRunBackupSQL keeps a device run whose parent result is a backup, or
// whose parent is not mirrored (the same rule as lastSuccessSQL); alias r.
const goodRunBackupSQL = `NOT EXISTS (SELECT 1 FROM abb_results p WHERE p.nas = r.nas AND p.result_id = r.result_id AND p.job_action <> 1)`

// --- abb unprotected ---

// UnprotectedRow is a device or task that dropped out of protection, or a
// row about a whole NAS whose mirror cannot answer.
type UnprotectedRow struct {
	NAS      string `json:"nas"`
	Client   string `json:"client,omitempty"`
	Kind     string `json:"kind"` // device_dropped, task_deleted, task_unscheduled, nas_not_synced, abb_missing, abb_not_installed, partial_history
	DeviceID int64  `json:"device_id,omitempty"`
	HostName string `json:"host_name,omitempty"`
	TaskID   int64  `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`
	// Devices is how many devices an unscheduled task still holds.
	Devices *int `json:"devices,omitempty"`
	// Absence is never zero: a device or task with no run in the window, or
	// no good run ever, has null here, never 0 (1970).
	LastRunAt   *int64 `json:"last_run_at"`
	LastSuccess *int64 `json:"last_success"`
	HistoryFrom int64  `json:"history_from,omitempty"`
	Note        string `json:"note,omitempty"`
}

// ReportIdentity is what --quiet prints for the row.
func (u UnprotectedRow) ReportIdentity() string { return firstNonEmpty(u.HostName, u.TaskName, u.NAS) }

// IsFinding reports whether the row needs attention (exit 6).
func (u UnprotectedRow) IsFinding() bool {
	return u.Kind != "abb_not_installed" && u.Kind != "partial_history"
}

func knownTime(v int64) *int64 {
	if v > 0 {
		return &v
	}
	return nil
}

// Unprotected lists, per fresh NAS: devices with a backup run since `since`
// that no current task protects (device_dropped), tasks with a backup result
// since `since` that no longer exist (task_deleted), and current tasks whose
// schedule is off while they still hold devices (task_unscheduled). The
// current task and device lists are rewritten on every sync; only the
// retained run history remembers what used to be protected.
func Unprotected(ctx context.Context, db Querier, nasList []NAS, since time.Time, now time.Time, maxSyncAge time.Duration) ([]UnprotectedRow, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]UnprotectedRow, 0)
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, UnprotectedRow{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
			continue
		}
		if kind, note, gone := st.ABBGone(); gone {
			out = append(out, UnprotectedRow{NAS: n.Name, Client: n.Client, Kind: kind, Note: note})
			continue
		}
		var rows []UnprotectedRow
		dropped, err := droppedDevices(ctx, db, n.Name, since.Unix())
		if err != nil {
			return nil, err
		}
		rows = append(rows, dropped...)
		deleted, err := deletedTasks(ctx, db, n.Name, since.Unix())
		if err != nil {
			return nil, err
		}
		rows = append(rows, deleted...)
		off, err := unscheduledTasks(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		rows = append(rows, off...)
		partial := st.HistoryFrom == 0 || since.Unix() < st.HistoryFrom
		for i := range rows {
			rows[i].NAS, rows[i].Client = n.Name, n.Client
			if partial {
				rows[i].HistoryFrom = st.HistoryFrom
				rows[i].Note = strings.TrimSpace(rows[i].Note + " " + partialNote(n.Name, st.HistoryFrom))
			}
		}
		if partial && len(rows) == 0 {
			// No row is not "nothing dropped" when the window starts before
			// the mirrored history.
			rows = append(rows, UnprotectedRow{NAS: n.Name, Client: n.Client, Kind: "partial_history", HistoryFrom: st.HistoryFrom, Note: partialNote(n.Name, st.HistoryFrom)})
		}
		out = append(out, rows...)
	}
	return out, nil
}

// droppedDevices are devices with a backup run in the window and no
// abb_devices row on the NAS (any task). A device now protected under another
// device id with the same host name (a re-registered agent) is named in the
// note, never merged: two machines can share a host name.
func droppedDevices(ctx context.Context, db Querier, nas string, since int64) ([]UnprotectedRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT r.device_id, COALESCE(MAX(r.device_name),''),
  MAX(CASE WHEN r.time_start > 0 THEN r.time_start ELSE 0 END),
  COALESCE(MAX(CASE WHEN r.status IN (2,3) THEN CASE WHEN r.time_end > 0 THEN r.time_end ELSE r.time_start END END), 0)
FROM abb_device_runs r
WHERE r.nas = ? AND `+goodRunBackupSQL+`
  AND NOT EXISTS (SELECT 1 FROM abb_devices d WHERE d.nas = r.nas AND d.device_id = r.device_id)
GROUP BY r.device_id
HAVING MAX(CASE WHEN r.time_start > 0 THEN r.time_start ELSE r.time_end END) >= ?
ORDER BY 2, 1`, nas, since)
	if err != nil {
		return nil, err
	}
	var out []UnprotectedRow
	for rows.Next() {
		var id, lastRun, lastOK int64
		var host string
		if err := rows.Scan(&id, &host, &lastRun, &lastOK); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, UnprotectedRow{Kind: "device_dropped", DeviceID: id, HostName: host, LastRunAt: knownTime(lastRun), LastSuccess: knownTime(lastOK),
			Note: "backed up in the window but no current Active Backup task protects this device"})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Drain-first: the hint lookups run after the parent cursor is closed.
	for i := range out {
		if out[i].HostName == "" {
			continue
		}
		var ids sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT group_concat(device_id) FROM abb_devices WHERE nas = ? AND lower(host_name) = lower(?)`,
			nas, out[i].HostName).Scan(&ids); err != nil {
			return nil, err
		}
		if ids.Valid && ids.String != "" {
			out[i].Note += fmt.Sprintf("; a device with the same host name is protected now as device_id %s (a re-registered agent, or another machine with this name: check before closing)", ids.String)
		}
	}
	return out, nil
}

// deletedTasks are task ids with a backup result in the window and no
// abb_tasks row on the NAS.
func deletedTasks(ctx context.Context, db Querier, nas string, since int64) ([]UnprotectedRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT x.task_id, COALESCE(MAX(x.task_name),''),
  MAX(CASE WHEN x.time_start > 0 THEN x.time_start ELSE 0 END),
  COALESCE(MAX(CASE WHEN x.status IN (2,3) THEN CASE WHEN x.time_end > 0 THEN x.time_end ELSE x.time_start END END), 0)
FROM abb_results x
WHERE x.nas = ? AND x.job_action = 1
  AND NOT EXISTS (SELECT 1 FROM abb_tasks t WHERE t.nas = x.nas AND t.task_id = x.task_id)
GROUP BY x.task_id
HAVING MAX(CASE WHEN x.time_start > 0 THEN x.time_start ELSE x.time_end END) >= ?
ORDER BY 2, 1`, nas, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnprotectedRow
	for rows.Next() {
		var id, lastRun, lastOK int64
		var name string
		if err := rows.Scan(&id, &name, &lastRun, &lastOK); err != nil {
			return nil, err
		}
		out = append(out, UnprotectedRow{Kind: "task_deleted", TaskID: id, TaskName: name, LastRunAt: knownTime(lastRun), LastSuccess: knownTime(lastOK),
			Note: "this task backed up in the window and no longer exists on the NAS; the devices it protected may have no backup"})
	}
	return out, rows.Err()
}

// unscheduledTasks are current tasks with no schedule that still hold devices.
func unscheduledTasks(ctx context.Context, db Querier, nas string) ([]UnprotectedRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT t.task_id, COALESCE(t.task_name,''), COUNT(d.device_id),
  COALESCE((SELECT MAX(CASE WHEN x.time_start > 0 THEN x.time_start ELSE 0 END) FROM abb_results x WHERE x.nas = t.nas AND x.task_id = t.task_id AND x.job_action = 1), 0),
  COALESCE((SELECT MAX(CASE WHEN x.time_end > 0 THEN x.time_end ELSE x.time_start END) FROM abb_results x WHERE x.nas = t.nas AND x.task_id = t.task_id AND x.job_action = 1 AND x.status IN (2,3)), 0)
FROM abb_tasks t JOIN abb_devices d ON d.nas = t.nas AND d.task_id = t.task_id
WHERE t.nas = ? AND COALESCE(t.scheduled,0) = 0 AND COALESCE(t.running,0) = 0
GROUP BY t.task_id, t.task_name
ORDER BY 2, 1`, nas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnprotectedRow
	for rows.Next() {
		var id, lastRun, lastOK int64
		var name string
		var devices int
		if err := rows.Scan(&id, &name, &devices, &lastRun, &lastOK); err != nil {
			return nil, err
		}
		d := devices
		out = append(out, UnprotectedRow{Kind: "task_unscheduled", TaskID: id, TaskName: name, Devices: &d, LastRunAt: knownTime(lastRun), LastSuccess: knownTime(lastOK),
			Note: "this task holds devices but has no schedule (off or paused); they are backed up only when someone runs it"})
	}
	return out, rows.Err()
}

// --- fleet capacity ---

// CapacityRow is one volume's fill level and runway, or a row about a whole
// NAS whose storage cannot be read.
type CapacityRow struct {
	NAS    string `json:"nas"`
	Client string `json:"client,omitempty"`
	Kind   string `json:"kind"` // volume, nas_not_synced, storage_snapshot
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	// Absence is never zero: an unreported capacity, a growth rate that needs
	// more history, and the runway of a flat or shrinking volume are null.
	UsedPct         *float64 `json:"used_pct"`
	SizeTotal       *int64   `json:"size_total_bytes"`
	SizeUsed        *int64   `json:"size_used_bytes"`
	GrowthBytesDay  *float64 `json:"growth_bytes_per_day"`
	DaysToFull      *float64 `json:"days_to_full"`
	WithinHorizon   bool     `json:"within_horizon"`
	CapacityUnknown bool     `json:"capacity_unknown,omitempty"`
	TakenAt         int64    `json:"taken_at,omitempty"`
	Note            string   `json:"note,omitempty"`
}

// ReportIdentity is what --quiet prints for the row.
func (c CapacityRow) ReportIdentity() string { return firstNonEmpty(c.Name, c.ID, c.NAS) }

// IsFinding: a volume inside the horizon, one whose capacity is unknown, or
// a NAS whose storage cannot be read (unknown is never OK).
func (c CapacityRow) IsFinding() bool {
	return c.WithinHorizon || c.CapacityUnknown || c.Kind != "volume"
}

// Capacity lists every volume on every fresh NAS with its used percentage,
// growth per day and days to full (the slope fleet health uses: the oldest
// snapshot with known capacity in the last 30 days against the newest),
// soonest first. The newest snapshot must be the one the last good sync took.
func Capacity(ctx context.Context, db Querier, nasList []NAS, horizon time.Duration, now time.Time, maxSyncAge time.Duration) ([]CapacityRow, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]CapacityRow, 0)
	horizonDays := horizon.Hours() / 24
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, CapacityRow{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
			continue
		}
		items, takenAt, err := latestStorage(ctx, db, n.Name)
		if err != nil {
			return nil, err
		}
		var vols []StorageItem
		for _, it := range items {
			if it.Kind == "volume" {
				vols = append(vols, it)
			}
		}
		if takenAt != st.SucceededAt || len(vols) == 0 {
			out = append(out, CapacityRow{NAS: n.Name, Client: n.Client, Kind: "storage_snapshot", TakenAt: takenAt,
				Note: "no current storage snapshot with volumes from the last successful sync"})
			continue
		}
		for _, it := range vols {
			row := CapacityRow{NAS: n.Name, Client: n.Client, Kind: "volume", ID: it.ID, Name: it.Name, TakenAt: takenAt}
			if it.SizeTotal <= 0 || it.SizeUsed < 0 {
				row.CapacityUnknown, row.Note = true, "capacity not reported; the runway cannot be judged"
				out = append(out, row)
				continue
			}
			total, used := it.SizeTotal, it.SizeUsed
			pct := round1(100 * float64(used) / float64(total))
			row.SizeTotal, row.SizeUsed, row.UsedPct = &total, &used, &pct
			perDay, measured, err := volumeGrowthPerDay(ctx, db, n.Name, it.ID, it, takenAt)
			if err != nil {
				return nil, err
			}
			switch {
			case !measured:
				row.Note = "needs storage snapshots at least a day apart in the last 30 days to measure growth"
			case perDay <= 0:
				g := round1(perDay)
				row.GrowthBytesDay, row.Note = &g, "flat or shrinking: no fill date"
			default:
				g := round1(perDay)
				d := round1(float64(total-used) / perDay)
				row.GrowthBytesDay, row.DaysToFull = &g, &d
				row.WithinHorizon = horizonDays > 0 && d <= horizonDays
			}
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].DaysToFull, out[j].DaysToFull
		switch {
		case a != nil && b != nil:
			return *a < *b
		case a != nil || b != nil:
			return a != nil
		}
		if pa, pb := out[i].UsedPct, out[j].UsedPct; pa != nil && pb != nil {
			return *pa > *pb
		}
		return false
	})
	return out, nil
}

// volumeGrowthPerDay is the used-bytes slope between the oldest snapshot with
// known capacity in the last 30 days and the newest one (cur, at takenAt).
// measured is false with under a day between them.
func volumeGrowthPerDay(ctx context.Context, db Querier, nas, volID string, cur StorageItem, takenAt int64) (float64, bool, error) {
	var oldAt, oldUsed sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT taken_at, size_used FROM storage_snapshots WHERE nas = ? AND kind = 'volume' AND item_id = ?
  AND taken_at >= ? AND size_used >= 0 AND size_total > 0 ORDER BY taken_at ASC LIMIT 1`, nas, volID, takenAt-30*24*3600).Scan(&oldAt, &oldUsed)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	dt := float64(takenAt - oldAt.Int64)
	if dt < 24*3600 {
		return 0, false, nil
	}
	return float64(cur.SizeUsed-oldUsed.Int64) / (dt / 86400), true, nil
}

// --- abb change-rate ---

// ChangeRateRow is a device whose newest good backup moved far more data
// than its own median (spike) or nothing for several runs (flat), or a row
// about a whole NAS.
type ChangeRateRow struct {
	NAS      string `json:"nas"`
	Client   string `json:"client,omitempty"`
	Kind     string `json:"kind"` // spike, flat, nas_not_synced, abb_missing, abb_not_installed
	DeviceID int64  `json:"device_id,omitempty"`
	HostName string `json:"host_name,omitempty"`
	// Absence is never zero: these are null on a row about a whole NAS.
	LatestBytes   *int64   `json:"latest_bytes"`
	LatestAt      *int64   `json:"latest_at"`
	MedianBytes   *int64   `json:"median_bytes"`
	Ratio         *float64 `json:"ratio"`
	BaselineRuns  *int     `json:"baseline_runs"`
	ZeroRuns      *int     `json:"zero_byte_runs"`
	NASSpikeCount *int     `json:"nas_spike_count"`
	Note          string   `json:"note,omitempty"`
}

// ReportIdentity is what --quiet prints for the row.
func (c ChangeRateRow) ReportIdentity() string { return firstNonEmpty(c.HostName, c.NAS) }

// IsFinding: every row except abb_not_installed needs attention.
func (c ChangeRateRow) IsFinding() bool { return c.Kind != "abb_not_installed" }

// minBaselineRuns is the fewest earlier good runs a median is taken from.
const minBaselineRuns = 5

// ChangeRate compares each device's newest good (success or partial) backup
// since `since` with the median of its earlier good runs in the window. Only
// runs whose byte count DSM reported count (-1 = unknown is skipped; 0 is a
// real value). spike: newest > factor x median, median > 0. flat: the newest
// flatRuns runs all moved 0 bytes while the median is above 0.
func ChangeRate(ctx context.Context, db Querier, nasList []NAS, since time.Time, factor float64, flatRuns int, now time.Time, maxSyncAge time.Duration) ([]ChangeRateRow, error) {
	states, err := LoadSyncStates(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]ChangeRateRow, 0)
	var fresh []NAS
	for _, n := range nasList {
		st, ok := states[n.Name]
		if !ok || !st.FreshFor(n, now, maxSyncAge) {
			out = append(out, ChangeRateRow{NAS: n.Name, Client: n.Client, Kind: "nas_not_synced", Note: syncNote(st, ok, n)})
			continue
		}
		if kind, note, gone := st.ABBGone(); gone {
			out = append(out, ChangeRateRow{NAS: n.Name, Client: n.Client, Kind: kind, Note: note})
			continue
		}
		fresh = append(fresh, n)
	}
	var flagged []ChangeRateRow
	for _, n := range fresh {
		rows, err := db.QueryContext(ctx, `SELECT r.device_id, COALESCE(r.device_name,''), r.transfered_bytes, r.time_start FROM abb_device_runs r
  WHERE r.nas = ? AND r.status IN (2,3) AND r.time_start >= ? AND r.transfered_bytes >= 0 AND `+goodRunBackupSQL+`
  ORDER BY r.device_id, r.time_start, r.device_result_id`, n.Name, since.Unix())
		if err != nil {
			return nil, err
		}
		type run struct {
			bytes, at int64
		}
		byDev := map[int64][]run{}
		names := map[int64]string{}
		var order []int64
		for rows.Next() {
			var id, b, at int64
			var name string
			if err := rows.Scan(&id, &name, &b, &at); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if _, seen := byDev[id]; !seen {
				order = append(order, id)
			}
			byDev[id] = append(byDev[id], run{b, at})
			if name != "" {
				names[id] = name
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		var nasRows []ChangeRateRow
		for _, id := range order {
			runs := byDev[id]
			latest := runs[len(runs)-1]
			base := runs[:len(runs)-1]
			if len(base) < minBaselineRuns {
				continue // no baseline: not judged, not a finding
			}
			sorted := make([]int64, len(base))
			for i, r := range base {
				sorted[i] = r.bytes
			}
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			k := len(sorted)
			median := sorted[k/2]
			if k%2 == 0 {
				median = (sorted[k/2-1] + sorted[k/2]) / 2
			}
			zeros := 0
			for i := len(runs) - 1; i >= 0 && runs[i].bytes == 0; i-- {
				zeros++
			}
			row := ChangeRateRow{NAS: n.Name, Client: n.Client, DeviceID: id, HostName: names[id]}
			lb, la, med, br, zr := latest.bytes, latest.at, median, len(base), zeros
			row.LatestBytes, row.LatestAt, row.MedianBytes, row.BaselineRuns, row.ZeroRuns = &lb, knownTime(la), &med, &br, &zr
			switch {
			case median > 0 && factor > 0 && float64(latest.bytes) > factor*float64(median):
				ratio := round1(float64(latest.bytes) / float64(median))
				row.Kind, row.Ratio = "spike", &ratio
				row.Note = fmt.Sprintf("the newest good backup moved %.1fx this device's median; a mass file change (bulk copy, re-encryption) looks like this", ratio)
			case median > 0 && flatRuns > 0 && zeros >= flatRuns:
				ratio := 0.0
				row.Kind, row.Ratio = "flat", &ratio
				row.Note = fmt.Sprintf("the last %d good backups moved 0 bytes against a median of %d; check the backup source (excluded drive, empty selection)", zeros, median)
			default:
				continue
			}
			nasRows = append(nasRows, row)
		}
		spikes := 0
		for _, r := range nasRows {
			if r.Kind == "spike" {
				spikes++
			}
		}
		for i := range nasRows {
			c := spikes
			nasRows[i].NASSpikeCount = &c
		}
		flagged = append(flagged, nasRows...)
	}
	sort.SliceStable(flagged, func(i, j int) bool {
		ri, rj := 0.0, 0.0
		if flagged[i].Ratio != nil {
			ri = *flagged[i].Ratio
		}
		if flagged[j].Ratio != nil {
			rj = *flagged[j].Ratio
		}
		return ri > rj
	})
	return append(flagged, out...), nil
}
