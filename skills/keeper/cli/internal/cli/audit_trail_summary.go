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

func newNovelAuditTrailSummaryCmd(flags *rootFlags) *cobra.Command {
	var mc string
	var period, created string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Count audit events by type per day, week, month or the whole period",
		Long: `Runs 'audit-report --format=json --report-type <period> --aggregate occurrences --columns audit_event_type'.
Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).`,
		Example: strings.Trim(`
  keeper-cli audit-trail summary --period day --created last_7_days --agent
  keeper-cli audit-trail summary --mc 4421 --period span --created last_30_days --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, func() (keeper.Command, error) { return keeper.AuditSummary(period, created) })
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	cmd.Flags().StringVar(&period, "period", "day", "Bucket: day, week, month or span (the whole period)")
	cmd.Flags().StringVar(&created, "created", "last_30_days", "Period: today, yesterday, last_7_days, last_30_days, month_to_date, last_month, year_to_date, last_year")
	return cmd
}
