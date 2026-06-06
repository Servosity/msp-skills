// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: detection coverage drift vs the MSP basis ruleset.
// pp:data-source local

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagAgainst string

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Detection rules missing or disabled in your org versus the MSP basis ruleset",
		Long: "Join the MSP basis detection rules against your org's deployed rules (matched by\n" +
			"rule name) and report each basis rule that is missing or not enabled. Reads the\n" +
			"local store; sync 'msp-basis-detection-rules' and 'org-detection-rules' first.",
		Example: "  blumira-cli coverage --against basis\n" +
			"  blumira-cli coverage --json --select rule_name,gap",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagAgainst != "" && flagAgainst != "basis" {
				return usageErr(fmt.Errorf("invalid --against %q (only 'basis' is supported)", flagAgainst))
			}
			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "msp-basis-detection-rules") {
				hintIfStale(cmd, s, "msp-basis-detection-rules", flags.maxAge)
			}
			if !hintIfUnsynced(cmd, s, "org-detection-rules") {
				hintIfStale(cmd, s, "org-detection-rules", flags.maxAge)
			}
			basis, err := loadRules(s, "msp-basis-detection-rules")
			if err != nil {
				return apiErr(err)
			}
			orgRules, err := loadRules(s, "org-detection-rules")
			if err != nil {
				return apiErr(err)
			}
			if len(basis) == 0 {
				return emitAnalyticsRows(cmd, flags, []coverageRow{}, 0,
					"no basis rules in the local store — run 'sync --resource msp-basis-detection-rules' first")
			}
			rows := computeCoverage(basis, orgRules)
			return emitAnalyticsRows(cmd, flags, rows, len(rows), "no coverage gaps: every basis rule is deployed and enabled")
		},
	}
	cmd.Flags().StringVar(&flagAgainst, "against", "basis", "Baseline to compare against (only 'basis' is supported)")
	return cmd
}
