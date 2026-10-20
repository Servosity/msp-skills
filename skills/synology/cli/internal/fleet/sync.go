// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/dsm"
)

// Caller is the DSM RPC surface the sync needs; *dsm.Client implements it.
type Caller interface {
	Get(ctx context.Context, path string, params url.Values) (json.RawMessage, error)
}

// Spec paths (spec.yaml) the fleet layer calls. TestPathsTraceToSpec asserts
// each one appears verbatim in spec.yaml.
const (
	PathAPIInfo          = "/webapi/query.cgi?api=SYNO.API.Info&method=query&version=1&query=all"
	PathSystemInfo       = "/webapi/entry.cgi?api=SYNO.Core.System&method=info&version=3"
	PathABBTasks         = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&load_status=true&load_result=true&load_devices=true"
	PathABBResults       = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Log&method=list_result&version=1"
	PathABBResultDetail  = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Log&method=list_result_detail&version=1"
	PathABBLogs          = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Log&method=list_log&version=1"
	PathABBTransfers     = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Overview&method=list_device_transfer_size&version=1"
	PathStorage          = "/webapi/entry.cgi?api=SYNO.Storage.CGI.Storage&method=load_info&version=1"
	PathHyperBackupTasks = "/webapi/entry.cgi?api=SYNO.Backup.Task&method=list&version=1"
)

// APIOf returns the api= namespace of a spec path.
func APIOf(path string) string { return dsm.APIName(path) }

// ABB result status codes (N4S4 core_active_backup.py:82-88; dumbmonit abb.rs:181-189).
const (
	StatusRunning  = 1
	StatusSuccess  = 2
	StatusPartial  = 3
	StatusFailed   = 4
	StatusCanceled = 5
	StatusNoBackup = 6
	// JobBackup is job_action 1; other actions (version deletion, restore, ...)
	// also land in result history and in a task's last_result.
	JobBackup = 1
)

// StatusLabel renders an ABB status code.
func StatusLabel(code int) string {
	switch code {
	case StatusRunning:
		return "running"
	case StatusSuccess:
		return "success"
	case StatusPartial:
		return "partial"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		return "canceled"
	case StatusNoBackup:
		return "no_backup"
	}
	return "status_" + strconv.Itoa(code)
}

// SystemInfo is the identity a sync records per NAS.
type SystemInfo struct {
	Model        string
	DSMVersion   string
	ABBInstalled *bool
}

// Snapshot is one NAS's fetched state, written in one transaction.
type Snapshot struct {
	TakenAt      time.Time
	Warnings     []string
	ABBInstalled bool
	ABBAbsent    bool  // DSM answered 102: the package is gone, retire its rows
	StorageOK    bool  // Storage holds a complete fetch (replace that generation)
	HistoryFrom  int64 // results/device runs are complete from this time on (0 = not committed)
	Tasks        []Task
	Results      []Result
	DeviceRuns   []DeviceRun
	Storage      []StorageItem
}

type Task struct {
	TaskID, BackupType, LastResultID, LastStatus, LastJobAction int64
	// RunStart is when the backup now running started; 0 = unknown (the
	// task's last_result is not that unfinished backup).
	RunStart                        int64
	Name                            string
	Running, Scheduled              bool
	NextTrigger, LastStart, LastEnd int64
	Devices                         []Device
}

type Device struct {
	DeviceID, BackupType          int64
	HostName, OSName, AgentStatus string
}

type Result struct {
	ResultID, TaskID, Status, JobAction, BackupType int64
	TaskName, Hosts, FirstError                     string
	DeviceIDs                                       string // ",1,7," ids from task_config.device_list
	Start, End, Errors, Warnings, Bytes             int64
	// FromTask marks a run known only from its task's last_result: it carries
	// no hosts or counts, so it never overwrites a mirrored row's measurements.
	FromTask bool
}

type DeviceRun struct {
	DeviceResultID, ResultID, DeviceID, Status int64
	DeviceName                                 string
	Start, End, Bytes                          int64
}

type StorageItem struct {
	Kind, ID, Name, Status, SmartStatus, Detail string
	Temp, SizeTotal, SizeUsed                   int64
}

