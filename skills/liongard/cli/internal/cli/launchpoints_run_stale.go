// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"liongard-pp-cli/internal/cliutil"
)

// newNovelLaunchpointsRunStaleCmd implements `launchpoints run-stale` — find
// the launchpoints whose newest inspection is older than --older-than (the same
// computation as `launchpoints stale`) and trigger an inspection run on each.
// Plan-only by default; --confirm performs the runs. Short-circuits under the
// verifier and --dry-run so it never makes API calls during verification.
// pp:data-source local
func newNovelLaunchpointsRunStaleCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan string
	var flagEnv string
	var flagConfirm bool

	cmd := &cobra.Command{
		Use:   "run-stale",
		Short: "Find stale launchpoints and trigger an inspection run on each, in one guarded command.",
		Long: `Composes 'launchpoints stale' with the launchpoint run endpoint: finds every
launchpoint whose newest inspection is older than --older-than and triggers a
fresh inspection on each. Prints the plan by default and makes no API calls;
pass --confirm to actually trigger the runs. Reads the locally synced store for
the stale set; run 'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # Preview which stale launchpoints would be re-run
  liongard-cli launchpoints run-stale --older-than 7d

  # Actually trigger the runs
  liongard-cli launchpoints run-stale --older-than 7d --confirm
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Verify / dry-run probes: exit 0 before any store or API access.
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would find stale launchpoints in the local store and plan inspection runs (no API calls)")
				return nil
			}
			olderThan, err := parseLookbackDuration(flagOlderThan)
			if err != nil {
				return err
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			stale, err := computeStaleLaunchpoints(db, olderThan, flagEnv)
			if err != nil {
				return err
			}

			// Belt-and-suspenders: never fire side effects under the verifier.
			if cliutil.IsVerifyEnv() {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"action":     "plan",
					"older_than": flagOlderThan,
					"would_run":  len(stale),
					"note":       "verify mode: no API calls made",
				}, flags)
			}

			if !flagConfirm {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"action":     "plan",
					"older_than": flagOlderThan,
					"would_run":  len(stale),
					"targets":    stale,
					"note":       "Plan only — re-run with --confirm to trigger inspection runs.",
				}, flags)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)

			type runResult struct {
				LaunchpointID string `json:"launchpoint_id"`
				Environment   string `json:"environment,omitempty"`
				OK            bool   `json:"ok"`
				Error         string `json:"error,omitempty"`
			}
			results := make([]runResult, 0, len(stale))
			triggered := 0
			for _, lp := range stale {
				path := replacePathParam("/launchpoints/{LaunchpointID}/run", "LaunchpointID", lp.LaunchpointID)
				rr := runResult{LaunchpointID: lp.LaunchpointID, Environment: lp.Environment}
				if _, err := c.Get(ctx, path, nil); err != nil {
					rr.Error = err.Error()
				} else {
					rr.OK = true
					triggered++
				}
				results = append(results, rr)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
				"action":     "run",
				"older_than": flagOlderThan,
				"attempted":  len(results),
				"triggered":  triggered,
				"results":    results,
			}, flags)
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "7d", "Staleness threshold (e.g. 24h, 7d, 90m)")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	cmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Actually trigger the inspection runs (default: plan only)")
	return cmd
}
