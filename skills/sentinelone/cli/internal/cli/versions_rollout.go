// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type rolloutPoint struct {
	CapturedAt string  `json:"captured_at"`
	Total      int     `json:"total"`
	OnTarget   int     `json:"on_target"`
	Pct        float64 `json:"on_target_pct"`
}

// newNovelVersionsRolloutCmd shows how the target (latest) agent version is
// spreading across the fleet over time, built from history snapshots. This is
// impossible from the live API alone, which only returns the current version
// filter with no week-over-week progress.
// pp:data-source local
func newNovelVersionsRolloutCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var target string

	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Track how the latest agent version is spreading across the fleet over time",
		Long: `Track upgrade-wave progress: for each history snapshot, what fraction of the
fleet is on the target version. The target defaults to the most common version
in the latest snapshot (override with --version). A flat or falling percentage
between the last two snapshots is flagged as a stalled rollout.

History accrues one snapshot per 'sentinelone-cli sync', so run sync
repeatedly over time (e.g. daily) to build a meaningful rollout curve.`,
		Example: `  # Rollout of the current target version over time
  sentinelone-cli versions rollout

  # Track a specific target version
  sentinelone-cli versions rollout --version 24.1.2.6 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}

			times, err := db.FleetSnapshotTimes("agents")
			if err != nil {
				return fmt.Errorf("reading snapshot history: %w", err)
			}
			if len(times) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No agent history yet. Run 'sentinelone-cli sync' to capture the first snapshot.", nil)
			}

			// Target = explicit flag, else modal of the latest snapshot.
			latest := times[len(times)-1]
			latestRows, err := db.FleetSnapshotRows("agents", latest)
			if err != nil {
				return fmt.Errorf("reading latest snapshot: %w", err)
			}
			latestAgents := decodeObjects(latestRows)
			if target == "" {
				target = modalVersion(latestAgents)
			}
			if target == "" {
				return honestEmptyJSON(cmd, flags, "Could not determine a target version from the latest snapshot.", nil)
			}

			var timeline []rolloutPoint
			for _, t := range times {
				rows, err := db.FleetSnapshotRows("agents", t)
				if err != nil {
					return fmt.Errorf("reading snapshot %s: %w", t.Format(time.RFC3339), err)
				}
				ags := decodeObjects(rows)
				total, onTarget := 0, 0
				for _, a := range ags {
					if agentDecommissioned(a) {
						continue
					}
					total++
					if gstr(a, "agentVersion") == target {
						onTarget++
					}
				}
				pct := 0.0
				if total > 0 {
					pct = round1(float64(onTarget) / float64(total) * 100)
				}
				timeline = append(timeline, rolloutPoint{
					CapturedAt: t.Format(time.RFC3339),
					Total:      total,
					OnTarget:   onTarget,
					Pct:        pct,
				})
			}

			stalled := false
			if len(timeline) >= 2 {
				last := timeline[len(timeline)-1]
				prev := timeline[len(timeline)-2]
				stalled = last.Pct <= prev.Pct && last.Pct < 100
			}

			if flags.asJSON {
				payload := map[string]any{
					"target_version": target,
					"snapshots":      len(timeline),
					"timeline":       timeline,
					"stalled":        stalled,
				}
				if len(timeline) < 2 {
					payload["note"] = "Only one snapshot so far — run sync again over time to show rollout progress."
				}
				return flags.printJSON(cmd, payload)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Rollout of version %s across %d snapshot(s):\n\n", target, len(timeline))
			fmt.Fprintf(w, "%-25s %7s %10s %9s\n", "CAPTURED", "TOTAL", "ON-TARGET", "PCT")
			for _, p := range timeline {
				fmt.Fprintf(w, "%-25s %7d %10d %8.1f%%\n", p.CapturedAt, p.Total, p.OnTarget, p.Pct)
			}
			if len(timeline) < 2 {
				fmt.Fprintln(w, "\nOnly one snapshot so far — run 'sentinelone-cli sync' again over time to show rollout progress.")
			} else if stalled {
				fmt.Fprintln(w, "\n⚠ Rollout appears stalled (no progress between the last two snapshots).")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().StringVar(&target, "version", "", "Target version to track (default: most common in the latest snapshot)")
	return cmd
}
