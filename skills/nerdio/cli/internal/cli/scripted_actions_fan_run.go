// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: scripted-actions fan-run (execute one action across accounts).
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"nerdio-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// fanRunResult is one account's execution outcome.
type fanRunResult struct {
	AccountID int64  `json:"account_id"`
	Account   string `json:"account"`
	JobID     int64  `json:"job_id,omitempty"`
	Status    string `json:"status"`
	Succeeded *bool  `json:"succeeded,omitempty"` // populated only with --wait
}

// fanRunView is the JSON envelope for scripted-actions fan-run.
type fanRunView struct {
	ScriptID      int64               `json:"script_id"`
	Results       []fanRunResult      `json:"results"`
	Dispatched    int                 `json:"dispatched"`
	Failed        int                 `json:"failed"`
	JobFailures   int                 `json:"job_failures"`
	FetchFailures []fleetFetchFailure `json:"fetch_failures,omitempty"`
	Note          string              `json:"note,omitempty"`
}

// countJobFailures tallies results whose --wait outcome reached a terminal
// state that was NOT a success (Failed/Cancelled/CompletedWithErrors). A
// dispatch that succeeded but whose job failed must not read as clean.
func countJobFailures(results []fanRunResult) int {
	n := 0
	for _, r := range results {
		if r.Succeeded != nil && !*r.Succeeded {
			n++
		}
	}
	return n
}

func newNovelScriptedActionsFanRunCmd(flags *rootFlags) *cobra.Command {
	var flagAccounts string
	var flagWait bool
	var flagInterval string
	var flagTimeout string
	var flagParams string
	var flagMinutesToWait int

	cmd := &cobra.Command{
		Use:   "fan-run <script_id>",
		Short: "Execute one scripted action across many customer accounts",
		Long: strings.Trim(`
Use this command to run an existing scripted action across multiple accounts.
Do NOT use it to poll a single already-returned job ID; use 'job wait' instead.

Fans out POST /accounts/{id}/scripted-actions/{script}/execution across the
given accounts, collects each returned job ID, and with --wait polls every
job to a terminal state. Per-account outcomes are aggregated; accounts whose
dispatch failed are reported separately in fetch_failures, never silently
dropped. This is a write operation - it executes the action for real on every
targeted account, so --accounts is required (no implicit all-accounts sweep).
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli scripted-actions fan-run 42 --accounts 101,102,103
  nerdio-cli scripted-actions fan-run 42 --accounts 101,102 --wait --agent
`, "\n"),
		Annotations: map[string]string{
			"pp:typed-exit-codes": "0,2,5,6",
			"pp:happy-args":       "script_id=42;--accounts=101",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would execute the scripted action across the targeted accounts")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("script_id is required"))
			}
			scriptID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("script_id must be numeric, got %q", args[0]))
			}
			accounts, err := parseAccountsCSV(flagAccounts)
			if err != nil {
				return usageErr(err)
			}
			if len(accounts) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--accounts is required: fan-run executes for real, so the target set must be explicit"))
			}
			var paramsBindings map[string]any
			if flagParams != "" {
				if err := json.Unmarshal([]byte(flagParams), &paramsBindings); err != nil {
					return usageErr(fmt.Errorf("--params must be a JSON object of parameter bindings: %w", err))
				}
			}
			interval, err := cliutil.ParseDurationLoose(flagInterval)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --interval: %w", err))
			}
			timeout, err := cliutil.ParseDurationLoose(flagTimeout)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --timeout: %w", err))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if cliutil.IsDogfoodEnv() && len(accounts) > 1 {
				accounts = accounts[:1] // live-dogfood: one account, no fleet-wide writes
			}

			body := map[string]any{
				"adConfigId":    nil,
				"minutesToWait": flagMinutesToWait,
			}
			if paramsBindings != nil {
				body["paramsBindings"] = paramsBindings
			}

			results, errs := cliutil.FanoutRun(cmd.Context(), accounts,
				func(a fleetAccount) string { return fmt.Sprintf("%s (%d)", a.Name, a.ID) },
				func(ctx context.Context, a fleetAccount) (fanRunResult, error) {
					out := fanRunResult{AccountID: a.ID, Account: a.Name, Status: "dispatched"}
					data, _, err := c.Post(ctx, fmt.Sprintf("/rest-api/v1/accounts/%d/scripted-actions/%d/execution", a.ID, scriptID), body)
					if err != nil {
						return out, fmt.Errorf("dispatching scripted action: %w", err)
					}
					objs := decodeObjects(data)
					if len(objs) > 0 {
						if jobID, ok := extractIntAny(objs[0], "id", "jobId", "job"); ok {
							out.JobID = jobID
						}
					}
					if flagWait && out.JobID > 0 {
						outcome, waitErr := waitForJob(ctx, c, out.JobID, interval, timeout)
						out.Status = outcome.Status
						succeeded := outcome.Succeeded
						out.Succeeded = &succeeded
						if waitErr != nil {
							return out, fmt.Errorf("waiting for job %d: %w", out.JobID, waitErr)
						}
					}
					return out, nil
				},
				cliutil.WithConcurrency(4))

			view := fanRunView{ScriptID: scriptID, Results: make([]fanRunResult, 0), FetchFailures: fanoutFailures(errs)}
			for _, r := range results {
				view.Results = append(view.Results, r.Value)
				view.Dispatched++
			}
			view.Failed = len(view.FetchFailures)
			if view.Failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d account dispatches failed; results cover the remaining %d accounts\n",
					view.Failed, len(accounts), view.Dispatched)
			}
			if flagWait {
				view.Note = "statuses reflect terminal job states; run 'nerdio-cli job tasks <job_id>' for per-task detail"
			} else if view.Dispatched > 0 {
				view.Note = "jobs dispatched asynchronously; run 'nerdio-cli job wait <job_id>' to poll each to completion"
			}
			view.JobFailures = countJobFailures(view.Results)
			if view.JobFailures > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d job(s) reached a failed terminal state\n", view.JobFailures)
			}
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			if view.Failed > 0 && view.Dispatched == 0 {
				return apiErr(fmt.Errorf("all %d account dispatches failed", view.Failed))
			}
			if view.Failed > 0 || view.JobFailures > 0 {
				return partialFailureErr(fmt.Errorf("%d of %d dispatches failed; %d job(s) ended in a failed state", view.Failed, len(accounts), view.JobFailures))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagAccounts, "accounts", "", "CSV of NMM account IDs to execute in (required - writes are never implicit)")
	cmd.Flags().BoolVar(&flagWait, "wait", false, "Poll every returned job to a terminal state before exiting")
	cmd.Flags().StringVar(&flagInterval, "interval", "10s", "Poll interval used with --wait")
	cmd.Flags().StringVar(&flagTimeout, "timeout", "30m", "Per-job timeout used with --wait")
	cmd.Flags().StringVar(&flagParams, "params", "", "JSON object of script parameter bindings (paramsBindings)")
	cmd.Flags().IntVar(&flagMinutesToWait, "minutes-to-wait", 30, "minutesToWait value sent with the execution request")
	return cmd
}