// Options bound how much history one sync pulls.
type Options struct {
	IgnoreCursor   bool          // re-fetch the whole InitialWindow (fleet sync --full)
	InitialWindow  time.Duration // history pulled on a NAS's first sync
	MaxResultPages int           // 200 results per page
	MaxDetails     int           // result-detail calls for failed/partial runs
}

// DefaultOptions are conservative enough for a NAS on a slow WAN link.
var DefaultOptions = Options{InitialWindow: 30 * 24 * time.Hour, MaxResultPages: 50, MaxDetails: 20}

// Fetch pulls one NAS's ABB tasks, results, device runs and storage. It returns
// a partial snapshot plus a joined error when some calls failed, so one broken
// API never hides the rest. Login and transport failures abort immediately.
func Fetch(ctx context.Context, nas Caller, cursor int64, now time.Time, opts Options) (*Snapshot, SystemInfo, error) {
	snap := &Snapshot{TakenAt: now}
	var info SystemInfo
	var errs []error

	sys, err := nas.Get(ctx, PathSystemInfo, nil)
	switch {
	case err == nil:
		m := obj(sys)
		info.Model, info.DSMVersion = str(m, "model"), str(m, "firmware_ver")
	case !isDSMError(err):
		return nil, info, err // unreachable NAS or rejected login
	default:
		errs = append(errs, fmt.Errorf("system info: %w", err))
	}

	tasksRaw, err := nas.Get(ctx, PathABBTasks, url.Values{"load_versions": {"false"}})
	// ABB counts as installed only when DSM answered for it; 102 means it is
	// absent, and a timeout or transport error says nothing either way.
	installed, answered := true, err == nil || (isDSMError(err) && !dsm.SessionExpired(dsmCode(err)))
	switch {
	case err == nil:
		if snap.Tasks, err = parseTasks(tasksRaw, now.Unix()); err != nil {
			errs = append(errs, fmt.Errorf("ABB tasks: %w", err))
		} else {
			snap.ABBInstalled = true
		}
	case dsmCode(err) == 102:
		installed = false
		snap.ABBAbsent = true
	default:
		errs = append(errs, fmt.Errorf("ABB tasks: %w", err))
	}
	if answered {
		info.ABBInstalled = &installed
	}

	if snap.ABBInstalled {
		from := now.Add(-opts.InitialWindow).Unix()
		if cursor > 0 && !opts.IgnoreCursor {
			from = cursor - 24*3600 // overlap so late-finishing runs get their final status
		}
		// Results and per-device runs share the cursor, so they are committed
		// together or not at all: a partial history would move the cursor
		// past runs that were never fetched.
		results, truncated, rerr := fetchResults(ctx, nas, from, now.Unix(), opts.MaxResultPages)
		if rerr == nil && truncated {
			rerr = fmt.Errorf("more than %d results since %s; rerun 'fleet sync --nas <name> --full --since <shorter window>'", opts.MaxResultPages*200, time.Unix(from, 0).UTC().Format(time.RFC3339))
		}
		if rerr == nil {
			rerr = attachFirstErrors(ctx, nas, results, now, opts.MaxDetails)
		}
		var runs []DeviceRun
		raw, terr := nas.Get(ctx, PathABBTransfers, url.Values{
			"time_start": {strconv.FormatInt(from, 10)}, "time_end": {strconv.FormatInt(now.Unix(), 10)},
		})
		if terr == nil {
			runs, terr = parseDeviceRuns(raw, now.Unix())
		}
		switch {
		case rerr != nil:
			errs = append(errs, fmt.Errorf("ABB results: %w", rerr))
		case terr != nil:
			errs = append(errs, fmt.Errorf("ABB device transfers: %w", terr))
		default:
			merged, merr := mergeLastResults(results, snap.Tasks)
			if merr != nil {
				errs = append(errs, fmt.Errorf("ABB results: %w", merr))
				break
			}
			snap.Results, snap.DeviceRuns, snap.HistoryFrom = merged, runs, from
		}
	}

	storage, err := nas.Get(ctx, PathStorage, nil)
	if err == nil {
		snap.Storage, err = parseStorage(storage)
		snap.StorageOK = err == nil
	}
	if err != nil {
		errs = append(errs, fmt.Errorf("storage: %w", err))
	}
	return snap, info, errors.Join(errs...)
}

