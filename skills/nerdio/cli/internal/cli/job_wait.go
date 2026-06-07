// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: job wait (poll an NMM async job to terminal state).
// pp:data-source live

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"nerdio-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func newNovelJobWaitCmd(flags *rootFlags) *cobra.Command {
	var flagInterval string
	var flagTimeout string

	cmd := &cobra.Command{
		Use:   "wait <job_id>",
		Short: "Wait for an NMM async job to reach a terminal state",
		Long: strings.Trim(`
Polls GET /rest-api/v1/job/{job_id} on a throttled interval until the job
reaches a terminal state (Completed, CompletedWithErrors, Failed, Cancelled),
then exits with a code reflecting the outcome: 0 when the job completed
successfully, 5 when it failed, was cancelled, or completed with errors.

Every NMM mutation (provisioning, scripted actions, backup, host power ops)
returns a job ID; callers that never poll appear to silently fail. This
command is that poll loop as one composable primitive. Transient fetch
errors (429s, network blips) are tolerated up to 3 consecutive misses.
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli job wait 4821
  nerdio-cli job wait 4821 --interval 10s --timeout 30m --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:typed-exit-codes": "0,2,5",
			"pp:happy-args":       "job_id=4821",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would poll job to terminal state")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("job_id is required"))
			}
			jobID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("job_id must be numeric, got %q", args[0]))
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
			outcome, waitErr := waitForJob(cmd.Context(), c, jobID, interval, timeout)
			if printErr := printJSONFiltered(cmd.OutOrStdout(), outcome, flags); printErr != nil {
				return printErr
			}
			if waitErr != nil {
				return apiErr(waitErr)
			}
			if outcome.Terminal && !outcome.Succeeded {
				return apiErr(fmt.Errorf("job %d ended %s", jobID, outcome.Status))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagInterval, "interval", "10s", "Poll interval (community-recommended 10s throttle; accepts 30s/1m/etc)")
	cmd.Flags().StringVar(&flagTimeout, "timeout", "30m", "Give up after this long if the job is still running")
	return cmd
}
