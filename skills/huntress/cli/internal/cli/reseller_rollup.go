// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (reprint 20260606): per-account reseller roll-up.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"huntress-pp-cli/internal/cliutil"
)

// pp:data-source live
func newNovelResellerRollupCmd(flags *rootFlags) *cobra.Command {
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "reseller-rollup",
		Short: "Per-account roll-up for resellers: subscriptions, seats, deployed agents",
		Long: "For reseller credentials spanning multiple accounts, joins each account's\n" +
			"subscriptions (products, minimum usage) with its actually-deployed agent count —\n" +
			"a correlation the API never returns in one place.\n" +
			"Do NOT use it for per-org billing-vs-deployment drift inside one account; use 'billing-reconcile' instead.\n" +
			"Live command: calls the API directly and requires reseller-scoped credentials.",
		Example:     "  huntress-cli reseller-rollup --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list reseller accounts, subscriptions, and per-account agent counts")
				return nil
			}
			if flags.dataSource == "local" {
				return fmt.Errorf("this command has no local data source (accounts and subscriptions are not synced); use --data-source auto or live")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}

			// listAll walks cursor pagination for one collection endpoint,
			// bounded by maxScanPages, returning the items under arrayKey.
			listAll := func(path, arrayKey string) ([]map[string]interface{}, int, error) {
				items := []map[string]interface{}{}
				pages := 0
				pageToken := ""
				for pages < maxScanPages {
					params := map[string]string{"limit": "500"}
					if pageToken != "" {
						params["page_token"] = pageToken
					}
					raw, err := c.Get(ctx, path, params)
					if err != nil {
						return items, pages, err
					}
					var envelope map[string]json.RawMessage
					if err := json.Unmarshal(raw, &envelope); err != nil {
						return items, pages, fmt.Errorf("decoding %s response: %w", path, err)
					}
					var page []map[string]interface{}
					if body, ok := envelope[arrayKey]; ok {
						if err := json.Unmarshal(body, &page); err != nil {
							return items, pages, fmt.Errorf("decoding %s.%s: %w", path, arrayKey, err)
						}
					}
					items = append(items, page...)
					pages++
					pageToken = ""
					if pg, ok := envelope["pagination"]; ok {
						var p struct {
							NextPageToken string `json:"next_page_token"`
						}
						if json.Unmarshal(pg, &p) == nil {
							pageToken = p.NextPageToken
						}
					}
					if pageToken == "" || len(page) == 0 {
						break
					}
				}
				return items, pages, nil
			}

			accounts, accountPages, err := listAll("/v1/accounts", "accounts")
			if err != nil {
				return fmt.Errorf("listing reseller accounts: %w (this command requires reseller-scoped credentials)", err)
			}
			subs, _, err := listAll("/v1/reseller/subscriptions", "subscriptions")
			if err != nil {
				return fmt.Errorf("listing reseller subscriptions: %w", err)
			}

			// Group subscriptions by account id.
			type subAgg struct {
				count    int
				products []string
				minUsage float64
			}
			subsByAccount := map[string]*subAgg{}
			for _, s := range subs {
				accID := ""
				if acc, ok := s["account"].(map[string]interface{}); ok {
					if v, ok := toFloat(acc["id"]); ok {
						accID = fmt.Sprintf("%.0f", v)
					}
				}
				if accID == "" {
					if v, ok := toFloat(s["account_id"]); ok {
						accID = fmt.Sprintf("%.0f", v)
					}
				}
				if accID == "" {
					continue
				}
				agg := subsByAccount[accID]
				if agg == nil {
					agg = &subAgg{}
					subsByAccount[accID] = agg
				}
				agg.count++
				if p, ok := s["product"].(string); ok && p != "" {
					agg.products = append(agg.products, p)
				}
				if mu, ok := toFloat(s["minimum_usage"]); ok {
					agg.minUsage += mu
				}
			}

			// Fan out per-account agent counts (limit=1; read pagination.total_count)
			// via cliutil.FanoutRun for bounded concurrency, ordered results,
			// panic recovery, and ctx-cancel semantics.
			if cliutil.IsDogfoodEnv() && len(accounts) > 3 {
				accounts = accounts[:3]
			}
			type acctSrc struct {
				idx int
				id  string
			}
			type acctCount struct {
				idx   int
				count int
			}
			srcs := make([]acctSrc, len(accounts))
			for idx, acc := range accounts {
				if v, ok := toFloat(acc["id"]); ok {
					srcs[idx].id = fmt.Sprintf("%.0f", v)
				}
				srcs[idx].idx = idx
			}
			nameFor := func(s acctSrc) string {
				if s.id != "" {
					return s.id
				}
				return fmt.Sprintf("account[%d]", s.idx)
			}
			fanResults, fanErrs := cliutil.FanoutRun(ctx, srcs, nameFor, func(ctx context.Context, s acctSrc) (acctCount, error) {
				if s.id == "" {
					return acctCount{}, fmt.Errorf("account row missing id")
				}
				raw, err := c.Get(ctx, "/v1/accounts/"+url.PathEscape(s.id)+"/agents", map[string]string{"limit": "1"})
				if err != nil {
					return acctCount{}, err
				}
				var envelope struct {
					Pagination struct {
						TotalCount int `json:"total_count"`
					} `json:"pagination"`
					Agents []json.RawMessage `json:"agents"`
				}
				if err := json.Unmarshal(raw, &envelope); err != nil {
					return acctCount{}, fmt.Errorf("decoding agents envelope: %w", err)
				}
				n := envelope.Pagination.TotalCount
				if n == 0 && len(envelope.Agents) > 0 {
					n = len(envelope.Agents)
				}
				return acctCount{idx: s.idx, count: n}, nil
			})
			agentCounts := make([]int, len(accounts))
			fetchErrors := make([]error, len(accounts))
			nameToIdx := make(map[string]int, len(srcs))
			for _, s := range srcs {
				nameToIdx[nameFor(s)] = s.idx
			}
			for _, r := range fanResults {
				agentCounts[r.Value.idx] = r.Value.count
			}
			for _, e := range fanErrs {
				if idx, ok := nameToIdx[e.Source]; ok {
					fetchErrors[idx] = e.Err
				}
			}
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), fanErrs)

			type failure struct {
				AccountID string `json:"account_id"`
				Error     string `json:"error"`
			}
			failures := make([]failure, 0)
			rows := make([]map[string]interface{}, 0, len(accounts))
			for idx, acc := range accounts {
				accID := ""
				if v, ok := toFloat(acc["id"]); ok {
					accID = fmt.Sprintf("%.0f", v)
				}
				row := map[string]interface{}{
					"account_id":   accID,
					"account_name": acc["name"],
					"status":       acc["status"],
					"subdomain":    acc["subdomain"],
				}
				if agg := subsByAccount[accID]; agg != nil {
					row["subscriptions"] = agg.count
					row["subscription_products"] = agg.products
					row["minimum_usage_total"] = agg.minUsage
				} else {
					row["subscriptions"] = 0
					row["subscription_products"] = []string{}
					row["minimum_usage_total"] = 0
				}
				if fetchErrors[idx] != nil {
					failures = append(failures, failure{AccountID: accID, Error: fetchErrors[idx].Error()})
					row["deployed_agents"] = nil
				} else {
					row["deployed_agents"] = agentCounts[idx]
					if mu, ok := toFloat(row["minimum_usage_total"]); ok {
						row["usage_vs_minimum_delta"] = agentCounts[idx] - int(mu)
					}
				}
				rows = append(rows, row)
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d per-account agent-count fetches failed; deltas computed only for the remaining %d accounts\n",
					len(failures), len(accounts), len(accounts)-len(failures))
			}
			result := map[string]interface{}{
				"accounts":       rows,
				"account_count":  len(rows),
				"scanned_pages":  accountPages,
				"max_scan_pages": maxScanPages,
				"fetch_failures": failures,
			}
			if len(rows) == 0 {
				result["note"] = fmt.Sprintf("no accounts returned within %d page(s); raise --max-scan-pages or verify the credential is reseller-scoped", maxScanPages)
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum account/subscription list pages to scan before returning partial results")
	return cmd
}
