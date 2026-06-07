// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelInspectorsCoverageCmd implements `inspectors coverage` — for each
// inspector type, which environments have it bound via a launchpoint and
// which are missing it: the estate-wide rollout-gap view.
// pp:data-source local
func newNovelInspectorsCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagInspector string
	var flagOnlyGaps bool

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Which environments are missing a given inspector type - the estate-wide rollout-gap view",
		Long: `Use this command to find which environments are MISSING a given inspector
type (adoption gap by inspector), anti-joining inspectors, launchpoints, and
environments from the local store. Run 'liongard-cli sync' first.

Do NOT use this command to find systems with NO launchpoint at all; use
'coverage' instead.`,
		Example: strings.Trim(`
  # Adoption summary for every inspector, biggest gap first
  liongard-cli inspectors coverage

  # Who is missing the Microsoft 365 inspector?
  liongard-cli inspectors coverage --inspector "Microsoft 365" --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would query local store for inspector adoption gaps")
				return nil
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			inspectors, err := loadObjs(db, rtInspectors)
			if err != nil {
				return err
			}
			launchpoints, err := loadObjs(db, rtLaunchpoints)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}

			rows := computeInspectorCoverage(inspectors, launchpoints, envs, flagInspector)
			if flagOnlyGaps {
				gaps := rows[:0]
				for _, r := range rows {
					if r.EnvironmentsMissing > 0 {
						gaps = append(gaps, r)
					}
				}
				rows = gaps
			}
			result := map[string]any{
				"environments": len(envs),
				"count":        len(rows),
				"inspectors":   rows,
			}
			if flagInspector != "" {
				result["inspector_query"] = flagInspector
			}
			if len(rows) == 0 {
				if len(inspectors) == 0 {
					result["note"] = "no inspectors synced locally; run 'liongard-cli sync --resources inspectors,launchpoints,environments' first"
				} else {
					result["note"] = "no inspectors matched the query; check the name with 'liongard-cli inspectors' or drop --inspector"
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagInspector, "inspector", "", "Filter to inspectors whose name contains this text (case-insensitive)")
	cmd.Flags().BoolVar(&flagOnlyGaps, "only-gaps", false, "Show only inspectors with at least one missing environment")
	return cmd
}
