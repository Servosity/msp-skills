// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb unprotected: devices and tasks that dropped out of protection.
// pp:data-source local

package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbUnprotectedCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var since string
	cmd := &cobra.Command{
		Use:   "unprotected",
		Short: "Devices and tasks that dropped out of Active Backup protection across the fleet",
		Long: strings.Trim(`
Use this command for machines and tasks that dropped out of protection
(removed from a task, task deleted, schedule off or paused). Do NOT use this
command for machines still in a task whose last good backup is old; use
'abb stale' instead.

Reads only the local fleet mirror (run 'fleet sync' first). Rows:
device_dropped (a device backed up in the window that no current task
protects), task_deleted (a task that backed up in the window and no longer
exists) and task_unscheduled (a task with no schedule that still holds
devices). A device now protected under another device id with the same host
name is named in the note, never merged. A NAS whose mirror is not fresh is
listed as nas_not_synced; abb_missing means ABB ran there before and no
longer answers. When the window starts before the mirrored history the rows
say so (partial_history). last_run_at and last_success are null, never 0,
when there is no such run. Exit code 6 when any row other than
abb_not_installed or partial_history is returned.`, "\n"),
		Example:     "  synology-cli abb unprotected --since 30d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,6", "pp:happy-args": "--no-fail"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb unprotected")
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			return r.run(cmd, flags, "no device or task dropped out of protection on %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					now := time.Now()
					rows, err := fleet.Unprotected(ctx, db, nas, now.Add(-win), now, maxSyncAge)
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
	cmd.Flags().StringVar(&since, "since", "30d", "Look for devices and tasks that backed up in this window, for example 7d or 30d")
	return cmd
}
