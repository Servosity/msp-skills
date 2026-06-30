// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `waste rank` — one dollar-ranked rollup of every waste candidate across all hunters.
//
// pp:client-call
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelWasteRankCmd(flags *rootFlags) *cobra.Command {
	var dbPath, profile, region, account string

	cmd := &cobra.Command{
		Use:   "rank",
		Short: "One table of every waste candidate, ranked by estimated monthly dollars",
		Long: `Aggregate every waste hunter (idle EC2, unattached EBS, orphaned snapshots,
unassociated Elastic IPs, gp2 volumes) into one table sorted by estimated
monthly dollars wasted, with a grand-total you could save. The table to paste
into Slack before the bill lands.`,
		Example: `  # Rank waste in the current account
  aws-billing-cli waste rank --profile-aws dev

  # JSON for an agent, just the dollar columns
  aws-billing-cli waste rank --agent --select rows.resource_id,rows.monthly_waste_usd`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			// typeFilter "" => every resource type.
			return runWasteList(cmd, flags, o, "waste rank", "", account, "savings")
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live scan")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live scan")
	cmd.Flags().StringVar(&account, "account", "", "Filter to a 12-digit account ID")
	return cmd
}
