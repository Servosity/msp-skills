// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// newNovelMetricsPivotCmd implements `metrics pivot <metric-name>` — one
// RoarPath metric pulled across every system it applies to, as a
// system-by-value table. Reads the local store, matches metric definitions by
// name, and resolves the covered systems (directly or via the metric's
// inspector). Values populate from synced metric data when present.
// pp:data-source local
func newNovelMetricsPivotCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pivot [metric-name]",
		Short: "One RoarPath metric pulled across every system as a system-by-value table, CSV-ready for reports.",
		Long: `Pivots a single metric across the systems it applies to, from the locally
synced store. Matches metric definitions by case-insensitive substring on name,
resolves the covered systems (from a SystemID on the metric, else via the
metric's inspector), and includes the evaluated value per system when the synced
data carries it. Use --csv for a report-ready table. Run
'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # Pivot a metric across all systems, CSV for a report
  liongard-cli metrics pivot "MFA Enabled Count" --csv

  # As agent JSON
  liongard-cli metrics pivot "Patch Age Days" --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			name := strings.TrimSpace(args[0])
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, matched, anyValue, err := buildMetricRows(db, name)
			if err != nil {
				return err
			}
			result := map[string]any{
				"metric_query":        name,
				"matched_definitions": matched,
				"system_rows":         len(rows),
				"rows":                rows,
			}
			if matched == 0 {
				result["note"] = "No metric definition matched; check the name or run 'liongard-cli sync'."
			} else if !anyValue {
				result["note"] = "Matched metric definitions but no evaluated values are present in the synced data; rows show the systems the metric covers. Sync metric evaluations to populate values."
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	return cmd
}
