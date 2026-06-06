// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: recurring-finding fingerprint.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelRecurringCmd(flags *rootFlags) *cobra.Command {
	var flagWindow string
	var flagMinCount int

	cmd := &cobra.Command{
		Use:   "recurring",
		Short: "Detections that fire repeatedly across accounts and time, noisiest first",
		Long: "Count distinct findings (grouped by finding name) seen across the snapshot\n" +
			"history within --window, surfacing names that recur at least --min-count times —\n" +
			"a chronic noisy rule or unremediated root cause. Reads the local snapshot history.",
		Example: "  blumira-cli recurring --window 90d --min-count 3\n" +
			"  blumira-cli recurring --json --select name,count,accounts",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			window, err := parseWindow(flagWindow)
			if err != nil {
				return usageErr(err)
			}
			if flagMinCount < 1 {
				flagMinCount = 1
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
			rows := computeRecurring(snaps, since, flagMinCount)
			return emitAnalyticsRows(cmd, flags, rows, len(rows),
				"no recurring findings at the given threshold across the recorded history")
		},
	}
	cmd.Flags().StringVar(&flagWindow, "window", "90d", "Look-back window (e.g. 90d, 12w, 2160h); empty = all history")
	cmd.Flags().IntVar(&flagMinCount, "min-count", 3, "Minimum distinct occurrences for a finding name to count as recurring")
	return cmd
}
