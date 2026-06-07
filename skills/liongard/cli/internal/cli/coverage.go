// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// newNovelCoverageCmd implements `coverage` — monitoring gaps from the locally
// synced store: systems with no launchpoint bound (nothing is inspecting them)
// and environments with no systems at all (a client provisioned but never
// pointed an inspector at). Liongard systems carry a nested Launchpoint object,
// so "uninspected" is a system whose Launchpoint reference is absent.
// pp:data-source local
func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagEnv string

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Monitoring gaps: systems with no launchpoint bound, and environments with no systems at all.",
		Long: `Onboarding / coverage QA. Reads the locally synced store and reports two gaps:
systems with no launchpoint bound, and environments with no systems. Run
'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # Estate-wide coverage gaps
  liongard-cli coverage --agent

  # Gaps within one environment
  liongard-cli coverage --env 42
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			systems, err := loadObjs(db, rtSystems)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}

			uninspected, emptyEnvs := findCoverageGaps(systems, envs, flagEnv)

			result := map[string]any{
				"systems_without_launchpoint": uninspected,
				"environments_without_system": emptyEnvs,
				"uninspected_system_count":    len(uninspected),
				"empty_environment_count":     len(emptyEnvs),
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	return cmd
}