// fetchResults pages job results since from. truncated reports that the page
// budget ran out before the window did.
func fetchResults(ctx context.Context, nas Caller, from, now int64, maxPages int) (out []Result, truncated bool, err error) {
	const limit = 200
	seen := map[int64]Result{}
	knownTotal := int64(-1) // the largest total any page reported
	filter, _ := json.Marshal(map[string]any{"from_timestamp": from})
	for page := 0; page < maxPages; page++ {
		raw, err := nas.Get(ctx, PathABBResults, url.Values{
			"offset": {strconv.Itoa(page * limit)}, "limit": {strconv.Itoa(limit)}, "filter": {string(filter)},
		})
		if err != nil {
			return nil, false, err
		}
		rows, err := RequireArr(raw, "results")
		if err != nil {
			return nil, false, err
		}
		// When DSM reports a total, a short page before it is a capped page,
		// not the end of history.
		if v, present := obj(raw)["total"]; present {
			t, ok := NumValue(v)
			if !ok || t < 0 {
				return nil, false, errors.New("unexpected payload: invalid total")
			}
			knownTotal = max(knownTotal, t)
		}
		dups := 0
		for _, r := range rows {
			m := asObj(r)
			if err := CheckRun(m, now, "result_id", "task_id", "job_action"); err != nil {
				return nil, false, fmt.Errorf("unexpected payload: result: %w", err)
			}
			var hosts []string
			ids := ","
			for _, d := range arr(obj2(m, "task_config"), "device_list") {
				if h := str(asObj(d), "host_name"); h != "" {
					hosts = append(hosts, h)
				}
				if id, ok := numOK(asObj(d), "device_id"); ok && id > 0 {
					ids += strconv.FormatInt(id, 10) + ","
				}
			}
			ids = normalizeIDs(ids)
			res := Result{
				ResultID: num(m, "result_id"), TaskID: num(m, "task_id"), TaskName: str(m, "task_name"),
				Status: num(m, "status"), JobAction: num(m, "job_action"), BackupType: num(m, "backup_type"),
				Start: num(m, "time_start"), End: num(m, "time_end"), Errors: num(m, "error_count"),
				Warnings: num(m, "warning_count"), Bytes: bytesOrUnknown(m), Hosts: strings.Join(hosts, ","),
				DeviceIDs: ids,
			}
			// The same run can reappear when pages shift; a conflicting copy
			// would let an upsert replace a failure with a success.
			if prev, dup := seen[res.ResultID]; dup {
				dups++
				if prev != res {
					return nil, false, fmt.Errorf("unexpected payload: result %d listed twice with different values", res.ResultID)
				}
				continue
			}
			seen[res.ResultID] = res
			out = append(out, res)
		}
		// Only distinct runs count toward the reported total: a repeated page
		// must not make a partial history look complete.
		if len(rows) == limit && dups == limit {
			return nil, false, fmt.Errorf("unexpected payload: page %d repeats earlier results", page+1)
		}
		distinct := int64(len(seen))
		if knownTotal >= 0 && distinct >= knownTotal {
			return out, false, nil
		}
		if len(rows) < limit {
			if knownTotal >= 0 {
				return nil, false, fmt.Errorf("unexpected payload: %d results reported but the listing ended at %d", knownTotal, distinct)
			}
			return out, false, nil
		}
	}
	return out, true, nil
}

// attachFirstErrors fetches the detail log of recent non-success backup runs
// and keeps the first error (or warning) line as a one-line reason.
func attachFirstErrors(ctx context.Context, nas Caller, results []Result, now time.Time, max int) error {
	recent := now.Add(-7 * 24 * time.Hour).Unix()
	for i := range results {
		r := &results[i]
		if max <= 0 {
			return nil
		}
		if r.JobAction != JobBackup || r.Start < recent || (r.Status != StatusPartial && r.Status != StatusFailed && r.Status != StatusCanceled) {
			continue
		}
		max--
		raw, err := nas.Get(ctx, PathABBResultDetail, url.Values{
			"result_id": {strconv.FormatInt(r.ResultID, 10)}, "limit": {"20"}, "order_by": {"log_level"}, "direction": {"ASC"},
		})
		if err != nil {
			// A throttled NAS must not read as "no detail"; stop and report it.
			var rateErr *cliutil.RateLimitError
			if errors.As(err, &rateErr) {
				return err
			}
			continue // a missing detail log is not worth failing the sync
		}
		r.FirstError = summarizeDetail(arr(obj(raw), "result_detail_list"))
	}
	return nil
}

