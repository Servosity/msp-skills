// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/client"
	"kaseya-vsa-pp-cli/internal/cliutil"
)

type taskScriptRow struct {
	DeviceID    string `json:"device_id"`
	Device      string `json:"device"`
	DeviceState string `json:"device_state"`
	ScriptID    string `json:"script_id"`
	Script      string `json:"script"`
	Successful  *bool  `json:"successful"`
	ExitCode    *int   `json:"exit_code"`
	Output      string `json:"output"`
}

type taskRunResult struct {
	TaskID          int64                 `json:"task_id"`
	ExecutionID     int64                 `json:"execution_id"`
	State           string                `json:"state"`
	DeviceCount     any                   `json:"device_count"`
	SuccessfulCount any                   `json:"successful_device_count"`
	FailedCount     any                   `json:"failed_device_count"`
	Duration        any                   `json:"duration"`
	Polls           int                   `json:"polls"`
	TimedOut        bool                  `json:"timed_out"`
	Results         []taskScriptRow       `json:"results"`
	DevicesScanned  int                   `json:"devices_scanned"`
	Note            string                `json:"note,omitempty"`
	FetchFailures   []briefSectionFailure `json:"fetch_failures"`
}

func newNovelTasksRunWaitCmd(flags *rootFlags) *cobra.Command {
	var flagDevices, flagAttach, flagWebhook, flagMaxWait, flagInterval string
	var failedOnly, taskScope bool
	var maxDevices, concurrency int

	cmd := &cobra.Command{
		Use:   "run-wait [task-id]",
		Short: "Run a saved task and collect per-device, per-script exit codes and output.",
		Long: `Runs a saved automation task (POST /automation/tasks/{id}/run), waits for the execution to end,
then walks every targeted device and every script the task ran on it, returning one row per
device x script with the device state, success flag, exit code and output. --devices takes device
names or Identifiers, comma-separated; to run on every device in the task's configured scope instead,
pass --task-scope explicitly (the run starts immediately; there is no confirmation prompt). --attach
EXECUTION_ID skips the run and collects an existing execution (for example one a webhook reported).

Exit codes: 0 when no device failed; 6 when any device failed; 8 when the execution was still
running after --max-wait (re-run with --attach to keep waiting).

Use this command to run a saved automation task, or attach to a task execution ID, and collect
per-device, per-script output. Do NOT use this command for a single ad-hoc script on one device;
use 'scripts run-wait' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli tasks run-wait 1234 --attach 10450 --failed-only --agent
  kaseya-vsa-cli tasks run-wait 1234 --devices FRONTDESK-01,FRONTDESK-02 --max-wait 30m --agent
  kaseya-vsa-cli tasks run-wait 1234 --task-scope --max-wait 1h --agent`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:typed-exit-codes": "0,6,8"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
					return writeDryRun(cmd.OutOrStdout(), flags, "tasks run-wait")
				}
				tid := strings.TrimSpace(args[0])
				if flagAttach != "" {
					return writeRunPreview(cmd.OutOrStdout(), flags, "tasks run-wait", "GET", "/automation/tasks/executions/"+strings.TrimSpace(flagAttach), nil,
						"poll until the execution ends, then collect each device's script output; nothing is started")
				}
				body := map[string]any{}
				if strings.TrimSpace(flagDevices) != "" {
					body["DeviceIdentifiers"] = strings.Split(flagDevices, ",")
				}
				if flagWebhook != "" {
					body["WebhookUrl"] = flagWebhook
				}
				then := "poll /automation/tasks/executions/{id}, then collect each device's script output (device names resolve at run time)"
				if len(body) == 0 || body["DeviceIdentifiers"] == nil {
					then += "; with no --devices the task runs on its whole configured scope (needs --task-scope)"
				}
				return writeRunPreview(cmd.OutOrStdout(), flags, "tasks run-wait", "POST", "/automation/tasks/"+tid+"/run", body, then)
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("task-id is required (see 'kaseya-vsa-cli tasks list')"))
			}
			taskID, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil {
				return usageErr(fmt.Errorf("task-id must be a number, got %q", args[0]))
			}
			var attachID int64
			if cmd.Flags().Changed("attach") && strings.TrimSpace(flagAttach) == "" {
				return usageErr(fmt.Errorf("--attach needs a task execution ID"))
			}
			if strings.TrimSpace(flagAttach) != "" {
				if attachID, err = strconv.ParseInt(strings.TrimSpace(flagAttach), 10, 64); err != nil || attachID <= 0 {
					return usageErr(fmt.Errorf("--attach must be a positive task execution ID, got %q", flagAttach))
				}
			}
			maxWait, err := cliutil.ParseDurationLoose(flagMaxWait)
			if err != nil || maxWait <= 0 {
				return usageErr(fmt.Errorf("invalid --max-wait %q (e.g. 30m)", flagMaxWait))
			}
			interval, err := cliutil.ParseDurationLoose(flagInterval)
			if err != nil || interval <= 0 {
				return usageErr(fmt.Errorf("invalid --interval %q (e.g. 5s)", flagInterval))
			}
			if concurrency < 1 {
				concurrency = 1
			}
			deviceArgs := make([]string, 0)
			for _, d := range strings.Split(flagDevices, ",") {
				if d = strings.TrimSpace(d); d != "" {
					deviceArgs = append(deviceArgs, d)
				}
			}
			if cmd.Flags().Changed("devices") && len(deviceArgs) == 0 {
				return usageErr(fmt.Errorf("--devices was given but names no device"))
			}
			if attachID == 0 && len(deviceArgs) == 0 && !taskScope {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass --devices to choose devices, or --task-scope to run on every device in the task's configured scope"))
			}
			if attachID == 0 && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "run automation task on managed devices")
			}
			ctx, cancel := waitContext(cmd.Context(), maxWait)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireTenantBaseURL(c); err != nil {
				return err
			}
			c.NoCache = true
			res := taskRunResult{TaskID: taskID, ExecutionID: attachID, Results: make([]taskScriptRow, 0), FetchFailures: make([]briefSectionFailure, 0)}
			if attachID == 0 {
				body := map[string]any{}
				if len(deviceArgs) > 0 {
					ids := make([]string, 0, len(deviceArgs))
					for _, d := range deviceArgs {
						id, _, err := resolveDeviceID(ctx, flags, c, d)
						if err != nil {
							return err
						}
						ids = append(ids, id)
					}
					body["DeviceIdentifiers"] = ids
				}
				if flagWebhook != "" {
					body["WebhookUrl"] = flagWebhook
				}
				raw, _, err := c.Post(ctx, fmt.Sprintf("/automation/tasks/%d/run", taskID), body)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				var started struct {
					ID int64 `json:"Id"`
				}
				if err := json.Unmarshal(vsaData(raw), &started); err != nil || started.ID == 0 {
					return apiErr(fmt.Errorf("run task: response carried no execution Id: %s", bodyForError(c, raw)))
				}
				res.ExecutionID = started.ID
				fmt.Fprintf(cmd.ErrOrStderr(), "started task execution %d; waiting up to %s\n", res.ExecutionID, maxWait)
			}
			rec, polls, timedOut, err := pollExecution(ctx, c, fmt.Sprintf("/automation/tasks/executions/%d", res.ExecutionID), interval)
			if err != nil {
				return err
			}
			res.Polls, res.TimedOut = polls, timedOut
			if rec != nil {
				res.State = fmt.Sprint(rec["State"])
				res.DeviceCount, res.SuccessfulCount, res.FailedCount, res.Duration = rec["DeviceCount"], rec["SuccessfulDeviceCount"], rec["FailedDeviceCount"], rec["Duration"]
				if got, ok := exitCodeValue(rec["TaskId"]); ok && int64(got) != taskID {
					return usageErr(fmt.Errorf("execution %d belongs to task %d, not task %d", res.ExecutionID, got, taskID))
				}
			}
			anyFailed := false
			// A finished execution that did not succeed (Failed, Stopped) is a failure even when it
			// reports zero failed devices.
			if !timedOut && rec != nil && executionDone(rec) && !strings.EqualFold(res.State, "Successful") {
				anyFailed = true
			}
			if !timedOut {
				collectedFailed, err := collectTaskResults(ctx, c, &res, failedOnly, maxDevices, concurrency)
				if err != nil {
					return err
				}
				anyFailed = anyFailed || collectedFailed
			}
			if failed, ok := exitCodeValue(res.FailedCount); ok && failed > 0 {
				anyFailed = true
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), res, flags); err != nil {
					return err
				}
			} else {
				printTaskRun(cmd, res)
			}
			switch {
			case timedOut:
				return runTimeoutErr(fmt.Errorf("task execution %d still %s after %s; re-run with --attach %d", res.ExecutionID, res.State, maxWait, res.ExecutionID))
			case anyFailed:
				return runFailedErr(fmt.Errorf("task execution %d ended %s with failed devices", res.ExecutionID, res.State))
			case len(res.FetchFailures) > 0:
				return apiErr(fmt.Errorf("%d result fetches failed; see fetch_failures", len(res.FetchFailures)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDevices, "devices", "", "Comma-separated device names or Identifiers to run on (default: the task's configured scope)")
	cmd.Flags().StringVar(&flagAttach, "attach", "", "Collect this existing task execution ID instead of starting a new run")
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "Only collect devices whose execution failed")
	cmd.Flags().BoolVar(&taskScope, "task-scope", false, "Run on every device in the task's configured scope (required when --devices is omitted)")
	cmd.Flags().StringVar(&flagWebhook, "webhook-url", "", "Optional URL VSA 10 POSTs to as each device finishes")
	cmd.Flags().StringVar(&flagMaxWait, "max-wait", "30m", "Stop waiting after this long (the task keeps running; exit code 8)")
	cmd.Flags().StringVar(&flagInterval, "interval", "5s", "Time between execution status polls")
	cmd.Flags().IntVar(&maxDevices, "max-devices", 200, "Maximum devices to collect per-script output for (each costs at least two API calls)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Parallel device result fetches")
	return cmd
}

