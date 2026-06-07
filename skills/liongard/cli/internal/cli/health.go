// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelHealthCmd implements `health` — the one-shot estate scorecard:
// stale launchpoints, offline agents, failed inspections, and coverage gaps
// folded into a single summary with an optional typed exit code for cron.
// pp:data-source local
func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan string
	var flagSince string
	var flagFailOnIssues bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "One estate-wide health scorecard: stale launchpoints, offline agents, failed inspections, and coverage gaps",
		Long: `Use this command for a single estate-wide health SUMMARY (counts + optional
typed exit code) suitable for cron. It folds the stale-launchpoint sweep,
agent-offline sweep, inspection-failure sweep, and coverage reconcile into one
local-store rollup. Run 'liongard-cli sync' first.

Do NOT use this command to list individual stale collectors or offline agents;
use 'launchpoints stale', 'agents offline', or 'detections failures' for the
detail rows.

Exit codes: 0 = summary produced (default). With --fail-on-issues: 0 = healthy,
8 = one or more issues found.`,
		Example: strings.Trim(`
  # The morning glance: whole-estate health in one line of JSON
  liongard-cli health --agent

  # Cron guard: non-zero exit (8) when anything needs attention
  liongard-cli health --fail-on-issues --quiet
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:typed-exit-codes": "0,8",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compute estate health rollup from local store")
				return nil
			}
			olderThan, err := parseLookbackDuration(flagOlderThan)
			if err != nil {
				return err
			}
			var failSince time.Duration
			if flagSince != "" {
				d, err := parseLookbackDuration(flagSince)
				if err != nil {
					return err
				}
				failSince = d
			}

			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			stale, err := computeStaleLaunchpoints(db, olderThan, "")
			if err != nil {
				return err
			}
			agents, err := loadObjs(db, rtAgents)
			if err != nil {
				return err
			}
			systems, err := loadObjs(db, rtSystems)
			if err != nil {
				return err
			}
			timeline, err := loadObjs(db, rtTimeline)
			if err != nil {
				return err
			}
			launchpoints, err := loadObjs(db, rtLaunchpoints)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			offline := filterOfflineAgents(agents, envs, "")
			failed := filterFailedInspections(timeline, launchpoints, envs, failSince, now, "", 0)
			sysGaps, envGaps := findCoverageGaps(systems, envs, "")

			summary := summarizeHealth(len(stale), len(offline), len(failed), len(sysGaps), len(envGaps), flagOlderThan, flagSince)
			if err := printJSONFiltered(cmd.OutOrStdout(), summary, flags); err != nil {
				return err
			}
			if flagFailOnIssues && summary.TotalIssues > 0 {
				return &cliError{code: 8, err: fmt.Errorf("estate has %d issue(s): %d stale launchpoints, %d offline agents, %d failed inspections, %d uninspected systems, %d empty environments",
					summary.TotalIssues, summary.StaleLaunchpoints, summary.OfflineAgents, summary.FailedInspections, summary.UninspectedSystems, summary.EmptyEnvironments)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "7d", "Staleness threshold for launchpoints (e.g. 24h, 7d)")
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Window for the failed-inspection count (e.g. 24h, 7d); empty = all synced history")
	cmd.Flags().BoolVar(&flagFailOnIssues, "fail-on-issues", false, "Exit 8 when any issue is found (for cron/CI guards)")
	return cmd
}
