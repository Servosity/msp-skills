// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: cross-account MSP overview rollup.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelOverviewCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Per-account rollup: open findings by priority, oldest age, and agent health at a glance",
		Long: "Use this command for a per-account rollup table (counts/health across all\n" +
			"clients at a glance). Do NOT use this command for the single ranked list of\n" +
			"individual findings to work next; use 'triage' instead.\n\n" +
			"Joins synced findings and agent devices per account: open counts by priority,\n" +
			"oldest open age, resolved counts, device/DC/stale-agent tallies. The hottest\n" +
			"account (most P1s, then most open) sorts first. Reads the local store.",
		Example: "  blumira-cli overview\n" +
			"  blumira-cli overview --json --select account_name,p1_open,open_findings",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
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
			devices, err := loadDevices(s)
			if err != nil {
				return apiErr(err)
			}
			rows := computeOverview(findings, devices, time.Now().UTC())
			return emitAnalyticsRows(cmd, flags, rows, len(rows), emptyStoreHint)
		},
	}
	return cmd
}
