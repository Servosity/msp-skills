// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: MTTR & response-velocity report.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelVelocityCmd(flags *rootFlags) *cobra.Command {
	var flagWindow string
	var flagBy string

	cmd := &cobra.Command{
		Use:   "velocity",
		Short: "Mean-time-to-resolve and open-rate from the snapshot history, per account or overall",
		Long: "Derive MTTR (time from a finding's first appearance to the snapshot that\n" +
			"began its current resolved stretch) and open-rate from the local snapshot\n" +
			"history within --window. A finding counts by its latest observed state, so one\n" +
			"resolved and then reopened is open. Group by account or report overall. Every\n" +
			"sync records a snapshot, so history accrues as 'sync' runs over time.",
		Example: "  blumira-cli velocity --window 30d --by account\n" +
			"  blumira-cli velocity --window 7d --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			by := strings.ToLower(strings.TrimSpace(flagBy))
			if by == "" {
				by = "account"
			}
			if by != "account" && by != "all" {
				return usageErr(fmt.Errorf("invalid --by %q (use 'account' or 'all')", flagBy))
			}
			window, err := parseWindow(flagWindow)
			if err != nil {
				return usageErr(err)
			}
			now := time.Now().UTC()
			since := time.Time{}
			if window > 0 {
				since = now.Add(-window)
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
			if _, err := captureFindingHistory(s.DB(), s, findings, now); err != nil {
				return apiErr(err)
			}
			snaps, err := loadHistorySnapshots(s.DB())
			if err != nil {
				return apiErr(err)
			}
			rows := computeVelocity(snaps, since, by == "account")
			return emitAnalyticsRows(cmd, flags, rows, len(rows),
				"no velocity data yet — MTTR needs findings observed across multiple syncs over time")
		},
	}
	cmd.Flags().StringVar(&flagWindow, "window", "30d", "Look-back window (e.g. 30d, 2w, 720h); empty = all history")
	cmd.Flags().StringVar(&flagBy, "by", "account", "Group results by: account or all")
	return cmd
}
