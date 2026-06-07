// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelSystemsHistoryCmd implements `systems history <system-id>` — the
// full chronological change story of one system: every inspection run
// (timeline) and detected change (detection) merged newest-first from the
// local store.
// pp:data-source local
func newNovelSystemsHistoryCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "history <system-id>",
		Short: "The full chronological change history of one system: every detection and inspection entry in time order",
		Long: `Use this command to see the full change history of ONE system over time —
inspections (timeline) and detected changes (detections) merged newest-first
from the locally synced store. Run 'liongard-cli sync' first.

Do NOT use this command for an estate-wide change feed in a window; use
'drift' instead. Do NOT use it for a single environment's current full
picture; use 'environments overview' instead.`,
		Example: strings.Trim(`
  # Everything Liongard ever recorded for system 4821, newest first
  liongard-cli systems history 4821

  # Only the last 30 days, as agent-friendly JSON
  liongard-cli systems history 4821 --since 30d --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would query local store for one system's merged timeline+detections history")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<system-id> is required"))
			}
			systemID := strings.TrimSpace(args[0])

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
			detections, err := loadObjs(db, rtDetections)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}

			events := filterSystemHistory(timeline, detections, envs, systemID, since, time.Now().UTC(), flagLimit)
			result := map[string]any{
				"system_id": systemID,
				"count":     len(events),
				"events":    events,
			}
			if flagSince != "" {
				result["since"] = flagSince
			}
			if len(events) == 0 {
				result["note"] = "no synced timeline or detection entries reference this system; check the ID with 'liongard-cli systems list' and run 'liongard-cli sync' to refresh"
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Optional time window to look back (e.g. 24h, 7d, 90m); empty = all history")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum events to return (0 = all)")
	return cmd
}