type taskDevice struct {
	Identifier string `json:"Identifier"`
	Name       string `json:"Name"`
	State      string `json:"ExecutionState"`
	Status     string `json:"ExecutionStatus"` // the doc's field table names it this; the example uses ExecutionState
}

// collectTaskResults pages the execution's devices (bounded by maxDevices) and
// fetches each device's scripts and their output with bounded concurrency.
// Per-fetch failures are kept in FetchFailures, never as zero-valued rows.
func collectTaskResults(ctx context.Context, c *client.Client, res *taskRunResult, failedOnly bool, maxDevices, concurrency int) (bool, error) {
	base := fmt.Sprintf("/automation/tasks/executions/%d/devices", res.ExecutionID)
	devices := make([]taskDevice, 0)
	truncated := false
	for skip := 0; ; skip += 100 {
		params := map[string]string{"$top": "100", "$skip": strconv.Itoa(skip)}
		if failedOnly {
			params["includeSuccessful"] = "false"
		}
		data, err := vsaGet(ctx, c, base, params)
		if err != nil {
			return false, classifyAPIErrorOnly(err)
		}
		var page []taskDevice
		if err := json.Unmarshal(data, &page); err != nil {
			return false, fmt.Errorf("decode task devices: %w", err)
		}
		devices = append(devices, page...)
		if maxDevices > 0 && len(devices) >= maxDevices {
			truncated = len(devices) > maxDevices || len(page) == 100
			if len(devices) > maxDevices {
				devices = devices[:maxDevices]
			}
			break
		}
		if len(page) < 100 {
			break
		}
	}
	res.DevicesScanned = len(devices)
	if truncated {
		res.Note = fmt.Sprintf("collected the first %d devices; raise --max-devices to collect more", maxDevices)
	}
	anyFailed := false
	type deviceResult struct {
		rows     []taskScriptRow
		failures []briefSectionFailure
	}
	states := make([]string, len(devices))
	for i, d := range devices {
		states[i] = d.State
		if states[i] == "" {
			states[i] = d.Status
		}
		switch strings.ToLower(states[i]) {
		case "failed", "unresponsive", "stopped":
			anyFailed = true
		}
	}
	out := make([]deviceResult, len(devices))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ { // fixed worker pool: goroutines stay bounded however many devices
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i].rows, out[i].failures = collectDeviceScripts(ctx, c, base, devices[i], states[i])
			}
		}()
	}
	for i := range devices {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, r := range out {
		for _, row := range r.rows {
			if (row.Successful != nil && !*row.Successful) || (row.ExitCode != nil && *row.ExitCode != 0) {
				anyFailed = true
			}
		}
		res.Results = append(res.Results, r.rows...)
		res.FetchFailures = append(res.FetchFailures, r.failures...)
	}
	sort.SliceStable(res.Results, func(i, j int) bool {
		if res.Results[i].Device != res.Results[j].Device {
			return res.Results[i].Device < res.Results[j].Device
		}
		return res.Results[i].Script < res.Results[j].Script
	})
	return anyFailed, nil
}

