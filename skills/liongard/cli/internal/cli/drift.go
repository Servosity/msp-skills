// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// driftRow is one change Liongard detected, resolved to its owning system and
// environment.
type driftRow struct {
	DetectionID   string `json:"detection_id"`
	Name          string `json:"name,omitempty"`
	CreatedOn     string `json:"created_on,omitempty"`
	TimelineID    string `json:"timeline_id,omitempty"`
	SystemID      string `json:"system_id,omitempty"`
	System        string `json:"system,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

// newNovelDriftCmd implements `drift` — every change Liongard detected across
// the whole estate within a time window, joined to the owning environment and
// system. Liongard detections carry nested Environment and System objects, so
// attribution is direct; environment names fall back to the environments table.
// pp:data-source local
func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagEnv string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Every change detected across all your client environments within a time window",
		Long: `Cross-estate change feed. Lists detections recorded within --since, joined to
the owning environment and system, by reading the locally synced store. Run
'liongard-cli sync' first so detections and environments are present.`,
		Example: strings.Trim(`
  # What changed across all clients in the last 24h
  liongard-cli drift --since 24h

  # Last 7 days, one environment, as agent-friendly JSON
  liongard-cli drift --since 7d --env 42 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			since, err := parseLookbackDuration(flagSince)
			if err != nil {
				return err
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			detections, err := loadObjs(db, rtDetections)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			cutoff := now.Add(-since)
			rows := filterDriftRows(detections, envs, since, now, flagEnv, flagLimit)

			result := map[string]any{
				"since":      flagSince,
				"window_utc": cutoff.Format(time.RFC3339) + "/now",
				"count":      len(rows),
				"detections": rows,
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Time window to look back (e.g. 24h, 7d, 90m)")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum detections to return (0 = all in window)")
	return cmd
}
