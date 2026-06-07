// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: per-analyst workload balance.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelWorkloadCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workload",
		Short: "Open findings grouped by assignee across all accounts, with age buckets",
		Long: "Group every open synced finding by its assignee across all accounts, with age\n" +
			"buckets (<24h, 1-3d, >3d), P1 counts, and the number of accounts each analyst\n" +
			"touches. Findings with no owner aggregate under '(unassigned)' so triage gaps\n" +
			"stay visible. Reads the local store.",
		Example: "  blumira-cli workload\n" +
			"  blumira-cli workload --json --select assignee,open_total,p1_open",
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
			rows := computeWorkload(findings, time.Now().UTC())
			return emitAnalyticsRows(cmd, flags, rows, len(rows), emptyStoreHint)
		},
	}
	return cmd
}
