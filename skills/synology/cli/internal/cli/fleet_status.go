// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet status: one row per client NAS.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelFleetStatusCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var since, staleAfter string
	var fullPct float64
	var diskTemp int64
	cmd := &cobra.Command{
		Use:   "status",
		Short: "One row per client NAS: backup outcomes, stale devices, worst storage state and sync age",
		Long: strings.Trim(`
Use this command for a one-row-per-NAS answer to "is every client OK". Do NOT
use this command to list individual failed tasks or bad disks; use 'abb
failures' or 'fleet health' instead.

state is ok, warning, critical or unknown. A NAS that never synced, whose last
sync failed, or whose mirror is older than --max-sync-age is unknown, never ok.
Exit code 6 when any NAS is not ok (RMM-friendly); --no-fail exits 0.`, "\n"),
		Example:     "  synology-cli fleet status --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet status")
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			stale, err := parseWindow("stale-after", staleAfter)
			if err != nil {
				return err
			}
			if err := checkLimits(fullPct, 0, diskTemp); err != nil {
				return err
			}
			return r.run(cmd, flags, "no NAS units to report (%d checked)",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					now := time.Now()
					rows, err := fleet.Status(ctx, db, nas, now.Add(-win), stale, fullPct, diskTemp, now, maxSyncAge)
					n := 0
					for _, row := range rows {
						if row.State != fleet.SevOK {
							n++
						}
					}
					return rows, n, err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().StringVar(&since, "since", "26h", "Backup window: a scheduled task with no backup run in it counts as no-run")
	cmd.Flags().StringVar(&staleAfter, "stale-after", "36h", "A device with no successful backup for this long counts as stale")
	cmd.Flags().Float64Var(&fullPct, "full-pct", 85, "Volume used percentage that counts as nearly full")
	cmd.Flags().Int64Var(&diskTemp, "disk-temp", 55, "Disk temperature in Celsius that counts as hot")
	return cmd
}
