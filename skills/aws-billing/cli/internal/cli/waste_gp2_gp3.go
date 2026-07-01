// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `waste gp2-gp3` — gp2 EBS volumes with the exact monthly dollars saved by converting to gp3.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
)

func newNovelWasteGp2Gp3Cmd(flags *rootFlags) *cobra.Command {
	var dbPath, profile, region, account string

	cmd := &cobra.Command{
		Use:   "gp2-gp3",
		Short: "gp2 EBS volumes and the exact monthly dollars saved by converting each to gp3",
		Long: `List gp2 EBS volumes and compute the exact monthly dollars saved by
converting each to gp3 (gp3 is ~20% cheaper at baseline). Read-only: shows the
'aws ec2 modify-volume' you would run, never runs it.`,
		Example:     `  aws-billing-cli waste gp2-gp3 --profile-aws dev`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			vols, source, err := ensureInventory(cmd.Context(), o, "ebs", account, false)
			if err != nil {
				if awsx.IsAccessDenied(err) {
					return fmt.Errorf("denied: grant ec2:DescribeVolumes (run 'aws-billing-cli iam-setup --tier waste')")
				}
				return err
			}
			var gp2 []awsx.InventoryResource
			for _, v := range vols {
				if strings.Contains(strings.ToLower(v.WasteReason), "gp2") {
					gp2 = append(gp2, v)
				}
			}
			sort.Slice(gp2, func(i, j int) bool { return gp2[i].MonthlyWasteUSD > gp2[j].MonthlyWasteUSD })
			var saved float64
			for _, v := range gp2 {
				saved += v.MonthlyWasteUSD
			}

			if flags.asJSON {
				return flags.printJSON(cmd, wasteResult{Kind: "gp2-gp3", Source: source, Count: len(gp2), MonthlyWasteUSD: round2f(saved), Rows: gp2})
			}
			w := cmd.OutOrStdout()
			if len(gp2) == 0 {
				fmt.Fprintf(w, "gp2-gp3: no gp2 volumes found (source: %s)\n", source)
				return nil
			}
			fmt.Fprintf(w, "gp2 -> gp3 candidates (source: %s) — %d volumes, ~$%.2f/mo saved\n", source, len(gp2), round2f(saved))
			fmt.Fprintf(w, "  %-24s %-12s %10s\n", "VOLUME", "REGION", "$SAVE/mo")
			for _, v := range gp2 {
				fmt.Fprintf(w, "  %-24s %-12s %10.2f\n", truncate(v.ResourceID, 24), v.Region, v.MonthlyWasteUSD)
			}
			fmt.Fprintf(w, "\nTo convert (you run this — the CLI won't):\n  aws ec2 modify-volume --volume-id <id> --volume-type gp3\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live scan")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live scan")
	cmd.Flags().StringVar(&account, "account", "", "Filter to a 12-digit account ID")
	return cmd
}
