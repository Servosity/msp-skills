// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

type exposureDevice struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	HostName    string `json:"host_name"`
	DeviceID    string `json:"device_id"`
}

type exposureRow struct {
	CVE         string           `json:"cve"`
	Criticality string           `json:"criticality"`
	Patches     []string         `json:"patches"`
	Accounts    int              `json:"accounts"`
	Devices     int              `json:"devices"`
	Affected    []exposureDevice `json:"affected"`
	rank        int
}

type exposureView struct {
	AccountsChecked   int            `json:"accounts_checked"`
	MissingPatchRows  int            `json:"missing_patch_rows"`
	Exposure          []exposureRow  `json:"exposure"`
	NoPatchData       []subscriber   `json:"no_patch_data"`
	TruncatedAccounts []string       `json:"truncated_accounts,omitempty"`
	FetchFailures     []fetchFailure `json:"fetch_failures,omitempty"`
	Note              string         `json:"note,omitempty"`
}

type accountPatches struct {
	sub       subscriber
	rows      []map[string]any
	truncated bool
}

// pivotExposure groups per-device missing-patch rows by CVE. Patches that list
// no CVE are grouped under "no-cve:<patch name>" so they are not dropped.
func pivotExposure(accounts []accountPatches, cveFilter string, minRank int) []exposureRow {
	byKey := map[string]*exposureRow{}
	seenDevice := map[string]map[string]bool{}
	seenAccount := map[string]map[string]bool{}
	seenPatch := map[string]map[string]bool{}
	for _, ap := range accounts {
		for _, m := range ap.rows {
			code, _ := intField(m, "patch_criticality")
			rank := criticalityRank[code]
			if rank < minRank {
				continue
			}
			keys := strList(m, "patch_cve_ids")
			if len(keys) == 0 {
				keys = []string{"no-cve:" + strField(m, "patch_name")}
			}
			for _, key := range keys {
				if cveFilter != "" && !strings.EqualFold(key, cveFilter) {
					continue
				}
				row, ok := byKey[key]
				if !ok {
					row = &exposureRow{CVE: key, Patches: make([]string, 0), Affected: make([]exposureDevice, 0)}
					byKey[key] = row
					seenDevice[key], seenAccount[key], seenPatch[key] = map[string]bool{}, map[string]bool{}, map[string]bool{}
				}
				if rank > row.rank {
					row.rank, row.Criticality = rank, decodeCode(criticalityCodes, m, "patch_criticality")
				}
				if p := strField(m, "patch_name"); p != "" && !seenPatch[key][p] {
					seenPatch[key][p] = true
					row.Patches = append(row.Patches, p)
				}
				seenAccount[key][ap.sub.ID] = true
				dev := ap.sub.ID + "/" + strField(m, "device_id")
				if !seenDevice[key][dev] {
					seenDevice[key][dev] = true
					row.Affected = append(row.Affected, exposureDevice{AccountID: ap.sub.ID, AccountName: ap.sub.Name, HostName: strField(m, "host_name"), DeviceID: strField(m, "device_id")})
				}
			}
		}
	}
	out := make([]exposureRow, 0, len(byKey))
	for _, key := range sortedMapKeys(byKey) {
		r := byKey[key]
		r.Accounts, r.Devices = len(seenAccount[key]), len(seenDevice[key])
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank > out[j].rank
		}
		return out[i].Devices > out[j].Devices
	})
	return out
}

func fetchPatches(ctx context.Context, c *client.Client, s subscriber, maxPages int) (accountPatches, error) {
	path := "/rest/endpoint-security/management/api/v1/accounts/" + url.PathEscape(s.ID) + "/patchavailability"
	rows, trunc, err := pageOData(ctx, c, s.ID, path, maxPages)
	if err != nil {
		return accountPatches{}, err
	}
	return accountPatches{sub: s, rows: rows, truncated: trunc}, nil
}

