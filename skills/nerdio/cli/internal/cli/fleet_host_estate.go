// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: fleet host-estate (cross-account session-host inventory).
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"

	"nerdio-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// hostEstateRow is one session host in the cross-account estate view.
type hostEstateRow struct {
	AccountID  int64  `json:"account_id"`
	Account    string `json:"account"`
	Pool       string `json:"pool"`
	Host       string `json:"host"`
	PowerState string `json:"power_state,omitempty"`
	Running    *bool  `json:"running"` // null when power state is not detectable
}

// hostEstateView is the JSON envelope for fleet host-estate.
type hostEstateView struct {
	Items             []hostEstateRow     `json:"items"`
	HostCount         int                 `json:"host_count"`
	RunningCount      int                 `json:"running_count"`
	ScannedAccounts   int                 `json:"scanned_accounts"`
	UnresolvedPools   []string            `json:"unresolved_pools,omitempty"`
	FetchFailures     []fleetFetchFailure `json:"fetch_failures,omitempty"`
	AccountsTruncated bool                `json:"accounts_truncated,omitempty"`
	Note              string              `json:"note,omitempty"`
}

func newNovelFleetHostEstateCmd(flags *rootFlags) *cobra.Command {
	var flagAccounts string
	var flagRunningOnly bool
	var flagMaxAccounts int

	cmd := &cobra.Command{
		Use:   "host-estate",
		Short: "List every session host across customer accounts with power state",
		Long: strings.Trim(`
Use this command for cross-account session-host inventory and power state.
Do NOT use it for autoscale configuration posture; use 'fleet autoscale-audit'
instead.

Fans out across customer accounts, lists each account's host pools, then each
pool's session hosts, and emits one table of account / pool / host / power
state - the weekend power-sweep view that the NMM web UI only shows one
account at a time. Accounts that fail to fetch are reported separately in
fetch_failures.
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli fleet host-estate --agent
  nerdio-cli fleet host-estate --running-only --agent --select items.account,items.host,items.power_state
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// No required inputs: bare invocation runs the fleet sweep.
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list session hosts across customer accounts")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			accounts, err := resolveFleetAccounts(cmd.Context(), cmd, flags, c, flagAccounts)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			accounts, truncated := capFleetAccounts(accounts, flagMaxAccounts)

			type accountEstate struct {
				rows       []hostEstateRow
				unresolved []string
			}
			results, errs := cliutil.FanoutRun(cmd.Context(), accounts,
				func(a fleetAccount) string { return fmt.Sprintf("%s (%d)", a.Name, a.ID) },
				func(ctx context.Context, a fleetAccount) (accountEstate, error) {
					out := accountEstate{}
					poolsData, err := c.Get(ctx, fmt.Sprintf("/rest-api/v1/accounts/%d/host-pool", a.ID), nil)
					if err != nil {
						return out, fmt.Errorf("listing host pools: %w", err)
					}
					for _, pool := range decodeObjects(poolsData) {
						sub, rg, name, ok := poolIdentity(pool)
						if !ok {
							label, _ := extractStringAny(pool, "name", "hostPoolName", "poolName")
							if label == "" {
								label = "(unnamed pool)"
							}
							out.unresolved = append(out.unresolved, fmt.Sprintf("%s/%s", a.Name, label))
							continue
						}
						hostsData, err := c.Get(ctx, fmt.Sprintf("/rest-api/v1/accounts/%d/host-pool/%s/%s/%s/hosts", a.ID, sub, rg, name), nil)
						if err != nil {
							out.unresolved = append(out.unresolved, fmt.Sprintf("%s/%s (hosts fetch: %v)", a.Name, name, err))
							continue
						}
						for _, host := range decodeObjects(hostsData) {
							hostName, _ := extractStringAny(host, "name", "hostName", "vmName", "sessionHostName")
							if hostName == "" {
								hostName = "(unnamed host)"
							}
							row := hostEstateRow{
								AccountID: a.ID,
								Account:   a.Name,
								Pool:      name,
								Host:      hostName,
							}
							if state, ok := extractStringAny(host, "powerState", "power_state", "status", "state", "vmStatus"); ok {
								row.PowerState = state
								running := strings.Contains(strings.ToLower(state), "running") || strings.Contains(strings.ToLower(state), "available")
								row.Running = &running
							}
							out.rows = append(out.rows, row)
						}
					}
					return out, nil
				})

			view := hostEstateView{
				Items:             make([]hostEstateRow, 0),
				ScannedAccounts:   len(accounts),
				FetchFailures:     fanoutFailures(errs),
				AccountsTruncated: truncated,
			}
			for _, r := range results {
				for _, row := range r.Value.rows {
					if flagRunningOnly && (row.Running == nil || !*row.Running) {
						continue
					}
					view.Items = append(view.Items, row)
				}
				view.UnresolvedPools = append(view.UnresolvedPools, r.Value.unresolved...)
			}
			view.HostCount = len(view.Items)
			for _, row := range view.Items {
				if row.Running != nil && *row.Running {
					view.RunningCount++
				}
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d account fetches failed; estate covers the remaining %d accounts\n",
					len(view.FetchFailures), len(accounts), len(accounts)-len(view.FetchFailures))
			}
			if len(view.Items) == 0 && len(view.FetchFailures) == 0 {
				view.Note = fmt.Sprintf("scanned %d accounts and found no session hosts%s", len(accounts),
					map[bool]string{true: " matching --running-only", false: ""}[flagRunningOnly])
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagAccounts, "accounts", "", "CSV of NMM account IDs to scan (default: every synced/listed account)")
	cmd.Flags().BoolVar(&flagRunningOnly, "running-only", false, "Only include hosts whose power state reads as running/available")
	cmd.Flags().IntVar(&flagMaxAccounts, "max-accounts", 50, "Maximum accounts to scan before returning partial results")
	return cmd
}