func collectDeviceScripts(ctx context.Context, c *client.Client, base string, d taskDevice, state string) ([]taskScriptRow, []briefSectionFailure) {
	rows := make([]taskScriptRow, 0)
	failures := make([]briefSectionFailure, 0)
	data, err := vsaGet(ctx, c, base+"/"+d.Identifier+"/scripts", nil)
	if err != nil {
		return rows, append(failures, briefSectionFailure{Section: d.Name + " scripts", Error: failureText(c, err)})
	}
	var scripts []struct {
		ID           string `json:"Id"`
		Name         string `json:"Name"`
		IsSuccessful *bool  `json:"IsSuccessful"`
	}
	if err := json.Unmarshal(data, &scripts); err != nil {
		return rows, append(failures, briefSectionFailure{Section: d.Name + " scripts", Error: failureText(c, err)})
	}
	for _, s := range scripts {
		row := taskScriptRow{DeviceID: d.Identifier, Device: d.Name, DeviceState: state, ScriptID: s.ID, Script: s.Name, Successful: s.IsSuccessful}
		// Keep the row even when its output cannot be fetched: IsSuccessful alone still decides failure.
		outData, err := vsaGet(ctx, c, base+"/"+d.Identifier+"/scripts/"+s.ID, nil)
		if err != nil {
			failures = append(failures, briefSectionFailure{Section: d.Name + " / " + s.Name + " output", Error: failureText(c, err)})
			rows = append(rows, row)
			continue
		}
		var o map[string]any
		if err := json.Unmarshal(outData, &o); err != nil {
			failures = append(failures, briefSectionFailure{Section: d.Name + " / " + s.Name + " output", Error: failureText(c, err)})
			rows = append(rows, row)
			continue
		}
		if code, ok := exitCodeValue(o["ExitCode"]); ok {
			row.ExitCode = &code
		}
		row.Output, _ = o["Output"].(string)
		rows = append(rows, row)
	}
	return rows, failures
}

func printTaskRun(cmd *cobra.Command, r taskRunResult) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "task %d execution %d: %s (devices=%v ok=%v failed=%v, %d polls)\n", r.TaskID, r.ExecutionID, r.State, r.DeviceCount, r.SuccessfulCount, r.FailedCount, r.Polls)
	for _, row := range r.Results {
		fmt.Fprintf(w, "  %-24s %-10s %-24s exit=%s %s\n", row.Device, row.DeviceState, row.Script, intPtrString(row.ExitCode), strings.TrimSpace(row.Output))
	}
	for _, f := range r.FetchFailures {
		fmt.Fprintf(w, "  FAILED %s: %s\n", f.Section, f.Error)
	}
	if r.Note != "" {
		fmt.Fprintln(w, r.Note)
	}
}
