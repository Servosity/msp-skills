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

func newNovelAuditTrailEventsCmd(flags *rootFlags) *cobra.Command {
	var mc string
	var created, username, eventType string
	var limit int
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Read raw audit events, newest first, filtered by period, user and event type",
		Long: `Runs 'audit-report --format=json --report-type raw --report-format message'. Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).
--created takes one of: today, yesterday, last_7_days, last_30_days, month_to_date, last_month,
year_to_date, last_year. --username takes an email; --event-type a Keeper event type (login, record_add).`,
		Example: strings.Trim(`
  keeper-cli audit-trail events --created last_7_days --limit 50 --agent
  keeper-cli audit-trail events --mc 4421 --username jane@example.com --event-type login --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, func() (keeper.Command, error) { return keeper.AuditEvents(created, limit, username, eventType) })
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	cmd.Flags().StringVar(&created, "created", "last_7_days", "Period: today, yesterday, last_7_days, last_30_days, month_to_date, last_month, year_to_date, last_year")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	cmd.Flags().StringVar(&username, "username", "", "Only events by this user (email)")
	cmd.Flags().StringVar(&eventType, "event-type", "", "Only this Keeper audit event type (e.g. login)")
	return cmd
}