// summarizeDetail renders the most severe detail row (log_level 0 error, 1
// warning) as "error log_type=6002 path=... fs_error=-65". DSM returns codes
// and parameters, not message text.
func summarizeDetail(rows []any) string {
	best, bestLevel := map[string]any(nil), int64(99)
	for _, r := range rows {
		m := asObj(r)
		if lvl := num(m, "log_level"); lvl < bestLevel {
			best, bestLevel = m, lvl
		}
	}
	if best == nil || bestLevel > 1 {
		return ""
	}
	parts := []string{map[int64]string{0: "error", 1: "warning"}[bestLevel], "log_type=" + strconv.FormatInt(num(best, "log_type"), 10)}
	if ec := num(best, "error_code"); ec != 0 {
		parts = append(parts, "error_code="+strconv.FormatInt(ec, 10))
	}
	op := obj2(best, "other_params")
	for _, k := range []string{"path", "fs_error", "os_name"} {
		if v := scalar(op[k]); v != "" && v != "0" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

func parseTasks(raw json.RawMessage, now int64) ([]Task, error) {
	tasks, err := RequireArr(raw, "tasks")
	if err != nil {
		return nil, err
	}
	if err := CheckTaskInventory(raw); err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		m := asObj(t)
		if m == nil || num(m, "task_id") <= 0 {
			return nil, errors.New("unexpected payload: a task without task_id")
		}
		devices, ok := m["devices"].([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected payload: task %d has no devices array", num(m, "task_id"))
		}
		lr, err := CheckLastResult(m["last_result"], now)
		if err != nil {
			return nil, fmt.Errorf("unexpected payload: task %d last_result: %w", num(m, "task_id"), err)
		}
		if v, present := lr["task_id"]; present {
			if id, ok := NumValue(v); !ok || id != num(m, "task_id") {
				return nil, fmt.Errorf("unexpected payload: task %d last_result names another task", num(m, "task_id"))
			}
		}
		trigger, triggerOK := numOK(m, "next_trigger_time")
		task := Task{
			TaskID: num(m, "task_id"), Name: str(m, "task_name"), BackupType: num(m, "backup_type"),
			NextTrigger: trigger, LastResultID: num(lr, "result_id"),
			LastStatus: num(lr, "status"), LastJobAction: num(lr, "job_action"),
			LastStart: num(lr, "time_start"), LastEnd: num(lr, "time_end"),
		}
		// Running: progress.running_task_status 1 (ha-activebackup), status
		// "backingup", or a last BACKUP job with a start, no end and no final
		// status (dumbmonit). A failed job without time_end is not running,
		// and an unfinished version deletion is not a backup.
		task.Running = num(obj2(m, "progress"), "running_task_status") == 1 || str(m, "status") == "backingup" ||
			(task.LastJobAction == JobBackup && task.LastStart > 0 && task.LastEnd == 0 &&
				(task.LastStatus == StatusRunning || task.LastStatus == 0))
		// The run's start is known only when last_result is that unfinished
		// backup; otherwise reports look for a running backup result.
		if task.Running && task.LastJobAction == JobBackup && task.LastEnd == 0 &&
			(task.LastStatus == StatusRunning || task.LastStatus == 0) {
			task.RunStart = task.LastStart
		}
		paused, _ := obj2(m, "sched_content")["is_continuous_paused"].(bool)
		// A valid next_trigger_time <= 0 means unscheduled; a missing or
		// malformed one is unknown, so the task stays scheduled and a missed
		// run is still reported.
		task.Scheduled = str(m, "status") != "unscheduled" && !paused && (!triggerOK || task.NextTrigger > 0)
		seen := map[int64]bool{}
		for _, d := range devices {
			dm := asObj(d)
			if dm == nil || num(dm, "device_id") <= 0 || seen[num(dm, "device_id")] {
				return nil, fmt.Errorf("unexpected payload: task %d has a device without a unique device_id", task.TaskID)
			}
			seen[num(dm, "device_id")] = true
			task.Devices = append(task.Devices, Device{
				DeviceID: num(dm, "device_id"), HostName: str(dm, "host_name"), OSName: str(dm, "os_name"),
				AgentStatus: str(dm, "agent_status"), BackupType: num(dm, "backup_type"),
			})
		}
		out = append(out, task)
	}
	return out, nil
}

// maxClockSkew is how far in the future a NAS timestamp may be before it is
// treated as malformed (a future time would hide later runs).
const maxClockSkew = 24 * 3600

// CheckRun validates the identity and time fields of a result or transfer
// row (abb triage applies it too): each id key a positive integer, status a
// positive integer, time_start a positive time not in the future, and
// time_end either 0 (unfinished) or a time between time_start and now.
func CheckRun(m map[string]any, now int64, idKeys ...string) error {
	if m == nil {
		return errors.New("not an object")
	}
	for _, k := range append(idKeys, "status") {
		if v, ok := numOK(m, k); !ok || v <= 0 {
			return fmt.Errorf("no valid %s", k)
		}
	}
	start, ok := numOK(m, "time_start")
	if !ok || start <= 0 {
		return fmt.Errorf("%s has no valid time_start", idKeys[0])
	}
	if num(m, "status") == StatusRunning && num(m, "time_end") > 0 {
		return fmt.Errorf("%s is running with an end time", idKeys[0])
	}
	return checkTimes(m, now)
}

// checkTimes bounds a row's present time fields: integers, not in the future,
// and an end (when nonzero) no earlier than its start.
func checkTimes(m map[string]any, now int64) error {
	start, end := int64(0), int64(0)
	for _, k := range []string{"time_start", "time_end"} {
		if _, present := m[k]; !present {
			continue
		}
		v, ok := numOK(m, k)
		if !ok || v < 0 || v > now+maxClockSkew {
			return fmt.Errorf("no valid %s", k)
		}
		if k == "time_start" {
			start = v
		} else {
			end = v
		}
	}
	if end > 0 && end < start {
		return errors.New("time_end before time_start")
	}
	return nil
}

// CheckTaskInventory validates an ABB task list as a complete inventory (abb
// triage applies it too): every task has a unique positive task_id and the
// list holds at least DSM's reported total. A task missing from the list
// would have its devices retired and read as "nothing to protect".
func CheckTaskInventory(raw json.RawMessage) error {
	tasks, err := RequireArr(raw, "tasks")
	if err != nil {
		return err
	}
	ids := map[int64]bool{}
	for _, t := range tasks {
		id, ok := numOK(asObj(t), "task_id")
		if !ok || id <= 0 {
			return errors.New("unexpected payload: a task without task_id")
		}
		if ids[id] {
			return fmt.Errorf("unexpected payload: task %d listed twice", id)
		}
		ids[id] = true
	}
	if v, present := obj(raw)["total"]; present {
		total, ok := NumValue(v)
		if !ok || total < 0 {
			return errors.New("unexpected payload: invalid total")
		}
		if int64(len(ids)) < total {
			return fmt.Errorf("unexpected payload: %d tasks reported but %d listed", total, len(ids))
		}
	}
	return nil
}

// normalizeIDs renders an id set as ",1,7," (sorted, unique; "" when empty)
// so two copies of one set always compare equal.
func normalizeIDs(s string) string {
	var ids []int64
	seen := map[int64]bool{}
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64); err == nil && n > 0 && !seen[n] {
			seen[n] = true
			ids = append(ids, n)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ","
	for _, n := range ids {
		out += strconv.FormatInt(n, 10) + ","
	}
	return out
}

// CheckLastResult validates a task's last_result (abb triage applies it too).
// Absent, null or an object with no run fields (a task that never ran) yields
// nil. Anything else must be an object describing a whole run: positive
// result_id, job_action and time_start, a status (0 or 1 while running) and
// bounded times. Any other shape is an error, never "no evidence".
func CheckLastResult(v any, now int64) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	lr, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("not an object")
	}
	// Empty means every run field is absent, null, 0 or "": any other value,
	// including an array or object, makes it a run that must then validate.
	empty := true
	for _, k := range []string{"result_id", "status", "job_action", "time_start", "time_end"} {
		x, present := lr[k]
		if !present || x == nil || x == "" {
			continue
		}
		if n, ok := NumValue(x); ok && n == 0 {
			continue
		}
		empty = false
	}
	if empty {
		return nil, nil
	}
	for _, k := range []string{"result_id", "job_action", "time_start"} {
		if n, ok := numOK(lr, k); !ok || n <= 0 {
			return nil, fmt.Errorf("no valid %s", k)
		}
	}
	status, ok := numOK(lr, "status")
	if !ok || status < 0 {
		return nil, errors.New("no valid status")
	}
	if end, _ := numOK(lr, "time_end"); end > 0 && (status == 0 || status == StatusRunning) {
		return nil, errors.New("an unfinished status with an end time")
	}
	if err := checkTimes(lr, now); err != nil {
		return nil, err
	}
	return lr, nil
}

