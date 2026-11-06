// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/comet"
)

func newNovelUsageCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var flagOver float64
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Get per-tenant storage against quota, billable devices, booster engines and Microsoft 365 accounts (not an invoice)",
		Long: `Get per-tenant storage against quota, billable devices, booster engines and Microsoft 365 accounts (not an invoice).

Use this command for per-tenant counts that drive billing and quota conversations: storage used vs limits, billable devices, booster engines, Microsoft 365 accounts, VM guests. Do NOT use this command for week-over-week changes in those counts; use 'changes --only inventory' instead.

These are the counts this server reports, not an invoice: Comet bills across replicated servers and time windows a single server cannot see. Storage is each Storage Vault's last measured size (ClientProvidedSize). A billable device owns at least one protected item. Booster engines follow the official comet-munin-plugin list (exchangeedb, mssql, vsswriter, hyperv, windisk, mongodb, winmsofficemail); newer engines are not flagged. Microsoft 365 accounts come from each Office 365 item's last job (TotalAccountsCount).`,
		Example: strings.Trim(`
  comet-backup-cli usage --over 80 --agent
  comet-backup-cli usage --by vault --csv
  comet-backup-cli usage --select tenant,storage_bytes,billable_devices,boosters,m365_accounts --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "usage")
			}
			if flagBy != "tenant" && flagBy != "vault" {
				return usageErr(fmt.Errorf("--by must be tenant or vault, got %q", flagBy))
			}
			if flagOver < 0 {
				return usageErr(fmt.Errorf("--over must be a percentage of 0 or more"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			raw, err := cometRead(ctx, c, pathListUsersFull, url.Values{})
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			profiles, err := comet.DecodeProfiles(raw)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			all := comet.Usage(profiles, flagBy == "vault")
			rows := make([]comet.UsageRow, 0, len(all))
			for _, r := range all {
				if flagOver > 0 && r.MaxQuotaPct < flagOver {
					continue
				}
				rows = append(rows, r)
			}
			if flagOver > 0 {
				sort.SliceStable(rows, func(a, b int) bool { return rows[a].MaxQuotaPct > rows[b].MaxQuotaPct })
			}
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, usageAgentKeep...)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tenants match.")
				return nil
			}
			headers := []string{"TENANT", "STORAGE", "LIMIT", "MAX_QUOTA_%", "DEVICES", "BOOSTERS", "M365"}
			if flagBy == "vault" {
				headers = []string{"TENANT", "VAULT", "STORAGE", "LIMIT", "SHARED_QUOTA", "USED_%"}
			}
			rows2 := make([][]string, 0, len(rows))
			for _, r := range rows {
				if flagBy == "vault" {
					rows2 = append(rows2, []string{r.Tenant, r.Vault, humanBytes(r.StorageBytes), humanBytes(r.StorageLimitBytes), r.SharedQuotaID, fmt.Sprintf("%.0f", r.MaxQuotaPct)})
					continue
				}
				rows2 = append(rows2, []string{r.Tenant, humanBytes(r.StorageBytes), humanBytes(r.StorageLimitBytes), fmt.Sprintf("%.0f", r.MaxQuotaPct),
					fmt.Sprint(r.BillableDevices), fmt.Sprint(r.Boosters), fmt.Sprint(r.M365Accounts)})
			}
			return printCleanTable(cmd, flags, headers, rows2)
		}),
	}
	cmd.Flags().StringVar(&flagBy, "by", "tenant", "Report per tenant or per Storage Vault (tenant|vault)")
	cmd.Flags().Float64Var(&flagOver, "over", 0, "Only rows at or above this percent of any quota (storage, devices, Microsoft 365), highest first")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
