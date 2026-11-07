// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix commander-allowlist).
// pp:data-source live
// pp:client-call

package cli

import (
	"strings"

	"keeper-pp-cli/internal/keeper"

	"github.com/spf13/cobra"
)

func newNovelBreachwatchSummaryCmd(flags *rootFlags) *cobra.Command {
	var mc string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show each user's BreachWatch at-risk, passed and ignored counts",
		Long: `Runs 'security-audit-report --format=json --breachwatch'. Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).
When BreachWatch is not active Keeper ignores --breachwatch, so rows carry no at_risk column.`,
		Example: strings.Trim(`
  keeper-cli breachwatch summary --agent
  keeper-cli breachwatch summary --mc 4421 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, fixed(keeper.SecurityAudit(true)))
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	return cmd
}