func newNovelEndpointPatchesExposureCmd(flags *rootFlags) *cobra.Command {
	var tf tenantFlags
	var cve, minCriticality string
	var limit int
	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Group missing patches by CVE and criticality across every subscriber, with affected accounts and devices (needs Patch Management)",
		Long: "Use this command to find which subscribers and devices are missing patches, grouped by CVE and criticality. " +
			"Do NOT use this command for protection, antivirus or isolation status; use 'endpoint-devices posture' instead.\n\n" +
			"Patch data exists only where WatchGuard Patch Management is licensed and enabled. Accounts that return no missing-patch rows " +
			"are listed under no_patch_data rather than counted as fully patched. Patches that name no CVE appear as \"no-cve:<patch name>\".\n\n" +
			"Exit codes: 0 all accounts read, 5 every account failed, 6 some accounts failed (see fetch_failures; --allow-partial-failure exits 0).",
		Example: strings.Trim(`
  watchguard-cli endpoint-patches exposure --agent
  watchguard-cli endpoint-patches exposure --min-criticality important --json
  watchguard-cli endpoint-patches exposure --cve CVE-2024-38063`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "endpoint-patches exposure")
			}
			minRank := 0
			if minCriticality != "" {
				r, ok := criticalityName[strings.ToLower(minCriticality)]
				if !ok {
					return usageErr(fmt.Errorf("--min-criticality must be one of critical, important, moderate, low"))
				}
				minRank = r
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			subs, accTrunc, err := resolveSubscribers(ctx, c, tf)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			results, errs := cliutil.FanoutRun(ctx, subs, func(s subscriber) string { return s.ID },
				func(ctx context.Context, s subscriber) (accountPatches, error) {
					return fetchPatches(ctx, c, s, tf.pages())
				}, cliutil.WithConcurrency(tf.workers()))
			view := exposureView{NoPatchData: make([]subscriber, 0), FetchFailures: fanoutFailures(errs)}
			accounts := make([]accountPatches, 0, len(results))
			for _, r := range results {
				view.AccountsChecked++
				view.MissingPatchRows += len(r.Value.rows)
				if len(r.Value.rows) == 0 {
					view.NoPatchData = append(view.NoPatchData, r.Value.sub)
				}
				if r.Value.truncated {
					view.TruncatedAccounts = append(view.TruncatedAccounts, r.Source)
				}
				accounts = append(accounts, r.Value)
			}
			view.Exposure = pivotExposure(accounts, cve, minRank)
			if limit > 0 && len(view.Exposure) > limit {
				view.Exposure = view.Exposure[:limit]
			}
			if len(view.NoPatchData) > 0 {
				view.Note = "no_patch_data lists accounts that returned no missing patches: fully patched, or Patch Management not licensed or enabled"
			}
			if len(view.TruncatedAccounts) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --max-pages cut the patch list short for: "+strings.Join(view.TruncatedAccounts, ", "))
			}
			if n := accountsTruncatedNote(cmd.ErrOrStderr(), accTrunc); n != "" {
				view.Note = strings.TrimPrefix(view.Note+"; "+n, "; ")
			}
			if err := renderExposure(cmd, flags, view); err != nil {
				return err
			}
			return fanoutVerdict(cmd.ErrOrStderr(), flags, len(errs), len(subs))
		},
	}
	tf.register(cmd.Flags())
	cmd.Flags().StringVar(&cve, "cve", "", "Only report this CVE ID, e.g. CVE-2024-38063")
	cmd.Flags().StringVar(&minCriticality, "min-criticality", "", "Lowest patch criticality to include: critical, important, moderate or low (security patches only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum CVE rows to return (0 = all)")
	return cmd
}

func renderExposure(cmd *cobra.Command, flags *rootFlags, view exposureView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	if len(view.Exposure) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No missing patches matched across %d account(s).\n", view.AccountsChecked)
		return nil
	}
	tw := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(tw, "CVE\tCRITICALITY\tACCOUNTS\tDEVICES\tPATCHES")
	for _, r := range view.Exposure {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", r.CVE, r.Criticality, r.Accounts, r.Devices, truncate(strings.Join(r.Patches, ", "), 60))
	}
	return tw.Flush()
}
