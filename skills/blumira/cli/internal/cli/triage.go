// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: unified cross-account triage queue.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagPriority string
	var flagStatus string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "One globally-ranked open-findings queue across every MSP client account",
		Long: "Merge findings synced from your direct org and every MSP sub-account into one\n" +
			"queue, ranked by priority (P1 first) then age (oldest first). Reads the local\n" +
			"store; run 'auth login' then 'sync --full' first.",
		Example: "  blumira-cli triage --priority high --status open\n" +
			"  blumira-cli triage --json --select account_name,name,priority,age_hours",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			prioMatch, err := parsePriorityFilter(flagPriority)
			if err != nil {
				return usageErr(err)
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
			rows := computeTriage(findings, time.Now().UTC(), triageOpts{
				priorityMatch: prioMatch,
				statusMatch:   statusMatch,
				limit:         flagLimit,
			})
			return emitAnalyticsRows(cmd, flags, rows, len(rows), emptyStoreHint)
		},
	}
	cmd.Flags().StringVar(&flagPriority, "priority", "", "Filter by priority: high, critical, medium, low, p1..p5, or a number (default: all)")
	cmd.Flags().StringVar(&flagStatus, "status", "open", "Filter by status: open, resolved, or all")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = no limit)")
	return cmd
}
