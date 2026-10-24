// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type rolloutCount struct {
	Total    int     `json:"total"`
	OnTarget int     `json:"on_target"`
	Pct      float64 `json:"on_target_pct"`
}

type rolloutPoint struct {
	CapturedAt string                  `json:"captured_at"`
	Total      int                     `json:"total"`
	OnTarget   int                     `json:"on_target"`
	Pct        float64                 `json:"on_target_pct"`
	ByOS       map[string]rolloutCount `json:"by_os"`
}

// newNovelVersionsRolloutCmd shows how the target (latest) agent version is
// spreading across the fleet over time, built from history snapshots. This is
// impossible from the live API alone, which only returns the current version
// filter with no week-over-week progress.
// pp:data-source local
func newNovelVersionsRolloutCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var target string
	var osName string

	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Track how the latest agent version is spreading across the fleet over time",
		Long: `Track upgrade-wave progress: for each history snapshot, what fraction of the
fleet is at or above its target version. Windows, macOS and Linux agents ship
separate packages, so each OS has its own target: by default the most common
version among live agents of that OS in the latest snapshot. --version names a
target explicitly and then needs --os. Decommissioned agents never count. A
flat or falling percentage between the last two snapshots is flagged as a
stalled rollout.

History accrues one snapshot per 'sentinelone-cli sync', so run sync
repeatedly over time (e.g. daily) to build a meaningful rollout curve.`,
		Example: `  # Rollout of the current target version over time
  sentinelone-cli versions rollout

  # Track a specific target version
  sentinelone-cli versions rollout --os windows --version 24.1.2.6 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			osName = strings.ToLower(strings.TrimSpace(osName))
			target = strings.TrimSpace(target)
			if target != "" && osName == "" {
				return usageErr(fmt.Errorf("--version needs --os (agent packages are per OS)"))
			}
			if target != "" && !s1VersionRe.MatchString(target) {
				return usageErr(fmt.Errorf("--version %q is not a dotted numeric agent version (e.g. 24.1.2.6)", target))
			}
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

			// Targets: explicit --os/--version, else the modal live version per OS
			// in the latest snapshot.
			latest := times[len(times)-1]
			latestRows, err := db.FleetSnapshotRows("agents", latest)
			if err != nil {
				return fmt.Errorf("reading latest snapshot: %w", err)
			}
			targets := modalVersionByOS(decodeObjects(latestRows))
			if target != "" {
				targets = map[string]string{osName: target}
			} else if osName != "" {
				targets = map[string]string{osName: targets[osName]}
			}
			for os, v := range targets {
				if v == "" {
					delete(targets, os)
				}
			}
			if len(targets) == 0 {
				return honestEmptyJSON(cmd, flags, "Could not determine a target version from the latest snapshot.", nil)
			}

			var timeline []rolloutPoint
			for _, t := range times {
				rows, err := db.FleetSnapshotRows("agents", t)
				if err != nil {
					return fmt.Errorf("reading snapshot %s: %w", t.Format(time.RFC3339), err)
				}
				point := rolloutPoint{CapturedAt: t.Format(time.RFC3339), ByOS: map[string]rolloutCount{}}
				for _, a := range decodeObjects(rows) {
					if agentDecommissioned(a) {
						continue
					}
					os := agentOS(a)
					goal, ok := targets[os]
					if !ok {
						continue
					}
					c := point.ByOS[os]
					c.Total++
					point.Total++
					if v := strings.TrimSpace(gstr(a, "agentVersion")); v != "" && compareS1Version(v, goal) >= 0 {
						c.OnTarget++
						point.OnTarget++
					}
					point.ByOS[os] = c
				}
				for os, c := range point.ByOS {
					if c.Total > 0 {
						c.Pct = round1(float64(c.OnTarget) / float64(c.Total) * 100)
					}
					point.ByOS[os] = c
				}
				if point.Total > 0 {
					point.Pct = round1(float64(point.OnTarget) / float64(point.Total) * 100)
				}
				timeline = append(timeline, point)
			}

			stalled := false
			if len(timeline) >= 2 {
				last := timeline[len(timeline)-1]
				prev := timeline[len(timeline)-2]
				stalled = last.Pct <= prev.Pct && last.Pct < 100
			}

			if flags.asJSON {
				payload := map[string]any{
					"target_by_os": targets,
					"snapshots":    len(timeline),
					"timeline":     timeline,
					"stalled":      stalled,
				}
				if len(targets) == 1 {
					for _, v := range targets {
						payload["target_version"] = v
					}
				}
				if len(timeline) < 2 {
					payload["note"] = "Only one snapshot so far — run sync again over time to show rollout progress."
				}
				return flags.printJSON(cmd, payload)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Rollout toward %s across %d snapshot(s):\n\n", formatTargets(targets), len(timeline))
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
	cmd.Flags().StringVar(&target, "version", "", "Target version to track (needs --os; default: most common live version per OS in the latest snapshot)")
	cmd.Flags().StringVar(&osName, "os", "", "Only track agents of this OS type as SentinelOne reports it (windows, macos, linux)")
	return cmd
}