// mergeLastResults folds each task's finished last_result backup into the
// fetched results: a run the listing omitted is added (marked FromTask), a
// run the listing still shows as running takes the task's final status, and
// any other disagreement about one run (identity or outcome) is an error, so
// the NAS reads as not synced instead of a guess.
func mergeLastResults(results []Result, tasks []Task) ([]Result, error) {
	idx := map[int64]int{}
	for i, r := range results {
		idx[r.ResultID] = i
	}
	// Every populated last_result names one run. Whatever its status or job,
	// the other tasks and the listing must agree on that run's task, job and
	// start, and it is mirrored (as a task-only row when the listing omits it)
	// so a later sync is checked against it too.
	byID := map[int64]Task{}
	for _, t := range tasks {
		if t.LastResultID <= 0 {
			continue
		}
		if o, dup := byID[t.LastResultID]; dup {
			return nil, fmt.Errorf("unexpected payload: result %d is the last result of tasks %d and %d", t.LastResultID, o.TaskID, t.TaskID)
		}
		byID[t.LastResultID] = t
		if i, ok := idx[t.LastResultID]; ok {
			if r := results[i]; r.TaskID != t.TaskID || r.JobAction != t.LastJobAction || r.Start != t.LastStart {
				return nil, fmt.Errorf("unexpected payload: result %d differs between the task list and the result listing", t.LastResultID)
			}
		}
	}
	for _, t := range tasks {
		if t.LastResultID <= 0 {
			continue
		}
		lr := Result{ResultID: t.LastResultID, TaskID: t.TaskID, TaskName: t.Name, Status: t.LastStatus,
			JobAction: t.LastJobAction, BackupType: t.BackupType, Start: t.LastStart, End: t.LastEnd, FromTask: true}
		if lr.Status == 0 {
			lr.Status = StatusRunning // a task reports an unfinished run as 0; mirrored, it is running
		}
		i, ok := idx[lr.ResultID]
		if !ok {
			idx[lr.ResultID] = len(results)
			results = append(results, lr)
			continue
		}
		if lr.JobAction != JobBackup || lr.Status == 0 || lr.Status == StatusRunning {
			continue // identity agreed above; only a finished backup reconciles the outcome
		}
		r := &results[i]
		if r.TaskID != lr.TaskID || r.JobAction != JobBackup || r.Start != lr.Start {
			return nil, fmt.Errorf("unexpected payload: result %d differs between the task list and the result listing", lr.ResultID)
		}
		if r.End > 0 && lr.End > 0 && r.End != lr.End {
			return nil, fmt.Errorf("unexpected payload: result %d has different end times in the task list and the result listing", lr.ResultID)
		}
		switch {
		case r.Status == lr.Status:
			r.End = max(r.End, lr.End) // the listing may not have the end time yet
		case r.Status == StatusRunning: // finished between the two calls
			r.Status, r.End = lr.Status, max(r.End, lr.End)
		default:
			return nil, fmt.Errorf("unexpected payload: result %d is %s in the result listing but %s in its task",
				lr.ResultID, StatusLabel(int(r.Status)), StatusLabel(int(lr.Status)))
		}
	}
	return results, nil
}

