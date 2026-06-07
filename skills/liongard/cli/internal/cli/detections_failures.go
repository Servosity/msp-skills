// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelDetectionsFailuresCmd implements `detections failures` — every
// inspection that ran but FAILED or errored across the estate, attributed to
// its owning environment, from the local store.
// pp:data-source local
func newNovelDetectionsFailuresCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagEnv string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Every inspection that ran but failed or errored across the estate, joined to the owning environment",
		Long: `Use this command to find inspections that ran but FAILED/errored — a
different failure mode from collectors that simply stopped reporting.
Reads the locally synced timeline; run 'liongard-cli sync' first.

Do NOT use this command to find collectors with no recent run at all;
use 'launchpoints stale' instead.`,
		Example: strings.Trim(`
  # Failed or errored inspections in the last 7 days, estate-wide
  liongard-cli detections failures

  # Last 24h, one environment, agent-friendly JSON
  liongard-cli detections failures --since 24h --env 42 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would query local store for failed/errored inspections")
				return nil
			}
			var since time.Duration
			if flagSince != "" {
				d, err := parseLookbackDuration(flagSince)
				if err != nil {
					return err
				}
				since = d
			}

			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

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

			rows := filterFailedInspections(timeline, launchpoints, envs, since, time.Now().UTC(), flagEnv, flagLimit)
			result := map[string]any{
				"count":    len(rows),
				"failures": rows,
			}
			if flagSince != "" {
				result["since"] = flagSince
			}
			if len(rows) == 0 {
				result["note"] = "no failed or errored inspections in the synced timeline for this window; widen --since or run 'liongard-cli sync --resources timeline' to refresh"
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Time window to look back (e.g. 24h, 7d); empty = all synced history")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
