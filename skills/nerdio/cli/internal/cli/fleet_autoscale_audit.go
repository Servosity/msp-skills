// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: fleet autoscale-audit (cross-account autoscale posture).
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"nerdio-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// autoscaleAuditRow is one host pool's autoscale posture in the audit.
type autoscaleAuditRow struct {
	AccountID        int64    `json:"account_id"`
	Account          string   `json:"account"`
	Pool             string   `json:"pool"`
	Subscription     string   `json:"subscription"`
	ResourceGroup    string   `json:"resource_group"`
	AutoscaleEnabled *bool    `json:"autoscale_enabled"` // null when not detectable from the config payload
	DivergentKeys    []string `json:"divergent_keys,omitempty"`
	Flagged          bool     `json:"flagged"`
}

// autoscaleAuditView is the JSON envelope for fleet autoscale-audit.
type autoscaleAuditView struct {
	Pools             []autoscaleAuditRow `json:"pools"`
	FlaggedCount      int                 `json:"flagged_count"`
	ScannedAccounts   int                 `json:"scanned_accounts"`
	UnresolvedPools   []string            `json:"unresolved_pools,omitempty"`
	FetchFailures     []fleetFetchFailure `json:"fetch_failures,omitempty"`
	AccountsTruncated bool                `json:"accounts_truncated,omitempty"`
	Note              string              `json:"note,omitempty"`
}

func newNovelFleetAutoscaleAuditCmd(flags *rootFlags) *cobra.Command {
	var flagAccounts string
	var flagBaseline string
	var flagMaxAccounts int

	cmd := &cobra.Command{
		Use:   "autoscale-audit",
		Short: "Audit autoscale posture for every host pool across customer accounts",
		Long: strings.Trim(`
Use this command to audit autoscale posture (on/off, baseline divergence)
across customer host pools. Do NOT use it for session-host power state; use
'fleet host-estate' instead. Do NOT use it for billing/cost rollups; use
'fleet billing-rollup' instead.

Fans out across customer accounts, lists each account's host pools, reads
each pool's autoscale-configuration, and flags pools where autoscale is
disabled or diverges from a baseline JSON file (--baseline: top-level keys
that must match). Accounts that fail to fetch are reported separately in
fetch_failures - a 401'd account is never a silent "all good".
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli fleet autoscale-audit --agent
  nerdio-cli fleet autoscale-audit --accounts 101,102 --baseline baseline.json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// No required inputs: bare invocation runs the fleet sweep, matching
			// the generated list-command convention. --dry-run short-circuits.
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would audit autoscale configuration across customer accounts")
				return nil
			}
			var baseline map[string]json.RawMessage
			if flagBaseline != "" {
				// #nosec G304 -- flagBaseline is a local file path the operator
				// explicitly passes via --baseline; reading the named file is the
				// feature, and this CLI runs with the invoking user's own privileges.
				raw, err := os.ReadFile(flagBaseline)
				if err != nil {
					return usageErr(fmt.Errorf("reading --baseline: %w", err))
				}
				if err := json.Unmarshal(raw, &baseline); err != nil {
					return usageErr(fmt.Errorf("--baseline must be a JSON object: %w", err))
				}
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

			type accountAudit struct {
				rows       []autoscaleAuditRow
				unresolved []string
			}
			results, errs := cliutil.FanoutRun(cmd.Context(), accounts,
				func(a fleetAccount) string { return fmt.Sprintf("%s (%d)", a.Name, a.ID) },
				func(ctx context.Context, a fleetAccount) (accountAudit, error) {
					out := accountAudit{}
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
						row := autoscaleAuditRow{
							AccountID:     a.ID,
							Account:       a.Name,
							Pool:          name,
							Subscription:  sub,
							ResourceGroup: rg,
						}
						cfgData, err := c.Get(ctx, fmt.Sprintf("/rest-api/v1/accounts/%d/host-pool/%s/%s/%s/autoscale-configuration", a.ID, sub, rg, name), nil)
						if err != nil {
							out.unresolved = append(out.unresolved, fmt.Sprintf("%s/%s (autoscale fetch: %v)", a.Name, name, err))
							continue
						}
						cfgs := decodeObjects(cfgData)
						if len(cfgs) > 0 {
							cfg := cfgs[0]
							if enabled, ok := extractBoolAny(cfg, "enableAutoscale", "isEnabled", "enabled", "autoscaleEnabled", "isAutoscaleEnabled"); ok {
								row.AutoscaleEnabled = &enabled
								if !enabled {
									row.Flagged = true
								}
							}
							for _, key := range sortedKeys(baseline) {
								want := baseline[key]
								got, present := cfg[key]
								if !present || !jsonEqual(want, got) {
									row.DivergentKeys = append(row.DivergentKeys, key)
								}
							}
							if len(row.DivergentKeys) > 0 {
								row.Flagged = true
							}
						}
						out.rows = append(out.rows, row)
					}
					return out, nil
				})

			view := autoscaleAuditView{
				Pools:             make([]autoscaleAuditRow, 0),
				ScannedAccounts:   len(accounts),
				FetchFailures:     fanoutFailures(errs),
				AccountsTruncated: truncated,
			}
			for _, r := range results {
				view.Pools = append(view.Pools, r.Value.rows...)
				view.UnresolvedPools = append(view.UnresolvedPools, r.Value.unresolved...)
			}
			for _, p := range view.Pools {
				if p.Flagged {
					view.FlaggedCount++
				}
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d account fetches failed; audit covers the remaining %d accounts\n",
					len(view.FetchFailures), len(accounts), len(accounts)-len(view.FetchFailures))
			}
			if len(view.Pools) == 0 && len(view.FetchFailures) == 0 {
				view.Note = fmt.Sprintf("scanned %d accounts and found no host pools; pass --accounts to widen or narrow the sweep", len(accounts))
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagAccounts, "accounts", "", "CSV of NMM account IDs to audit (default: every synced/listed account)")
	cmd.Flags().StringVar(&flagBaseline, "baseline", "", "Path to a JSON object of autoscale-configuration keys that must match")
	cmd.Flags().IntVar(&flagMaxAccounts, "max-accounts", 50, "Maximum accounts to scan before returning partial results")
	return cmd
}

// sortedKeys returns the keys of a JSON object in stable order so divergence
// output is deterministic across runs.
func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// jsonEqual compares two raw JSON values structurally (whitespace-insensitive).
func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	aj, _ := json.Marshal(av)
	bj, _ := json.Marshal(bv)
	return string(aj) == string(bj)
}