func parseDeviceRuns(raw json.RawMessage, now int64) ([]DeviceRun, error) {
	devices, err := RequireArr(raw, "device_list")
	if err != nil {
		return nil, err
	}
	var out []DeviceRun
	seen := map[int64]bool{}       // device_result_id
	identities := map[int64]bool{} // device id per entry
	for _, e := range devices {
		m := asObj(e)
		transfers, ok := m["transfer_list"].([]any)
		if m == nil || !ok {
			return nil, errors.New("unexpected payload: a device without transfer_list")
		}
		// Each entry is one device: its id comes from device.device_id or,
		// failing that, its transfers' config_device_id, and every source
		// present must agree. An entry with no identity, or one listed twice,
		// would credit (or hide) the wrong machine.
		dev := obj2(m, "device")
		id := int64(0)
		if v, present := dev["device_id"]; present {
			n, ok := NumValue(v)
			if !ok || n <= 0 {
				return nil, errors.New("unexpected payload: a device with an invalid device_id")
			}
			id = n
		}
		for _, t := range transfers {
			v, present := asObj(t)["config_device_id"]
			if !present {
				continue
			}
			n, ok := NumValue(v)
			if !ok || n <= 0 || (id != 0 && n != id) {
				return nil, fmt.Errorf("unexpected payload: device %d has an invalid or conflicting config_device_id", id)
			}
			id = n
		}
		if id <= 0 {
			return nil, errors.New("unexpected payload: a device entry without a device id")
		}
		if identities[id] {
			return nil, fmt.Errorf("unexpected payload: device %d listed twice", id)
		}
		identities[id] = true
		name := str(dev, "host_name")
		for _, t := range transfers {
			tm := asObj(t)
			if err := CheckRun(tm, now, "device_result_id"); err != nil {
				return nil, fmt.Errorf("unexpected payload: transfer: %w", err)
			}
			rid := num(tm, "device_result_id")
			if seen[rid] {
				return nil, fmt.Errorf("unexpected payload: transfer %d listed twice", rid)
			}
			seen[rid] = true
			if _, present := tm["result_id"]; present {
				if r, ok := numOK(tm, "result_id"); !ok || r < 0 {
					return nil, fmt.Errorf("unexpected payload: transfer %d has an invalid result_id", rid)
				}
			}
			tname := name
			if tname == "" {
				tname = str(tm, "device_name")
			}
			out = append(out, DeviceRun{
				DeviceResultID: rid, ResultID: num(tm, "result_id"), DeviceID: id,
				DeviceName: tname, Status: num(tm, "status"), Start: num(tm, "time_start"), End: num(tm, "time_end"),
				Bytes: bytesOrUnknown(tm),
			})
		}
	}
	// A listing with fewer distinct devices than DSM's own total is incomplete.
	if v, present := obj(raw)["total"]; present {
		total, ok := NumValue(v)
		if !ok || total < 0 {
			return nil, errors.New("unexpected payload: invalid total")
		}
		if int64(len(identities)) < total {
			return nil, fmt.Errorf("unexpected payload: %d devices reported but %d listed", total, len(identities))
		}
	}
	return out, nil
}
func parseStorage(raw json.RawMessage) ([]StorageItem, error) {
	m := obj(raw)
	if _, err := RequireArr(raw, "volumes"); err != nil {
		return nil, err
	}
	if _, err := RequireArr(raw, "disks"); err != nil {
		return nil, err
	}
	if _, err := RequireArr(raw, "storagePools"); err != nil {
		return nil, err
	}
	var out []StorageItem
	ids := map[string]bool{}
	// Every item needs a stable id unique within its kind: two items sharing
	// one key would let a healthy row replace a degraded one.
	idOf := func(kind string, im map[string]any) (string, error) {
		id := str(im, "id")
		if im == nil || id == "" || ids[kind+"/"+id] {
			return "", fmt.Errorf("unexpected payload: a %s without a unique id", kind)
		}
		ids[kind+"/"+id] = true
		return id, nil
	}
	for _, v := range arr(m, "volumes") {
		vm := asObj(v)
		id, err := idOf("volume", vm)
		if err != nil {
			return nil, err
		}
		size := obj2(vm, "size")
		name := str(vm, "vol_path")
		if name == "" {
			name = id
		}
		// Capacity counts only when both fields parse and make sense; -1
		// marks it unknown so health reports "capacity not reported".
		total, okT := numOK(size, "total")
		used, okU := numOK(size, "used")
		if !okT || !okU || total <= 0 || used < 0 || used > total {
			total, used = -1, -1
		}
		out = append(out, StorageItem{Kind: "volume", ID: id, Name: name, Status: str(vm, "status"),
			SizeTotal: total, SizeUsed: used, Detail: str(obj2(vm, "space_status"), "detail")})
	}
	for _, p := range arr(m, "storagePools") {
		pm := asObj(p)
		if _, err := idOf("pool", pm); err != nil {
			return nil, err
		}
		size := obj2(pm, "size")
		out = append(out, StorageItem{Kind: "pool", ID: str(pm, "id"), Name: str(pm, "id"), Status: str(pm, "status"),
			SizeTotal: num(size, "total"), SizeUsed: num(size, "used"), Detail: str(pm, "device_type")})
	}
	for _, d := range arr(m, "disks") {
		dm := asObj(d)
		if _, err := idOf("disk", dm); err != nil {
			return nil, err
		}
		var flags []string
		for _, k := range []string{"below_remain_life_thr", "exceed_bad_sector_thr", "remain_life_danger"} {
			if b, _ := dm[k].(bool); b {
				flags = append(flags, k)
			}
		}
		name := str(dm, "longName")
		if name == "" {
			name = str(dm, "name")
		}
		if dev := str(dm, "device"); dev != "" {
			name += " (" + dev + ")"
		}
		temp, ok := numOK(dm, "temp")
		if !ok || temp <= 0 {
			temp = -1 // not reported: health says so instead of reading it as cool
		}
		out = append(out, StorageItem{Kind: "disk", ID: str(dm, "id"), Name: strings.TrimSpace(name), Status: str(dm, "status"),
			SmartStatus: str(dm, "smart_status"), Temp: temp, SizeTotal: num(dm, "size_total"),
			Detail: strings.Join(append([]string{str(dm, "model")}, flags...), " ")})
	}
	return out, nil
}

