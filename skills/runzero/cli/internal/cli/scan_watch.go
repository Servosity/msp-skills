// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/cliutil"
)

// scanWatchResult is the final envelope scan-watch prints once the task
// reaches a terminal state (or the watch times out / is curtailed).
type scanWatchResult struct {
	SiteID    string          `json:"site_id"`
	TaskID    string          `json:"task_id"`
	Status    string          `json:"status"`
	Done      bool            `json:"done"`
	Polls     int             `json:"polls"`
	Elapsed   string          `json:"elapsed"`
	Task      json.RawMessage `json:"task,omitempty"`
	Curtailed bool            `json:"curtailed,omitempty"`
}

// pp:data-source live
func newNovelScanWatchCmd(flags *rootFlags) *cobra.Command {
	var targets string
	var scanName string
	var rate string
	var intervalFlag string
	var timeoutFlag string

	cmd := &cobra.Command{
		Use:   "scan-watch <site_id>",
		Short: "Start a scan on a site and follow the task to completion in one command, with a typed exit code on the result.",
		Long: strings.Trim(`
Use this command to start a scan on a site and block until it finishes. Do NOT
use it to merely enqueue without watching; use 'org create-scan' instead. Do
NOT use it to inspect an already-running task; use 'org get-task' / 'org
get-task-log' instead.

Issues PUT /org/sites/{site_id}/scan with the given targets, then polls the
created task until it reaches a terminal state (processed, stopped, canceled,
failed, or error), streaming each status change to stderr. Exits 0 when the
scan is processed; a failed, canceled, or error task returns a non-zero exit.

Requires an Organization (OT) or Account (CT) API token with scan permission.`, "\n"),
		Example: strings.Trim(`
  # Scan a subnet on a site and wait for the result
  runzero-cli scan-watch 550e8400-e29b-41d4-a716-446655440000 --targets 10.0.0.0/24

  # Named scan with a bounded watch
  runzero-cli scan-watch 550e8400-e29b-41d4-a716-446655440000 --targets 10.0.0.5 --scan-name nightly --timeout 10m --json`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args": "<site_id>=550e8400-e29b-41d4-a716-446655440000;--targets=192.0.2.10",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would start a scan and watch the task to completion")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<site_id> is required"))
			}
			if strings.TrimSpace(targets) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--targets is required"))
			}
			// Scan-watch mutates external state (it launches a real scan);
			// keep verify runs side-effect free.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would scan site %s targets %s and watch the task\n", args[0], targets)
				return nil
			}
			interval := 5 * time.Second
			if strings.TrimSpace(intervalFlag) != "" {
				d, err := cliutil.ParseDurationLoose(intervalFlag)
				if err != nil {
					return fmt.Errorf("invalid --interval %q: %w", intervalFlag, err)
				}
				if d > 0 {
					interval = d
				}
			}
			watchTimeout := 30 * time.Minute
			if strings.TrimSpace(timeoutFlag) != "" {
				d, err := cliutil.ParseDurationLoose(timeoutFlag)
				if err != nil {
					return fmt.Errorf("invalid --timeout %q: %w", timeoutFlag, err)
				}
				if d > 0 {
					watchTimeout = d
				}
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			siteID := strings.TrimSpace(args[0])
			body := map[string]string{"targets": targets}
			if strings.TrimSpace(scanName) != "" {
				body["scan-name"] = scanName
			}
			if strings.TrimSpace(rate) != "" {
				body["rate"] = rate
			}
			path := "/org/sites/" + siteID + "/scan"
			data, _, err := c.Put(cmd.Context(), path, body)
			if err != nil {
				return fmt.Errorf("starting scan on site %s: %w", siteID, err)
			}
			var task struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(data, &task); err != nil || strings.TrimSpace(task.ID) == "" {
				return fmt.Errorf("scan started but the response had no task id; inspect with 'org get-tasks' (raw: %.200s)", string(data))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "scan task %s created (status %s); watching every %s\n", task.ID, orDash(task.Status), interval)

			res := scanWatchResult{SiteID: siteID, TaskID: task.ID, Status: task.Status}
			start := time.Now()
			deadline := start.Add(watchTimeout)
			lastStatus := task.Status
			// Live dogfood runs a flat per-command timeout; poll once and
			// return the in-flight status instead of watching to completion.
			maxPolls := -1
			if cliutil.IsDogfoodEnv() {
				maxPolls = 1
			}
			for {
				if maxPolls >= 0 && res.Polls >= maxPolls {
					res.Curtailed = true
					break
				}
				if time.Now().After(deadline) {
					res.Curtailed = true
					fmt.Fprintf(cmd.ErrOrStderr(), "watch timed out after %s; the scan continues server-side (task %s)\n", watchTimeout, task.ID)
					break
				}
				select {
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				case <-time.After(interval):
				}
				data, err := c.GetNoCache(cmd.Context(), "/org/tasks/"+task.ID, nil)
				if err != nil {
					return fmt.Errorf("polling task %s: %w", task.ID, err)
				}
				res.Polls++
				var t struct {
					Status string `json:"status"`
				}
				if err := json.Unmarshal(data, &t); err != nil {
					return fmt.Errorf("parsing task %s status: %w", task.ID, err)
				}
				res.Task = data
				res.Status = t.Status
				if t.Status != lastStatus {
					fmt.Fprintf(cmd.ErrOrStderr(), "task %s: %s -> %s (%s elapsed)\n", task.ID, orDash(lastStatus), t.Status, time.Since(start).Round(time.Second))
					lastStatus = t.Status
				}
				if isTerminalTaskStatus(t.Status) {
					res.Done = true
					break
				}
			}
			res.Elapsed = time.Since(start).Round(time.Second).String()
			if err := flags.printJSON(cmd, res); err != nil {
				return err
			}
			// Gate on the observed status, not res.Done: a curtailed watch
			// (dogfood single-poll or timeout) that nonetheless saw a
			// terminal failure must still exit non-zero.
			if isTerminalTaskStatus(res.Status) && !strings.EqualFold(res.Status, "processed") {
				return fmt.Errorf("scan task %s ended with status %q", task.ID, res.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&targets, "targets", "", "Scan targets: IPs, CIDRs, or hostnames (comma- or space-separated, passed to the scan task)")
	cmd.Flags().StringVar(&scanName, "scan-name", "", "Optional name for the scan task")
	cmd.Flags().StringVar(&rate, "rate", "", "Optional packet rate for the scan (e.g. 1000)")
	cmd.Flags().StringVar(&intervalFlag, "interval", "", "Poll interval while watching the task (e.g. 5s, 30s; default 5s)")
	cmd.Flags().StringVar(&timeoutFlag, "timeout", "", "Maximum time to watch before returning with the scan still running (e.g. 10m, 1h; default 30m)")
	return cmd
}

// isTerminalTaskStatus reports whether a runZero task status means the task
// will make no further progress.
func isTerminalTaskStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "processed", "stopped", "canceled", "cancelled", "failed", "error":
		return true
	}
	return false
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
