// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// newNovelLaunchpointsStaleCmd implements `launchpoints stale` — every
// launchpoint whose newest inspection (timeline entry) is older than
// --older-than, plus launchpoints that have never produced one. Joined to the
// owning environment and system from the local store.
// pp:data-source local
func newNovelLaunchpointsStaleCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan string
	var flagEnv string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Launchpoints whose newest inspection is older than a threshold (or never ran)",
		Long: `Finds collectors that stopped reporting. For each launchpoint, the newest
timeline entry referencing it is compared against --older-than; launchpoints
past the threshold (or with no inspection at all) are returned with the owning
environment and system. Reads the locally synced store; run
'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # Launchpoints with no inspection in 7 days
  liongard-cli launchpoints stale --older-than 7d

  # In one environment, agent JSON
  liongard-cli launchpoints stale --older-than 3d --env 42 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			olderThan, err := parseLookbackDuration(flagOlderThan)
			if err != nil {
				return err
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			stale, err := computeStaleLaunchpoints(db, olderThan, flagEnv)
			if err != nil {
				return err
			}
			result := map[string]any{
				"older_than": flagOlderThan,
				"count":      len(stale),
				"stale":      stale,
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "7d", "Staleness threshold (e.g. 24h, 7d, 90m)")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	return cmd
}
