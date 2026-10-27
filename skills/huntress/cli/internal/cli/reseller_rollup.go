// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (reprint 20260606): per-account reseller roll-up.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"huntress-pp-cli/internal/cliutil"
)

// resellerPageLimit is the page size the roll-up asks for on every list.
const resellerPageLimit = "500"

// resellerComparableProduct is the subscription product whose minimum usage
// is a count of endpoint agents, the only product deployed_agents measures.
// sat (learners), itdr (identities) and siem (data) commit to other units.
const resellerComparableProduct = "edr"

// pp:data-source live
func newNovelResellerRollupCmd(flags *rootFlags) *cobra.Command {
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "reseller-rollup",
		Short: "Per-account roll-up for resellers: subscriptions, seats, deployed agents",
		Long: "For reseller credentials spanning multiple accounts, joins each account's\n" +
			"subscriptions (products, minimum usage) with its actually-deployed agent count —\n" +
			"a correlation the API never returns in one place. usage_vs_minimum_delta compares\n" +
			"deployed agents with the EDR minimum of active subscriptions only (other products\n" +
			"commit to other units), and is withheld whenever a list it depends on was cut short.\n" +
			"Do NOT use it to compare invoiced seats with the agents in the local store; use 'billing-reconcile' instead.\n" +
			"Live command: calls the API directly and requires reseller-scoped credentials.",
		Example:     "  huntress-cli reseller-rollup --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages %d: must be at least 1", maxScanPages))
			}
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
			// bounded by maxScanPages, returning the items under arrayKey and
			// whether the walk reached the end of the collection. Every page
			// goes through parseListPage: a 200 without the list, a null or
			// non-list value, or pagination it cannot read is an error, never
			// an empty, complete collection.
			listAll := func(ctx context.Context, path, arrayKey string) (items []map[string]interface{}, pages int, complete bool, err error) {
				items = []map[string]interface{}{}
				pageToken := ""
				for pages < maxScanPages {
					params := map[string]string{"limit": resellerPageLimit}
					if pageToken != "" {
						params["page_token"] = pageToken
					}
					raw, err := c.Get(ctx, path, params)
					if err != nil {
						return items, pages, false, err
					}
					page, next, err := parseListPage(raw, arrayKey)
					if err != nil {
						return items, pages, false, fmt.Errorf("reading %s: %w", path, err)
					}
					items = append(items, page...)
					pages++
					if next == "" {
						return items, pages, true, nil
					}
					pageToken = next
				}
				return items, pages, false, nil
			}

			accounts, accountPages, accountsComplete, err := listAll(ctx, "/v1/accounts", "accounts")
			if err != nil {
				return fmt.Errorf("listing reseller accounts: %w (this command requires reseller-scoped credentials)", err)
			}
			subs, subPages, subsComplete, err := listAll(ctx, "/v1/reseller/subscriptions", "subscriptions")
			if err != nil {
				return fmt.Errorf("listing reseller subscriptions: %w", err)
			}
			if !accountsComplete {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: the account list has more than %d page(s); only the first %d accounts are shown. Raise --max-scan-pages to see the rest.\n", maxScanPages, len(accounts))
			}
			if !subsComplete {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: the subscription list has more than %d page(s); subscription figures and deltas are withheld because they would be short. Raise --max-scan-pages.\n", maxScanPages)
			}

			// Group subscriptions by account id. Every figure built from a
			// subscription field the CLI cannot read (an account, a product, a
			// status or a minimum) is unknown: the figure is withheld (null)
			// and the warning names what could not be read. It is never summed
			// as 0, and an unattributed subscription is never dropped.
			type sum struct {
				total   float64
				unknown []string
			}
			type subAgg struct {
				count     int
				products  []string
				byProduct map[string]*sum
				edr       sum // the active EDR minimum
			}
			newAgg := func() *subAgg { return &subAgg{byProduct: map[string]*sum{}} }
			subsByAccount := map[string]*subAgg{}
			unattributed := []map[string]interface{}{}
			// Products an unattributed subscription might add to any account.
			unattributedProducts := map[string][]string{}
			var unattributedEDR []string
			for _, s := range subs {
				subID := entityKey(s["id"])
				desc := "subscription " + subID
				if subID == "" {
					desc = "a subscription with no id"
				}
				product, productOK := s["product"].(string)
				product = strings.ToLower(strings.TrimSpace(product))
				productOK = productOK && product != ""
				productKey := product
				if !productOK {
					productKey = "unknown"
				}
				status, statusOK := s["status"].(string)
				status = strings.ToLower(strings.TrimSpace(status))
				statusOK = statusOK && status != ""
				mu, muOK := toFloat(s["minimum_usage"])
				muOK = muOK && s["minimum_usage"] != nil && !math.IsNaN(mu) && !math.IsInf(mu, 0) && mu >= 0
				// mayCount: the subscription is, or may be, an active one.
				mayCount := !statusOK || status == "active"

				accID := ""
				if acc, ok := s["account"].(map[string]interface{}); ok {
					accID = entityKey(acc["id"])
				}
				if accID == "" {
					accID = entityKey(s["account_id"])
				}
				if accID == "" {
					unattributed = append(unattributed, map[string]interface{}{"id": s["id"], "product": s["product"], "status": s["status"]})
					if mayCount {
						unattributedProducts[productKey] = append(unattributedProducts[productKey], desc+" has no readable account and may belong to this account")
						if !productOK || product == resellerComparableProduct {
							unattributedEDR = append(unattributedEDR, desc+" has no readable account and may be this account's EDR commitment")
						}
					}
					continue
				}
				agg := subsByAccount[accID]
				if agg == nil {
					agg = newAgg()
					subsByAccount[accID] = agg
				}
				agg.count++
				if productOK {
					agg.products = append(agg.products, product)
				}
				// minimum_usage_by_product sums active minimums; a subscription
				// whose status cannot be read may be active.
				if mayCount {
					ps := agg.byProduct[productKey]
					if ps == nil {
						ps = &sum{}
						agg.byProduct[productKey] = ps
					}
					switch {
					case !statusOK:
						ps.unknown = append(ps.unknown, desc+" has no readable status")
					case !muOK:
						ps.unknown = append(ps.unknown, desc+" ("+productKey+", active) has no readable minimum_usage")
					default:
						ps.total += mu
					}
				}
				if productOK && product != resellerComparableProduct {
					continue
				}
				// An EDR subscription, or one whose product cannot be read
				// (it may be EDR).
				switch {
				case !mayCount:
				case !productOK:
					agg.edr.unknown = append(agg.edr.unknown, desc+" has no readable product and may be an EDR commitment")
				case !statusOK:
					agg.edr.unknown = append(agg.edr.unknown, desc+" (edr) has no readable status")
				case !muOK:
					agg.edr.unknown = append(agg.edr.unknown, desc+" (edr, active) has no readable minimum_usage")
				default:
					agg.edr.total += mu
				}
			}
			if len(unattributed) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d subscription(s) are unattributed (no readable account); the per-product minimums they may add to any account are withheld. They are listed under unattributed_subscriptions.\n", len(unattributed))
			}

			// Fan out per-account agent counts. Every page is counted: the
			// pagination envelope carries no total, and one page's length is
			// not the size of the collection.
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
				srcs[idx] = acctSrc{idx: idx, id: entityKey(acc["id"])}
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
				agents, pages, complete, err := listAll(ctx, "/v1/accounts/"+url.PathEscape(s.id)+"/agents", "agents")
				if err != nil {
					return acctCount{}, err
				}
				if !complete {
					return acctCount{}, fmt.Errorf("the agent list is longer than %d page(s); raise --max-scan-pages to count it", pages)
				}
				return acctCount{idx: s.idx, count: len(agents)}, nil
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
				accID := entityKey(acc["id"])
				row := map[string]interface{}{
					"account_id":   accID,
					"account_name": acc["name"],
					"status":       acc["status"],
					"subdomain":    acc["subdomain"],
				}
				agg := subsByAccount[accID]
				if agg == nil {
					agg = newAgg()
				}
				// Fold in what an unattributed subscription might add here.
				for _, p := range sortedSumKeys(unattributedProducts) {
					ps := agg.byProduct[p]
					if ps == nil {
						ps = &sum{}
						agg.byProduct[p] = ps
					}
					ps.unknown = append(ps.unknown, unattributedProducts[p]...)
				}
				edrUnknown := append(append([]string{}, agg.edr.unknown...), unattributedEDR...)
				unreadable := []string{}
				if subsComplete {
					products := append([]string{}, agg.products...)
					sort.Strings(products)
					byProduct := map[string]interface{}{}
					for _, p := range sortedSumKeys(agg.byProduct) {
						ps := agg.byProduct[p]
						if len(ps.unknown) > 0 {
							byProduct[p] = nil
							unreadable = append(unreadable, ps.unknown...)
						} else {
							byProduct[p] = ps.total
						}
					}
					row["subscriptions"] = agg.count
					row["subscription_products"] = products
					row["minimum_usage_by_product"] = byProduct
					if len(edrUnknown) > 0 {
						row["minimum_usage_total"] = nil
						unreadable = append(unreadable, edrUnknown...)
					} else {
						row["minimum_usage_total"] = agg.edr.total
					}
				} else {
					row["subscriptions"] = nil
					row["subscription_products"] = nil
					row["minimum_usage_by_product"] = nil
					row["minimum_usage_total"] = nil
				}
				unreadable = uniqueStrings(unreadable)
				row["unreadable_inputs"] = unreadable
				for _, u := range unreadable {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: account %s: %s; the figures that depend on it are withheld (null).\n", accID, u)
				}
				if fetchErrors[idx] != nil {
					failures = append(failures, failure{AccountID: accID, Error: fetchErrors[idx].Error()})
					row["deployed_agents"] = nil
				} else {
					row["deployed_agents"] = agentCounts[idx]
				}
				if subsComplete && fetchErrors[idx] == nil && len(edrUnknown) == 0 {
					row["usage_vs_minimum_delta"] = agentCounts[idx] - int(agg.edr.total)
				} else {
					row["usage_vs_minimum_delta"] = nil
				}
				rows = append(rows, row)
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d per-account agent-count fetches failed; deltas computed only for the remaining %d accounts\n",
					len(failures), len(accounts), len(accounts)-len(failures))
			}
			result := map[string]interface{}{
				"accounts":                   rows,
				"account_count":              len(rows),
				"accounts_complete":          accountsComplete,
				"subscriptions_complete":     subsComplete,
				"scanned_pages":              accountPages,
				"subscription_pages":         subPages,
				"max_scan_pages":             maxScanPages,
				"fetch_failures":             failures,
				"unattributed_subscriptions": unattributed,
			}
			if len(rows) == 0 {
				result["note"] = fmt.Sprintf("no accounts returned within %d page(s); raise --max-scan-pages or verify the credential is reseller-scoped", maxScanPages)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				return err
			}
			return printOutputWithFlagsMetaAndKeep(cmd.OutOrStdout(), json.RawMessage(raw), flags, map[string]any{"source": "live"},
				[]string{"accounts_complete", "subscriptions_complete", "fetch_failures", "unattributed_subscriptions", "usage_vs_minimum_delta", "deployed_agents", "unreadable_inputs"})
		},
	}
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum pages to scan per list (accounts, subscriptions, each account's agents) before reporting it incomplete")
	return cmd
}

// sortedSumKeys returns a map's keys in order.
func sortedSumKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// uniqueStrings keeps the first occurrence of each string, in order.
func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
