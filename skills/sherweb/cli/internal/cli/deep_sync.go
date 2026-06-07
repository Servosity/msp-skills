// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"sherweb-pp-cli/internal/client"
	"sherweb-pp-cli/internal/cliutil"
	"sherweb-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// newDeepSyncCmd populates the per-customer, normalized resource types that the
// transcendence commands (margin, orphans, fleet-subs, right-size, amend-preview,
// drift) join over. The base `sync` command only covers param-less list
// endpoints (customers, payable charges, platforms); the Service Provider
// subscription/receivable/usage endpoints all require a customerId, so they need
// this fan-out. Every API call goes through the same client path as the absorbed
// commands.
// pp:data-source live
func newDeepSyncCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "deep-sync",
		Short: "Fan out per-customer subscriptions, receivable charges, and usage into the local store.",
		Long: `Iterate every synced customer and pull their subscriptions (with pricing),
receivable charges, and metered usage from the Service Provider API into the
local store, then append payable, receivable, and subscription snapshots for
drift/trend/churn tracking.

Run 'sherweb-cli sync' first to populate customers + payable charges. The
transcendence commands (margin, orphans, fleet-subs, right-size, amend-preview,
drift, margin-trend, sub-changes, usage-leak) read what this command writes.`,
		Example:     "  sherweb-cli deep-sync\n  sherweb-cli deep-sync --limit 50",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would deep-sync per-customer subscriptions, receivable charges, usage, and a payable snapshot")
				return nil
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would deep-sync per-customer subscriptions, receivable charges, usage, and a payable snapshot")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			s, err := openInsightStore(cmd.Context())
			if err != nil {
				return err
			}
			defer s.Close()

			customers, err := loadCustomersForDeepSync(cmd.Context(), c, s)
			if err != nil {
				return fmt.Errorf("listing customers: %w", err)
			}
			if flagLimit > 0 && len(customers) > flagLimit {
				customers = customers[:flagLimit]
			}
			if len(customers) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no customers found — run 'sherweb-cli sync' first")
			}

			var subs, recv, usage, errCount, subsErrs, recvErrs int
			var allSubs, allRecv []map[string]any
			for _, cust := range customers {
				params := map[string]string{"customerId": cust.id}

				if items, e := deepSyncSubscriptions(cmd.Context(), c, s, cust, params); e != nil {
					errCount++
					subsErrs++
					fmt.Fprintf(cmd.ErrOrStderr(), "  %s: subscriptions: %v\n", cust.label(), e)
				} else {
					subs += len(items)
					allSubs = append(allSubs, items...)
				}
				if items, e := deepSyncReceivable(cmd.Context(), c, s, cust, params); e != nil {
					errCount++
					recvErrs++
					fmt.Fprintf(cmd.ErrOrStderr(), "  %s: receivable-charges: %v\n", cust.label(), e)
				} else {
					recv += len(items)
					allRecv = append(allRecv, items...)
				}
				if n, e := deepSyncUsage(cmd.Context(), c, s, cust, params); e != nil {
					errCount++
					fmt.Fprintf(cmd.ErrOrStderr(), "  %s: usage: %v\n", cust.label(), e)
				} else {
					usage += n
				}
			}

			snap, snapErr := deepSyncPayableSnapshot(cmd.Context(), c, s)
			if snapErr != nil {
				errCount++
				fmt.Fprintf(cmd.ErrOrStderr(), "  payable snapshot: %v\n", snapErr)
			}

			// Append fleet-wide subscription/receivable snapshots only when the
			// fan-out for that side completed without errors — a partial snapshot
			// would surface as phantom "removed" rows in sub-changes and as a
			// fake margin drop in margin-trend.
			snapID := time.Now().UTC().Format(time.RFC3339)
			subsSnap, recvSnap := "", ""
			if len(customers) > 0 && subsErrs == 0 {
				if err := upsertSnapshot(s, rtSubsSnapshot, snapID, allSubs); err != nil {
					errCount++
					fmt.Fprintf(cmd.ErrOrStderr(), "  subscription snapshot: %v\n", err)
				} else {
					subsSnap = snapID
				}
			} else if subsErrs > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "  subscription snapshot skipped: fan-out had errors")
			}
			if len(customers) > 0 && recvErrs == 0 {
				if err := upsertSnapshot(s, rtReceivableSnapshot, snapID, allRecv); err != nil {
					errCount++
					fmt.Fprintf(cmd.ErrOrStderr(), "  receivable snapshot: %v\n", err)
				} else {
					recvSnap = snapID
				}
			} else if recvErrs > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "  receivable snapshot skipped: fan-out had errors")
			}

			report := map[string]any{
				"customers":            len(customers),
				"subscriptions":        subs,
				"receivableCharges":    recv,
				"usageRows":            usage,
				"payableSnapshot":      snap,
				"subscriptionSnapshot": subsSnap,
				"receivableSnapshot":   recvSnap,
				"errors":               errCount,
			}
			return flags.printJSON(cmd, report)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum number of customers to fan out over (0 = all)")
	return cmd
}

type deepSyncCustomer struct {
	id   string
	name string
}

func (d deepSyncCustomer) label() string {
	if d.name != "" {
		return d.name
	}
	return d.id
}

// extractItems pulls the items[] array (or a bare array) out of a response.
func extractItems(data json.RawMessage) []map[string]any {
	var asObj map[string]any
	if err := json.Unmarshal(data, &asObj); err == nil {
		if arr, ok := pick(asObj, "items").([]any); ok {
			return toMaps(arr)
		}
	}
	var asArr []any
	if err := json.Unmarshal(data, &asArr); err == nil {
		return toMaps(asArr)
	}
	return nil
}

func toMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func loadCustomersForDeepSync(ctx context.Context, c *client.Client, s *store.Store) ([]deepSyncCustomer, error) {
	// Prefer a fresh live list; the base sync already stored them, but deep-sync
	// is explicitly a live refresh.
	data, err := c.GetWithHeaders(ctx, "/service-provider/v1/customers", nil, nil)
	if err != nil {
		// Fall back to whatever the base sync already stored.
		rows, lerr := loadResourceRows(s, rtCustomers) // uncapped via loadResourceRows; store.List would cap at 200
		if lerr != nil || len(rows) == 0 {
			return nil, err
		}
		var out []deepSyncCustomer
		for _, raw := range rows {
			m := asMap(raw)
			out = append(out, deepSyncCustomer{id: pstr(m, "id", "customerId"), name: pstr(m, "name", "customerName", "displayName")})
		}
		return out, nil
	}
	var out []deepSyncCustomer
	for _, m := range extractItems(data) {
		id := pstr(m, "id", "customerId")
		if id == "" {
			continue
		}
		out = append(out, deepSyncCustomer{id: id, name: pstr(m, "name", "customerName", "displayName")})
	}
	return out, nil
}

// upsertTagged decorates each item with the customer id/name and upserts it under
// resourceType, returning the number written.
func upsertTagged(s *store.Store, resourceType string, cust deepSyncCustomer, items []map[string]any, idFields ...string) (int, error) {
	n := 0
	for i, m := range items {
		if _, ok := m["customerId"]; !ok {
			m["customerId"] = cust.id
		}
		if _, ok := m["customerName"]; !ok && cust.name != "" {
			m["customerName"] = cust.name
		}
		id := ""
		for _, f := range idFields {
			if v := pstr(m, f); v != "" {
				id = v
				break
			}
		}
		if id == "" {
			id = fmt.Sprintf("%s-%d", cust.id, i)
		} else {
			id = cust.id + ":" + id
		}
		raw, err := json.Marshal(m)
		if err != nil {
			return n, err
		}
		if err := s.Upsert(resourceType, id, raw); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func deepSyncSubscriptions(ctx context.Context, c *client.Client, s *store.Store, cust deepSyncCustomer, params map[string]string) ([]map[string]any, error) {
	data, err := c.GetWithHeaders(ctx, "/service-provider/v1/billing/subscriptions/details", params, nil)
	if err != nil {
		return nil, err
	}
	items := extractItems(data)
	// Enrich with unit prices from pricing-information when available.
	if priceData, perr := c.GetWithHeaders(ctx, "/service-provider/v1/billing/subscriptions/pricing-information", params, nil); perr == nil {
		prices := map[string]float64{}
		for _, p := range extractItems(priceData) {
			if id := pstr(p, "subscriptionId", "id"); id != "" {
				prices[id] = pnum(p, "unitPrice", "price", "unitListPrice")
			}
		}
		for _, m := range items {
			id := pstr(m, "id", "subscriptionId")
			if up, ok := prices[id]; ok {
				if _, exists := m["unitPrice"]; !exists {
					m["unitPrice"] = up
				}
			}
		}
	}
	if _, err := upsertTagged(s, rtSubscriptions, cust, items, "id", "subscriptionId"); err != nil {
		return nil, err
	}
	return items, nil
}

func deepSyncReceivable(ctx context.Context, c *client.Client, s *store.Store, cust deepSyncCustomer, params map[string]string) ([]map[string]any, error) {
	data, err := c.GetWithHeaders(ctx, "/service-provider/v1/billing/receivable-charges", params, nil)
	if err != nil {
		return nil, err
	}
	items := extractItems(data)
	if len(items) == 0 {
		// receivable-charges may return a {charges:[]} envelope rather than items[].
		var obj map[string]any
		if json.Unmarshal(data, &obj) == nil {
			if arr, ok := pick(obj, "charges").([]any); ok {
				items = toMaps(arr)
			}
		}
	}
	if _, err := upsertTagged(s, rtReceivable, cust, items, "chargeId", "id"); err != nil {
		return nil, err
	}
	return items, nil
}

func deepSyncUsage(ctx context.Context, c *client.Client, s *store.Store, cust deepSyncCustomer, params map[string]string) (int, error) {
	data, err := c.GetWithHeaders(ctx, "/service-provider/v1/billing/subscriptions/meters", params, nil)
	if err != nil {
		return 0, err
	}
	return upsertTagged(s, rtUsage, cust, extractItems(data), "subscriptionId", "id", "sku")
}

// deepSyncPayableSnapshot appends a timestamped payable-charges snapshot so
// `drift` can diff period-over-period.
func deepSyncPayableSnapshot(ctx context.Context, c *client.Client, s *store.Store) (string, error) {
	data, err := c.GetWithHeaders(ctx, "/distributor/v1/billing/payable-charges", nil, nil)
	if err != nil {
		return "", err
	}
	id := time.Now().UTC().Format(time.RFC3339)
	if err := s.Upsert(rtPayableSnapshot, id, data); err != nil {
		return "", err
	}
	return id, nil
}

// upsertSnapshot marshals tagged items into a {items:[...]} snapshot document
// keyed by an RFC3339 snapshot id, mirroring deepSyncPayableSnapshot's shape.
func upsertSnapshot(s *store.Store, resourceType, id string, items []map[string]any) error {
	if items == nil {
		items = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return err
	}
	return s.Upsert(resourceType, id, raw)
}
