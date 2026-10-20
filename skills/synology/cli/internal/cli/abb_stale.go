// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb stale: protected devices without a recent good backup.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbStaleCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var olderThan string
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Protected PCs, servers and VMs whose last successful backup is older than a threshold, or that never succeeded",
		Long: strings.Trim(`
Use this command for protected devices with no successful backup in N hours.
Do NOT use this command for last night's failed task runs; use 'abb failures'
instead.

Last success is computed per device from mirrored per-device runs and backup
results (a partial success counts: it produced a restore point). A running
task does not hide a device (running is shown; the age decides); a NAS whose
mirror is not fresh is listed as nas_not_synced; abb_missing means ABB ran there before and no longer answers. Exit code 6 when any row other than abb_not_installed is returned.`, "\n"),
		Example:     "  synology-cli abb stale --older-than 36h --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb stale")
			}
			d, err := parseWindow("older-than", olderThan)
			if err != nil {
				return err
			}
			return r.run(cmd, flags, "every protected device on %d NAS has a recent good backup",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					rows, err := fleet.Stale(ctx, db, nas, d, time.Now(), maxSyncAge)
					n := 0
					for _, row := range rows {
						if row.Kind != "abb_not_installed" {
							n++
						}
					}
					return rows, n, err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().StringVar(&olderThan, "older-than", "36h", "Flag devices whose last good backup is older than this, for example 36h or 3d")
	return cmd
}
