// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: ticketing reconciliation export.
// pp:data-source local

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var flagStatus string

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Flat finding-to-org-to-assignee-to-status table for ticketing reconciliation",
		Long: "Project synced findings from every account into one flat, ticket-shaped row set\n" +
			"(account, finding, assignees, status, timestamps) ordered stably so repeated\n" +
			"exports diff cleanly against ConnectWise, Jira, or Zendesk. Reads the local store.",
		Example: "  blumira-cli reconcile --status open --csv\n" +
			"  blumira-cli reconcile --json --select account_name,name,assignees,status",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			statusMatch, err := parseStatusFilter(flagStatus)
			if err != nil {
				return usageErr(err)
			}
			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}
			findings, err := loadFindings(s)
			if err != nil {
				return apiErr(err)
			}
			rows := computeReconcile(findings, statusMatch)
			return emitAnalyticsRows(cmd, flags, rows, len(rows), emptyStoreHint)
		},
	}
	cmd.Flags().StringVar(&flagStatus, "status", "all", "Filter by status: open, resolved, or all")
	return cmd
}
