// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `waste` — read-only AWS waste hunters. Each subcommand surfaces candidates
// with an estimated monthly dollar figure and the remediation step; it never
// mutates AWS.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
)

func newNovelWasteCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "waste",
		Short:       "Find AWS waste — idle instances, orphaned storage, data-transfer bleed (read-only)",
		Long:        "Surface wasted AWS spend without buying anything. Read-only: each finding includes the exact remediation step but the CLI never stops an instance or deletes a volume.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelWasteRankCmd(flags))
	cmd.AddCommand(newNovelWasteGp2Gp3Cmd(flags))
	cmd.AddCommand(newNovelWasteTransferCmd(flags))
	cmd.AddCommand(newWasteIdleCmd(flags))
	cmd.AddCommand(newWasteEBSCmd(flags))
	cmd.AddCommand(newWasteSnapshotsCmd(flags))
	cmd.AddCommand(newWasteEIPCmd(flags))
	return cmd
}

// wasteResult is the JSON shape every waste subcommand returns.
type wasteResult struct {
	Kind            string                   `json:"kind"`
	Source          string                   `json:"source"`
	Count           int                      `json:"count"`
	MonthlyWasteUSD float64                  `json:"monthly_waste_usd"`
	Rows            []awsx.InventoryResource `json:"rows"`
}

// runWasteList is the shared body for the type-filtered waste subcommands. typeFilter
// is "" for the whole-account rank. snarkSignal keys the opt-in quip.
func runWasteList(cmd *cobra.Command, flags *rootFlags, o awsReadOpts, kind, typeFilter, account, snarkSignal string) error {
	items, source, err := ensureInventory(cmd.Context(), o, typeFilter, account, true)
	if err != nil {
		if awsx.IsAccessDenied(err) {
			return fmt.Errorf("denied: grant ec2:Describe*/cloudwatch:GetMetricStatistics for waste detection (run 'aws-billing-cli iam-setup --tier waste')")
		}
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MonthlyWasteUSD > items[j].MonthlyWasteUSD })
	var total float64
	for _, r := range items {
		total += r.MonthlyWasteUSD
	}
	res := wasteResult{Kind: kind, Source: source, Count: len(items), MonthlyWasteUSD: round2f(total), Rows: items}

	if flags.asJSON {
		return flags.printJSON(cmd, res)
	}
	w := cmd.OutOrStdout()
	snark := flags.snark && !flags.asJSON
	if len(items) == 0 {
		fmt.Fprintf(w, "%s: no waste found (source: %s)\n", kind, source)
		fmt.Fprint(w, snarkf(snark, "clean", 0))
		return nil
	}
	fmt.Fprint(w, snarkf(snark, "savings", len(items)))
	fmt.Fprintf(w, "%s (source: %s) — %d candidates, ~$%.2f/mo wasted\n", kind, source, res.Count, res.MonthlyWasteUSD)
	fmt.Fprintf(w, "  %-8s %-24s %-12s %10s  %s\n", "TYPE", "RESOURCE", "REGION", "$WASTE/mo", "REASON")
	for _, r := range items {
		fmt.Fprintf(w, "  %-8s %-24s %-12s %10.2f  %s\n", r.ResourceType, truncate(r.ResourceID, 24), r.Region, r.MonthlyWasteUSD, r.WasteReason)
	}
	if snarkSignal != "" {
		fmt.Fprint(w, snarkf(snark, snarkSignal, len(items)))
	}
	return nil
}

func wasteFlags(cmd *cobra.Command) (dbPath, profile, region, account *string) {
	var db, prof, reg, acct string
	cmd.Flags().StringVar(&db, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&prof, "profile-aws", "", "AWS shared-config profile for a live scan")
	cmd.Flags().StringVar(&reg, "region", "", "AWS region for a live scan")
	cmd.Flags().StringVar(&acct, "account", "", "Filter to a 12-digit account ID")
	return &db, &prof, &reg, &acct
}

func newWasteIdleCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "idle",
		Short:       "Running EC2 instances with near-zero CPU (stop or rightsize)",
		Example:     "  aws-billing-cli waste idle --profile-aws dev",
		Annotations: map[string]string{"mcp:read-only": "true"},
	}
	db, prof, reg, acct := wasteFlags(cmd)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		o := awsReadOptsFromFlags(flags, *db, *prof, *reg)
		return runWasteList(c, flags, o, "idle EC2", "ec2", *acct, "idle")
	}
	return cmd
}

func newWasteEBSCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "ebs",
		Short:       "Unattached EBS volumes (billed while connected to nothing)",
		Example:     "  aws-billing-cli waste ebs --profile-aws dev",
		Annotations: map[string]string{"mcp:read-only": "true"},
	}
	db, prof, reg, acct := wasteFlags(cmd)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		o := awsReadOptsFromFlags(flags, *db, *prof, *reg)
		return runWasteList(c, flags, o, "EBS waste", "ebs", *acct, "orphan")
	}
	return cmd
}

func newWasteSnapshotsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "snapshots",
		Short:       "Orphaned or very old EBS snapshots (review for deletion)",
		Example:     "  aws-billing-cli waste snapshots --profile-aws dev",
		Annotations: map[string]string{"mcp:read-only": "true"},
	}
	db, prof, reg, acct := wasteFlags(cmd)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		o := awsReadOptsFromFlags(flags, *db, *prof, *reg)
		return runWasteList(c, flags, o, "snapshot waste", "snapshot", *acct, "orphan")
	}
	return cmd
}

func newWasteEIPCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "eip",
		Short:       "Unassociated Elastic IPs (billed while attached to nothing)",
		Example:     "  aws-billing-cli waste eip --profile-aws dev",
		Annotations: map[string]string{"mcp:read-only": "true"},
	}
	db, prof, reg, acct := wasteFlags(cmd)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		o := awsReadOptsFromFlags(flags, *db, *prof, *reg)
		return runWasteList(c, flags, o, "Elastic IP waste", "eip", *acct, "orphan")
	}
	return cmd
}
