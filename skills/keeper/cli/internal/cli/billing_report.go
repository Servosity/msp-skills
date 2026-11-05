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

func newNovelBillingReportCmd(flags *rootFlags) *cobra.Command {
	var month string
	var byCompany, byDate bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show the monthly consumption billing statement, optionally by company and by date",
		Long: `Runs 'msp-billing-report --format=json'. --month takes YYYY-MM (default: last month, as Commander
decides).`,
		Example: strings.Trim(`
  keeper-cli billing report --agent
  keeper-cli billing report --month 2026-09 --by-company --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, "", func() (keeper.Command, error) { return keeper.Billing(month, byCompany, byDate) })
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "Billing month as YYYY-MM (default: last month)")
	cmd.Flags().BoolVar(&byCompany, "by-company", false, "Break the statement down by managed company")
	cmd.Flags().BoolVar(&byDate, "by-date", false, "Break the statement down by date")
	return cmd
}
