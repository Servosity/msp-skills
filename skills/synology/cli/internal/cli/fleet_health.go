// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet health: storage problems across every NAS.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var fullPct float64
	var diskTemp int64
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Degraded volumes and pools, bad SMART disks, hot disks and nearly full volumes across every NAS",
		Long: strings.Trim(`
Use this command for volume, pool and disk problems across every NAS. Do NOT
use this command for one NAS's live CPU/memory/system health; use 'system
health-check' instead.

Reads each NAS's newest storage snapshot from the mirror. Any status other than
normal is reported (unknown values as warnings, never as ok). Nearly full
volumes carry days_to_full when 2+ snapshots a day apart exist. Exit code 6
when any issue is found; --no-fail exits 0.`, "\n"),
		Example:     "  synology-cli fleet health --full-pct 85 --disk-temp 55 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if err := checkLimits(fullPct, 0, diskTemp); err != nil {
				return err
			}
			return r.run(cmd, flags, "no storage issues across %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					rows, err := fleet.Health(ctx, db, nas, fullPct, diskTemp, time.Now(), maxSyncAge)
					return rows, len(rows), err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().Float64Var(&fullPct, "full-pct", 85, "Volume used percentage that counts as nearly full (0 disables)")
	cmd.Flags().Int64Var(&diskTemp, "disk-temp", 55, "Disk temperature in Celsius that counts as hot (0 disables)")
	return cmd
}
