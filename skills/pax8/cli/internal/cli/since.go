// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): subscription change feed. Not generator-managed.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type subChange struct {
	SubscriptionID string `json:"subscriptionId"`
	ProductID      string `json:"productId,omitempty"`
	ProductName    string `json:"productName,omitempty"`
	CompanyID      string `json:"companyId,omitempty"`
	Change         string `json:"change"`
	Date           string `json:"date,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// parseSinceWindow parses durations like "7d", "24h", "2w", "30" (days).
func parseSinceWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 30 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// bare number => days
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid window %q (use forms like 7d, 24h, 2w, or 30)", s)
}

// parseAnyDate tries the date formats Pax8 timestamps commonly use.
func parseAnyDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339, time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "since [window]",
		Short:       "Diff subscription snapshots and history over time to show new, cancelled, and quantity-changed subscriptions.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Show subscription changes within a recent window from the local store.

Reads the synced subscription history plus current subscriptions to surface what
changed — new subscriptions, cancellations, and quantity/price changes — within
the window (default 30d). Run 'pax8-cli sync' first.

Use this command to see what changed in the book of business over a window (new, cancelled, quantity-changed subscriptions).
Do NOT use this command for revenue totals; use 'mrr' instead.
Do NOT use this command for a single customer's full picture; use 'company show' instead.`,
		Example: `  # Changes in the last 7 days
  pax8-cli since 7d

  # Last 24 hours as JSON
  pax8-cli since 24h --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			window := ""
			if len(args) > 0 {
				window = args[0]
			}
			dur, err := parseSinceWindow(window)
			if err != nil {
				return err
			}
			// windowLabel is what we show the user. When no positional window
			// is given, parseSinceWindow applies the documented 30d default;
			// reflect that default in output instead of an empty string.
			windowLabel := window
			if windowLabel == "" {
				windowLabel = "30d"
			}
			cutoff := time.Now().Add(-dur)

			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "subscriptions", flags.maxAge)

			products, _ := pax8ListObjects(db, "products")
			nameByID := pax8NameByID(products, []string{"id", "productId"}, []string{"name", "productName"})

			var changes []subChange

			// 1) Subscription history events within the window.
			history, _ := pax8ListObjects(db, "history")
			for _, h := range history {
				dateStr := pax8FieldStr(h, "effectiveDate", "createdDate", "date", "changedDate", "updatedDate")
				if t, ok := parseAnyDate(dateStr); ok && t.Before(cutoff) {
					continue
				}
				changes = append(changes, subChange{
					SubscriptionID: pax8FieldStr(h, "subscriptionId", "subscription_id", "id"),
					ProductID:      pax8FieldStr(h, "productId", "product_id"),
					Change:         "history",
					Date:           dateStr,
					Detail:         pax8FieldStr(h, "changeType", "type", "status", "description"),
				})
			}

			// 2) Current subscriptions created or updated within the window.
			subs, _ := pax8ListObjects(db, "subscriptions")
			for _, s := range subs {
				created := pax8FieldStr(s, "createdDate", "created_date", "startDate", "start_date")
				updated := pax8FieldStr(s, "updatedDate", "updated_date")
				pid := pax8FieldStr(s, "productId", "product_id")
				base := subChange{
					SubscriptionID: pax8FieldStr(s, "id", "subscriptionId"),
					ProductID:      pid,
					ProductName:    nameByID[pid],
					CompanyID:      pax8FieldStr(s, "companyId", "company_id"),
				}
				if t, ok := parseAnyDate(created); ok && !t.Before(cutoff) {
					c := base
					c.Change = "new"
					c.Date = created
					c.Detail = "status=" + pax8FieldStr(s, "status")
					changes = append(changes, c)
				} else if t, ok := parseAnyDate(updated); ok && !t.Before(cutoff) {
					c := base
					c.Change = "updated"
					c.Date = updated
					c.Detail = "status=" + pax8FieldStr(s, "status")
					changes = append(changes, c)
				}
			}

			sort.Slice(changes, func(i, j int) bool { return changes[i].Date > changes[j].Date })

			if flags.asJSON {
				return printJSONFiltered(out, changes, flags)
			}
			if len(changes) == 0 {
				fmt.Fprintf(out, "No subscription changes in the last %s. (Run 'pax8-cli sync' first if the store is empty.)\n", windowLabel)
				return nil
			}
			fmt.Fprintln(out, "Date\tChange\tSubscription\tProduct\tDetail")
			fmt.Fprintln(out, "----\t------\t------------\t-------\t------")
			for _, c := range changes {
				prod := c.ProductName
				if prod == "" {
					prod = c.ProductID
				}
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", c.Date, c.Change, c.SubscriptionID, prod, c.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
