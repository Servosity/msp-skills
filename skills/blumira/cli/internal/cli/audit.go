// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: resolution re-fire audit.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelAuditCmd(flags *rootFlags) *cobra.Command {
	var flagMinReopens int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Findings that were resolved then re-fired (reopened), worst first",
		Long: "Scan the snapshot history for findings that were marked resolved in one sync and\n" +
			"then appeared non-resolved in a later sync — a signal of premature or low-quality\n" +
			"resolutions. Reads the local store's snapshot history (accrues as 'sync' runs).",
		Example: "  blumira-cli audit\n" +
			"  blumira-cli audit --min-reopens 2 --json",
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
			if _, err := captureFindingHistory(s.DB(), s, findings, time.Now().UTC()); err != nil {
				return apiErr(err)
			}
			snaps, err := loadHistorySnapshots(s.DB())
			if err != nil {
				return apiErr(err)
			}
			rows := computeAudit(snaps)
			if flagMinReopens > 1 {
				filtered := rows[:0]
				for _, r := range rows {
					if r.Reopens >= flagMinReopens {
						filtered = append(filtered, r)
					}
				}
				rows = filtered
			}
			return emitAnalyticsRows(cmd, flags, rows, len(rows),
				"no re-fired findings detected across the recorded snapshot history")
		},
	}
	cmd.Flags().IntVar(&flagMinReopens, "min-reopens", 1, "Only show findings re-fired at least this many times")
	return cmd
}
