// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet capacity: every volume's runway from retained snapshots.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelFleetCapacityCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var horizon string
	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "Every volume on every NAS with used percent, growth per day and days until full, soonest first",
		Long: strings.Trim(`
Use this command for every volume's used percentage, growth per day and days
until full, soonest first. Do NOT use this command for volumes, pools or
disks that already have a problem; use 'fleet health' instead.

Reads only the local fleet mirror (run 'fleet sync' first). Growth is the
slope between the oldest storage snapshot with known capacity in the last 30
days and the newest one (the rule fleet health uses for days_to_full), so it
needs snapshots at least a day apart. growth_bytes_per_day and days_to_full
are null, never 0, when that history is missing; days_to_full is null for a
flat or shrinking volume. within_horizon is true when a volume fills within
--horizon. A NAS whose mirror is not fresh is listed as nas_not_synced, and
one whose last sync left no storage snapshot as storage_snapshot. Exit code 6
when a volume fills within --horizon, a volume's capacity is unknown, or a
NAS cannot be read.`, "\n"),
		Example:     "  synology-cli fleet capacity --horizon 30d\n  synology-cli fleet capacity --horizon 30d --csv",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6", "pp:happy-args": "--horizon=30d;--no-fail"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet capacity")
			}
			h, err := parseWindow("horizon", horizon)
			if err != nil {
				return err
			}
			return r.run(cmd, flags, "no volume on %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					rows, err := fleet.Capacity(ctx, db, nas, h, time.Now(), maxSyncAge)
					n := 0
					for _, row := range rows {
						if row.IsFinding() {
							n++
						}
					}
					return rows, n, err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().StringVar(&horizon, "horizon", "30d", "Flag volumes that fill within this window, for example 30d or 90d")
	return cmd
}
