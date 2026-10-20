// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb triage: why one ABB task failed, in one timeline.
// pp:data-source live
// pp:client-call - live DSM calls go through internal/dsm.Client (dsmClient + triage below).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

type triageEntry struct {
	Source    string         `json:"source"` // result_detail or abb_log
	Time      int64          `json:"time"`
	Level     string         `json:"level"`
	LogType   int64          `json:"log_type"`
	ErrorCode int64          `json:"error_code,omitempty"`
	Device    string         `json:"device,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

type triageReport struct {
	NAS      string         `json:"nas"`
	TaskID   int64          `json:"task_id"`
	TaskName string         `json:"task_name"`
	Result   map[string]any `json:"result,omitempty"`
	Status   string         `json:"status,omitempty"`
	Timeline []triageEntry  `json:"timeline"`
	Note     string         `json:"note,omitempty"`
}

// ReportIdentity is what --quiet prints: the task.
func (r triageReport) ReportIdentity() string {
	if r.TaskName != "" {
		return r.TaskName
	}
	return strconv.FormatInt(r.TaskID, 10)
}

func newNovelAbbTriageCmd(flags *rootFlags) *cobra.Command {
	var nasName string
	var scan, detailLimit int
	cmd := &cobra.Command{
		Use:   "triage [task]",
		Short: "Why one ABB task failed: its newest failed run, the run's detail lines and the ABB log from that window, errors first",
		Long: strings.Trim(`
Use this command to see why one specific ABB task failed. Do NOT use this
command to find which tasks failed; use 'abb failures' instead.

[task] is a task id or name (case-insensitive; a unique partial name works).
Live: reads the task list, the task's backup results, the newest non-success
run's detail lines and the ABB activity log between that run's start and end.
DSM reports log types and parameters (path, fs_error, error_code), not message
text.`, "\n"),
		Example: strings.Trim(`
  synology-cli abb triage "Office PCs" --agent
  synology-cli abb triage 8 --nas acme`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "task=example"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb triage")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a task id or name is required"))
			}
			if scan <= 0 || detailLimit <= 0 {
				return usageErr(fmt.Errorf("--scan and --detail-limit must be positive"))
			}
			n, err := resolveNAS(flags, nasName)
			if err != nil {
				return err
			}
			if !n.Credentials().Complete() {
				return authErr(fmt.Errorf("set %s_ACCOUNT and %s_PASSWORD", n.EnvPrefix, n.EnvPrefix))
			}
			c, err := dsmClient(flags, n)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			defer func() { _ = c.Logout(context.Background()) }()
			rep, err := triage(ctx, c, n, args[0], scan, detailLimit)
			if err != nil {
				return redactErr(c, classifyDSMErr(err))
			}
			return printReportJSON(cmd.OutOrStdout(), rep, flags) // agent-compaction-keeps-report-fields
		},
	}
	cmd.Flags().StringVar(&nasName, "nas", "", "Registered NAS to query (optional when only one NAS is configured)")
	cmd.Flags().IntVar(&scan, "scan", 50, "Newest backup runs to scan for the latest non-success run")
	cmd.Flags().IntVar(&detailLimit, "detail-limit", 200, "Maximum detail and log lines to fetch")
	return cmd
}

func triage(ctx context.Context, c *dsm.Client, n fleet.NAS, want string, scan, limit int) (*triageReport, error) {
	raw, err := c.Get(ctx, fleet.PathABBTasks, nil)
	if err != nil {
		return nil, err
	}
	// The same inventory rules as fleet sync: unique task ids, nothing short
	// of DSM's reported total.
	if err := fleet.CheckTaskInventory(raw); err != nil {
		return nil, apiErr(fmt.Errorf("ABB task list: %w", err))
	}
	var tasks struct {
		Tasks []struct {
			TaskID     int64  `json:"task_id"`
			TaskName   string `json:"task_name"`
			LastResult any    `json:"last_result"`
		} `json:"tasks"`
	}
	if err := fleet.Decode(raw, &tasks); err != nil {
		return nil, apiErr(fmt.Errorf("ABB task list: %w", err))
	}
	// Every populated last_result is validated and owned by one task, as in
	// fleet sync, before any task is selected.
	nowUnix := time.Now().Unix()
	claims := map[int64]map[string]any{} // result id -> last_result, with its task
	for _, t := range tasks.Tasks {
		lr, err := fleet.CheckLastResult(t.LastResult, nowUnix)
		if err != nil {
			return nil, apiErr(fmt.Errorf("ABB task list: task %d last_result: %w", t.TaskID, err))
		}
		if lr == nil {
			continue
		}
		if v, present := lr["task_id"]; present {
			if owner, ok := fleet.NumValue(v); !ok || owner != t.TaskID {
				return nil, apiErr(fmt.Errorf("ABB task list: task %d last_result names another task", t.TaskID))
			}
		}
		id := numOf(lr["result_id"])
		if owner, dup := claims[id]; dup {
			return nil, apiErr(fmt.Errorf("ABB task list: result %d is the last result of tasks %d and %d", id, numOf(owner["task_id"]), t.TaskID))
		}
		claim := map[string]any{"task_id": json.Number(strconv.FormatInt(t.TaskID, 10))}
		for _, k := range []string{"job_action", "time_start"} {
			claim[k] = lr[k]
		}
		claims[id] = claim
	}
	// A task id wins outright; otherwise an exact name, then a partial name,
	// and either must be unique. (A task named "8" never shadows task 8.)
	var byID, exact, partial []int
	for i, t := range tasks.Tasks {
		if t.TaskID <= 0 {
			return nil, apiErr(errors.New("ABB task list: unexpected payload: a task without task_id"))
		}
		switch {
		case strconv.FormatInt(t.TaskID, 10) == want:
			byID = append(byID, i)
		case strings.EqualFold(t.TaskName, want):
			exact = append(exact, i)
		case strings.Contains(strings.ToLower(t.TaskName), strings.ToLower(want)):
			partial = append(partial, i)
		}
	}
	matches := byID
	if len(matches) == 0 {
		matches = exact
	}
	if len(matches) == 0 {
		matches = partial
	}
	switch {
	case len(matches) == 0:
		return nil, notFoundErr(fmt.Errorf("no ABB task on %s matches %q", n.Name, want))
	case len(matches) > 1:
		var names []string
		for _, i := range matches {
			names = append(names, fmt.Sprintf("%d %q", tasks.Tasks[i].TaskID, tasks.Tasks[i].TaskName))
		}
		return nil, usageErr(fmt.Errorf("%q matches several tasks: %s", want, strings.Join(names, ", ")))
	}
	task := tasks.Tasks[matches[0]]
	rep := &triageReport{NAS: n.Name, TaskID: task.TaskID, TaskName: task.TaskName, Timeline: make([]triageEntry, 0)}

	filter, _ := json.Marshal(map[string]any{"job_action": fleet.JobBackup})
	raw, err = c.Get(ctx, fleet.PathABBResults, url.Values{
		"task_id": {strconv.FormatInt(task.TaskID, 10)}, "limit": {strconv.Itoa(scan)}, "filter": {string(filter)},
	})
	if err != nil {
		return nil, err
	}
	if _, err := fleet.RequireArr(raw, "results"); err != nil {
		return nil, apiErr(fmt.Errorf("ABB results: %w", err))
	}
	var results struct {
		Results []map[string]any `json:"results"`
		Total   any              `json:"total"`
	}
	if err := fleet.Decode(raw, &results); err != nil {
		return nil, apiErr(fmt.Errorf("ABB results: %w", err))
	}
	// A listing shorter than DSM's own total (up to the requested scan) is
	// incomplete, not a history without failures.
	wantTotal := int64(-1)
	if results.Total != nil {
		total, ok := fleet.NumValue(results.Total)
		if !ok || total < 0 {
			return nil, apiErr(errors.New("ABB results: unexpected payload: invalid total"))
		}
		wantTotal = min(total, int64(scan))
	}
	// Validate every row before sorting or selecting: a malformed row must not
	// read as "no failed run" or sort behind an older one. Another task's rows
	// (if DSM ignored the task_id filter) are dropped.
	mine := results.Results[:0]
	seen := map[int64]map[string]any{}
	for _, r := range results.Results {
		// The same row rules as fleet sync: ids, status, and time bounds.
		if err := fleet.CheckRun(r, nowUnix, "result_id", "task_id", "job_action"); err != nil {
			return nil, apiErr(fmt.Errorf("ABB results: unexpected payload: %w", err))
		}
		// One run listed twice must agree with itself; a conflicting copy
		// is inconsistent evidence, not a second run.
		id := numOf(r["result_id"])
		if prev, dup := seen[id]; dup {
			for _, k := range []string{"task_id", "status", "job_action", "time_start", "time_end"} {
				if numOf(prev[k]) != numOf(r[k]) {
					return nil, apiErr(fmt.Errorf("ABB results: result %d is listed twice with different %s", id, k))
				}
			}
			continue
		}
		seen[id] = r
		if numOf(r["task_id"]) == task.TaskID {
			mine = append(mine, r)
		}
	}
	// Every task's last_result must agree with the listing's copy of its run,
	// whichever task the listing was asked about.
	for id, claim := range claims {
		prev, listed := seen[id]
		if !listed {
			continue
		}
		for _, k := range []string{"task_id", "job_action", "time_start"} {
			if numOf(prev[k]) != numOf(claim[k]) {
				return nil, apiErr(fmt.Errorf("ABB results: result %d differs between the task list and the result listing (%s)", id, k))
			}
		}
	}
	// Completeness counts distinct backup runs of this task: a repeated row,
	// another task's run or a non-backup job is not one of the runs asked for.
	backups := int64(0)
	for _, r := range mine {
		if numOf(r["job_action"]) == fleet.JobBackup {
			backups++
		}
	}
	if backups < wantTotal {
		return nil, apiErr(fmt.Errorf("ABB results: DSM reported %d backup runs but returned %d", wantTotal, backups))
	}
	// The task's own finished last_result counts even when the listing omits
	// it, under the same rules as fleet sync: malformed evidence or a
	// disagreement about one run is an error, never a quiet "no failure".
	lr, err := fleet.CheckLastResult(task.LastResult, nowUnix)
	if err != nil {
		return nil, apiErr(fmt.Errorf("ABB task list: unexpected payload: last_result: %w", err))
	}
	if v, present := lr["task_id"]; present {
		if id, ok := fleet.NumValue(v); !ok || id != task.TaskID {
			return nil, apiErr(fmt.Errorf("ABB task list: task %d last_result names another task", task.TaskID))
		}
	}
	// Whatever its status or job, a populated last_result names one run; the
	// listing must agree on that run's task, job and start.
	if lr != nil {
		if prev, ok := seen[numOf(lr["result_id"])]; ok && (numOf(prev["task_id"]) != task.TaskID ||
			numOf(prev["job_action"]) != numOf(lr["job_action"]) || numOf(prev["time_start"]) != numOf(lr["time_start"])) {
			return nil, apiErr(fmt.Errorf("ABB results: result %d differs between the task list and the result listing", numOf(lr["result_id"])))
		}
	}
	if s := numOf(lr["status"]); lr != nil && numOf(lr["job_action"]) == fleet.JobBackup && s != 0 && s != fleet.StatusRunning {
		lr["task_id"] = json.Number(strconv.FormatInt(task.TaskID, 10))
		if prev, ok := seen[numOf(lr["result_id"])]; ok && numOf(prev["task_id"]) != task.TaskID {
			return nil, apiErr(fmt.Errorf("ABB results: result %d is task %d's last_result but belongs to task %d in the listing",
				numOf(lr["result_id"]), task.TaskID, numOf(prev["task_id"])))
		}
		listed := false
		for i, r := range mine {
			if numOf(r["result_id"]) != numOf(lr["result_id"]) {
				continue
			}
			listed = true
			rs := numOf(r["status"])
			re, le := numOf(r["time_end"]), numOf(lr["time_end"])
			switch {
			case numOf(r["job_action"]) != fleet.JobBackup || numOf(r["time_start"]) != numOf(lr["time_start"]) || (re > 0 && le > 0 && re != le):
				return nil, apiErr(fmt.Errorf("ABB results: result %d differs between the task list and the result listing", numOf(lr["result_id"])))
			case rs == s:
				if re == 0 && le > 0 {
					r["time_end"] = lr["time_end"] // the listing may not have the end time yet
				}
			case rs == fleet.StatusRunning:
				mine[i] = lr
			default:
				return nil, apiErr(fmt.Errorf("ABB results: result %d is %s in the listing but %s in its task", numOf(lr["result_id"]), fleet.StatusLabel(int(rs)), fleet.StatusLabel(int(s))))
			}
		}
		if !listed {
			mine = append(mine, lr)
		}
	}
	// Newest first; at a tied start a non-success sorts first, so a tie never
	// reads as "a newer run succeeded".
	sort.SliceStable(mine, func(i, j int) bool {
		si, sj := numOf(mine[i]["time_start"]), numOf(mine[j]["time_start"])
		if si != sj {
			return si > sj
		}
		return numOf(mine[i]["status"]) != fleet.StatusSuccess && numOf(mine[j]["status"]) == fleet.StatusSuccess
	})
	var bad map[string]any
	var newerOK int64
	for _, r := range mine {
		if numOf(r["job_action"]) != fleet.JobBackup {
			continue
		}
		s := numOf(r["status"])
		if s == fleet.StatusSuccess && newerOK == 0 {
			newerOK = numOf(r["time_start"])
		}
		if s != fleet.StatusSuccess && s != fleet.StatusRunning {
			bad = r
			break
		}
	}
	if bad == nil {
		rep.Note = fmt.Sprintf("no failed, partial or canceled backup run among the newest %d runs", len(mine))
		return rep, nil
	}
	rep.Result, rep.Status = bad, fleet.StatusLabel(int(numOf(bad["status"])))
	if newerOK > numOf(bad["time_start"]) {
		rep.Note = "a newer backup run succeeded (started " + time.Unix(newerOK, 0).UTC().Format(time.RFC3339) + "); this failure may already be resolved"
	}
	resultID := numOf(bad["result_id"])

	raw, err = c.Get(ctx, fleet.PathABBResultDetail, url.Values{
		"result_id": {strconv.FormatInt(resultID, 10)}, "limit": {strconv.Itoa(limit)}, "order_by": {"log_level"}, "direction": {"ASC"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := fleet.RequireArr(raw, "result_detail_list"); err != nil {
		return nil, apiErr(fmt.Errorf("ABB result detail: %w", err))
	}
	var detail struct {
		Rows []map[string]any `json:"result_detail_list"`
	}
	if err := fleet.Decode(raw, &detail); err != nil {
		return nil, apiErr(fmt.Errorf("ABB result detail: %w", err))
	}
	for _, d := range detail.Rows {
		rep.Timeline = append(rep.Timeline, entryOf("result_detail", d))
	}

	start, end := numOf(bad["time_start"]), numOf(bad["time_end"])
	if end < start {
		// No end time: search the log up to the next run's start (or now), so
		// an error late in a long run is not cut off at the first minute.
		end = nowUnix
		for _, r := range mine {
			if s := numOf(r["time_start"]); numOf(r["job_action"]) == fleet.JobBackup && s > start && s < end {
				end = s
			}
		}
		rep.Note = strings.TrimSpace(rep.Note + " The run has no end time; the activity log was read up to " + time.Unix(end, 0).UTC().Format(time.RFC3339) + ".")
	}
	logFilter, _ := json.Marshal(map[string]any{"from_timestamp": start, "to_timestamp": end + 60})
	raw, err = c.Get(ctx, fleet.PathABBLogs, url.Values{
		"task_id": {strconv.FormatInt(task.TaskID, 10)}, "limit": {strconv.Itoa(limit)}, "filter": {string(logFilter)},
	})
	if err != nil {
		return nil, err
	}
	if _, err := fleet.RequireArr(raw, "logs"); err != nil {
		return nil, apiErr(fmt.Errorf("ABB log: %w", err))
	}
	var logs struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := fleet.Decode(raw, &logs); err != nil {
		return nil, apiErr(fmt.Errorf("ABB log: %w", err))
	}
	for _, l := range logs.Logs {
		rep.Timeline = append(rep.Timeline, entryOf("abb_log", l))
	}
	rank := map[string]int{"error": 0, "warning": 1, "information": 2}
	sort.SliceStable(rep.Timeline, func(i, j int) bool {
		a, b := rep.Timeline[i], rep.Timeline[j]
		if rank[a.Level] != rank[b.Level] {
			return rank[a.Level] < rank[b.Level]
		}
		return a.Time < b.Time
	})
	return rep, nil
}

func entryOf(source string, m map[string]any) triageEntry {
	levels := map[int64]string{0: "error", 1: "warning", 2: "information"}
	level, ok := levels[numOf(m["log_level"])]
	if !ok {
		level = "information"
	}
	params, _ := m["other_params"].(map[string]any)
	device, _ := m["device_name"].(string)
	return triageEntry{Source: source, Time: numOf(m["log_time"]), Level: level, LogType: numOf(m["log_type"]),
		ErrorCode: numOf(m["error_code"]), Device: device, Params: params}
}

// numOf reads an exact integer the way fleet.CheckRun validated it (0 when
// invalid), so a value that passed validation never converts differently.
func numOf(v any) int64 {
	n, _ := fleet.NumValue(v)
	return n
}

// classifyDSMErr maps DSM failures onto the CLI's typed exit codes.
func classifyDSMErr(err error) error {
	var ce *cliError
	if As(err, &ce) {
		return err
	}
	var de *dsm.Error
	if As(err, &de) {
		switch de.HTTPStatus() {
		case 401, 403:
			return authErr(err)
		case 404:
			return notFoundErr(err)
		}
	}
	return apiErr(err)
}
