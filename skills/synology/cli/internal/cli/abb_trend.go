// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb trend: success rate and duration overruns from history.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbTrendCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var since, by string
	var overrun float64
	cmd := &cobra.Command{
		Use:   "trend",
		Short: "Per task or device: runs, success rate, median and p90 duration, and overruns",
		Long: strings.Trim(`
Use this command for success rate and run-duration trends over a window. Do
NOT use this command for what failed in the latest run; use 'abb failures'
instead.

overrun is true when the newest completed run took more than --overrun times
the median of at least 3 runs, and null (never false) when that cannot be
judged: the newest run has no end time, fewer than 3 runs have one, or
--overrun 0. median_duration_sec and p90_duration_sec are null, with a note,
when no run in the window has an end time. Rows are sorted overruns first,
then lowest success rate. A NAS whose mirror is not fresh adds a
nas_not_synced row (exit 6).`, "\n"),
		Example:     "  synology-cli abb trend --since 30d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb trend")
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			if by != "task" && by != "device" {
				return usageErr(fmt.Errorf("--by must be task or device"))
			}
			if err := checkLimits(0, overrun, 0); err != nil {
				return err
			}
			return r.run(cmd, flags, "no backup runs in the window on %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					now := time.Now()
					rows, err := fleet.Trend(ctx, db, nas, now.Add(-win), by == "device", overrun, now, maxSyncAge)
					unsynced := 0
					for _, t := range rows {
						if t.Kind == "nas_not_synced" || t.Kind == "abb_missing" {
							unsynced++
						}
					}
					return rows, unsynced, err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().StringVar(&since, "since", "30d", "History window, for example 7d or 30d")
	cmd.Flags().StringVar(&by, "by", "task", "Group by task or device")
	cmd.Flags().Float64Var(&overrun, "overrun", 2, "Flag a run longer than this multiple of the median")
	return cmd
}
