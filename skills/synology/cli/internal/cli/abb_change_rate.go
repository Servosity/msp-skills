// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb change-rate: per-device data-volume anomalies.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbChangeRateCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var since string
	var spike float64
	var flat int
	cmd := &cobra.Command{
		Use:   "change-rate",
		Short: "Devices whose newest good backup moved far more data than their own median, or nothing for several runs",
		Long: strings.Trim(`
Use this command for devices whose newest backup moved far more data than
usual or nothing at all. Do NOT use this command for run duration or
success-rate trends; use 'abb trend' instead.

Reads only the local fleet mirror (run 'fleet sync' first). For each device,
the newest good backup (success or partial) in --since is compared with the
median of its earlier good runs in the window; a device with fewer than 5
earlier runs is not judged. Runs whose byte count DSM did not report are
skipped; 0 bytes is a real value. spike: the newest run moved more than
--spike times the median. flat: the newest --flat-runs runs moved 0 bytes
against a median above 0. nas_spike_count tells one machine apart from a
fleet-wide patch night. A NAS whose mirror is not fresh is listed as
nas_not_synced. The byte counts are DSM's transfer sizes and have not been
checked against a live NAS. Exit code 6 when any row other than
abb_not_installed is returned.`, "\n"),
		Example:     "  synology-cli abb change-rate --since 30d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6", "pp:happy-args": "--no-fail"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb change-rate")
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			if math.IsNaN(spike) || math.IsInf(spike, 0) || spike < 0 {
				return usageErr(fmt.Errorf("--spike must be a finite number of 0 or more, got %v", spike))
			}
			if flat < 0 {
				return usageErr(fmt.Errorf("--flat-runs must be 0 or more, got %d", flat))
			}
			return r.run(cmd, flags, "no device on %d NAS moved an unusual amount of data",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					now := time.Now()
					rows, err := fleet.ChangeRate(ctx, db, nas, now.Add(-win), spike, flat, now, maxSyncAge)
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
	cmd.Flags().StringVar(&since, "since", "30d", "History window, for example 14d or 30d")
	cmd.Flags().Float64Var(&spike, "spike", 10, "Flag a backup that moved more than this multiple of the device's median (0 turns the check off)")
	cmd.Flags().IntVar(&flat, "flat-runs", 3, "Flag a device whose newest this-many good backups moved 0 bytes (0 turns the check off)")
	return cmd
}