// Decode unmarshals DSM JSON keeping numbers exact (json.Number): a float64
// round trip would turn 2.0000000000000001 into the integer 2.
func Decode(raw json.RawMessage, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(v)
}

// RequireArr reads key from a JSON object as an array, failing when the
// payload is not an object or the key is missing: an unexpected shape must
// not read as "nothing there".
func RequireArr(raw json.RawMessage, key string) ([]any, error) {
	var m map[string]any
	if err := Decode(raw, &m); err != nil {
		return nil, fmt.Errorf("unexpected payload (not a JSON object): %w", err)
	}
	a, ok := m[key].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected payload: no %q array", key)
	}
	return a, nil
}

// --- loose JSON helpers: DSM field types drift across versions (sizes are
// strings on DSM 7, remain_life became an object), so read leniently.

func obj(raw json.RawMessage) map[string]any {
	var m map[string]any
	_ = Decode(raw, &m)
	return m
}

func asObj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func obj2(m map[string]any, k string) map[string]any { return asObj(m[k]) }

func arr(m map[string]any, k string) []any {
	a, _ := m[k].([]any)
	return a
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// numOK reads a numeric field (JSON number or numeric string) and reports
// whether it was present and valid.
func numOK(m map[string]any, k string) (int64, bool) {
	return NumValue(m[k])
}

// NumValue reads an exact integer from a decoded JSON value: a json.Number or
// numeric string must parse as an integer; a float64 must be whole.
func NumValue(x any) (int64, bool) {
	switch v := x.(type) {
	case json.Number:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil { // ParseInt returns MaxInt64 on overflow; that is no value
			return 0, false
		}
		return n, true
	case float64:
		// Only a whole, in-range number counts: 0.5 must not truncate to 0.
		if v != math.Trunc(v) || math.Abs(v) > 1<<62 {
			return 0, false
		}
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// num reads an integer field through numOK; an absent, fractional or
// malformed value reads as 0, which every required-field check rejects.
// bytesOrUnknown reads DSM's transfered_bytes; -1 marks a count DSM did not
// report (or reported as no valid number), so a report shows it as unknown
// (null), never as 0 bytes transferred.
func bytesOrUnknown(m map[string]any) int64 {
	if b, ok := numOK(m, "transfered_bytes"); ok && b >= 0 {
		return b
	}
	return -1
}

func num(m map[string]any, k string) int64 {
	if v, ok := numOK(m, k); ok {
		return v
	}
	return 0
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func isDSMError(err error) bool {
	var de *dsm.Error
	return errors.As(err, &de) && !de.IsAuth()
}

func dsmCode(err error) int {
	var de *dsm.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return 0
}
