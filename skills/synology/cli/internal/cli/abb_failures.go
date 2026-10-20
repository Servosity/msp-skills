// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb failures: failed, partial and missed backups across the fleet.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbFailuresCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var since string
	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Failed, partial, canceled and missed ABB backups across every NAS in a time window",
		Long: strings.Trim(`
Use this command for which ABB task runs failed, partially succeeded or missed
in a window. Do NOT use this command for machines with no successful backup
over N hours; use 'abb stale' instead.

Rows: run_failed (the newest backup run in the window did not succeed; carries
error counts and the first error line), no_run_in_window (a scheduled task with
no backup run in the window; widen --since for weekly tasks), running_too_long
(a job running since before the window; it may be hung) and nas_not_synced (the
NAS's mirror is missing, failed or stale). A job that started inside the window
and is still running is skipped. Only backup jobs count: version deletions and restores are not
mistaken for backups. abb_missing means ABB ran on the NAS before and no longer answers. Exit code 6 when any row other than abb_not_installed is returned.`, "\n"),
		Example:     "  synology-cli abb failures --since 24h --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb failures")
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			return r.run(cmd, flags, "no failed or missed backups across %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					now := time.Now()
					rows, err := fleet.Failures(ctx, db, nas, now.Add(-win), now, maxSyncAge)
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
	cmd.Flags().StringVar(&since, "since", "24h", "Window to check, for example 24h, 36h or 7d")
	return cmd
}
