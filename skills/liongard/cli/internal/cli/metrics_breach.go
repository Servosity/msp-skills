// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelMetricsBreachCmd implements `metrics breach <metric-name> --op --value`
// — every system whose RoarPath metric value crosses a numeric threshold. Reads
// the local store (same pivot as `metrics pivot`), then filters to rows whose
// evaluated numeric value satisfies the comparison.
// pp:data-source local
func newNovelMetricsBreachCmd(flags *rootFlags) *cobra.Command {
	var flagOp string
	var flagValue string

	cmd := &cobra.Command{
		Use:   "breach [metric-name]",
		Short: "Every system whose RoarPath metric value crosses a numeric threshold.",
		Long: `Threshold scan across the estate. Pivots a metric across the systems it covers
(from the locally synced store) and returns only the systems whose evaluated
numeric value satisfies --op against --value. Operators: gt, ge, lt, le, eq, ne.
A system whose value is not a number is excluded and counted. Run
'liongard-cli sync' first.

The scan refuses when a covered system has no evaluated value in the local
store, instead of reporting it as not breaching. Liongard evaluates metric
values per system (GET /metrics/bulk, 'liongard-cli metrics evaluation'); the
synced metric definitions do not carry them.`,
		Example: strings.Trim(`
  # Systems whose patch age exceeds 30 days
  liongard-cli metrics breach "Patch Age Days" --op gt --value 30 --agent

  # Systems with fewer than the expected MFA count
  liongard-cli metrics breach "MFA Enabled Count" --op lt --value 5
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
			op := strings.TrimSpace(flagOp)
			if op == "" {
				op = "gt"
			}
			if !validMetricOp(op) {
				return fmt.Errorf("invalid --op %q; use one of gt, ge, lt, le, eq, ne", flagOp)
			}
			if strings.TrimSpace(flagValue) == "" {
				return fmt.Errorf("--value is required (the numeric threshold to compare against)")
			}
			threshold, perr := strconv.ParseFloat(strings.TrimSpace(flagValue), 64)
			if perr != nil {
				return fmt.Errorf("invalid --value %q; must be a number", flagValue)
			}

			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, matched, _, err := buildMetricRows(db, name)
			if err != nil {
				return err
			}
			// A covered system with no evaluated value could be breaching:
			// counting it as "not breaching" is a wrong zero (Codex r2 class
			// incomplete-snapshot-read-as-complete), so refuse.
			unevaluated := 0
			for _, r := range rows {
				if r.SystemID != "" && !r.HasValue {
					unevaluated++
				}
			}
			if unevaluated > 0 {
				return fmt.Errorf("%w of metric values: %d of the %d covered system(s) have no evaluated value for %q in the local store, so the threshold cannot be checked; Liongard evaluates metric values per system (GET /metrics/bulk, 'liongard-cli metrics evaluation') and the synced metric definitions do not carry them", errIncompleteSnapshot, unevaluated, countCoveredSystems(rows), name)
			}
			breaches := make([]metricRow, 0)
			nonNumeric := 0
			for _, r := range rows {
				if r.SystemID == "" {
					continue
				}
				if r.NumericValue == nil {
					nonNumeric++
					continue
				}
				if compareMetric(*r.NumericValue, op, threshold) {
					breaches = append(breaches, r)
				}
			}
			result := map[string]any{
				"metric_query":        name,
				"op":                  op,
				"threshold":           threshold,
				"matched_definitions": matched,
				"breach_count":        len(breaches),
				"breaches":            breaches,
				"non_numeric_systems": nonNumeric,
			}
			if matched == 0 {
				result["note"] = "No metric definition matched; check the name with 'liongard-cli metrics pivot' or 'liongard-cli metrics metrics'."
			} else if countCoveredSystems(rows) == 0 {
				result["note"] = "The matched metric definitions cover no synced system (no system carries their inspector and environment)."
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagOp, "op", "gt", "Comparison operator: gt, ge, lt, le, eq, ne")
	cmd.Flags().StringVar(&flagValue, "value", "", "Numeric threshold to compare each system's metric value against (required)")
	return cmd
}

// countCoveredSystems counts pivot rows that name a system (a definition
// that covers no synced system has a row with no system).
func countCoveredSystems(rows []metricRow) int {
	n := 0
	for _, r := range rows {
		if r.SystemID != "" {
			n++
		}
	}
	return n
}
