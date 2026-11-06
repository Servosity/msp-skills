// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: unfinished and errored orders across the account.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
)

type stuckOrderRow struct {
	OrderID     string   `json:"order_id"`
	CustomerID  string   `json:"customer_id"`
	Customer    string   `json:"customer"`
	Status      string   `json:"status"`
	Reason      string   `json:"reason"`
	AgeHours    *int     `json:"age_hours"` // null when ION sent no createTime
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Cancellable bool     `json:"cancellable"`
	Items       []string `json:"items"`
	Total       *float64 `json:"total"` // null when ION sent no total
	Currency    string   `json:"currency"`
}

// openOrderStatuses are non-terminal; cancellableStatuses is the subset the
// Cancel Order endpoint accepts (docs.streamone.cloud#cancel-order).
var openOrderStatuses = map[string]bool{"NEW": true, "IN_PROGRESS": true, "CONFIRMED": true, "ON_HOLD": true}
var cancellableStatuses = map[string]bool{"NEW": true, "ON_HOLD": true}

// orderItemLabel names an order item for humans. ION UPDATE items carry only
// resourceId (the vendor subscription id) and a CCP skuId code, so the synced
// subscription's own label wins; then the item's skuName, skuId and resourceId.
func orderItemLabel(it ionOrderItem, sub ionSub, matched bool) string {
	if matched {
		if l := sub.Label(); l != "" {
			return l
		}
	}
	for _, v := range []string{it.SkuName, it.SkuID, it.ResourceID} {
		if v != "" {
			return v
		}
	}
	return "unnamed item"
}

func ageText(h *int) string {
	if h == nil {
		return "unknown"
	}
	return strconv.Itoa(*h) + "h"
}

// stuckOrders returns open orders older than olderThan plus ERROR orders updated within since.
func stuckOrders(b *ionBook, now time.Time, olderThan, since time.Duration) []stuckOrderRow {
	byResource := subsByResource(b)
	rows := make([]stuckOrderRow, 0)
	for _, o := range b.Orders {
		status := strings.ToUpper(o.Status)
		created, updated := o.Created(), o.Updated()
		var reason string
		switch {
		case openOrderStatuses[status] && !created.IsZero():
			if now.Sub(created) < olderThan {
				continue
			}
			reason = "open past threshold"
		case openOrderStatuses[status]:
			// No createTime: an updateTime past the threshold proves the age is
			// too; otherwise the order is listed with its age unknown, never
			// dropped (adversarial round 17, P1).
			reason = "open, age unknown"
			if olderThan > 0 && !updated.IsZero() && now.Sub(updated) >= olderThan {
				reason = "open past threshold"
			}
		case status == "ERROR" && parseIONTime(o.UpdateTime).IsZero():
			// The error time is unknown (createTime is not when it errored), so
			// the order is listed, never dropped (adversarial round 18, P2).
			reason = "errored, time unknown"
		case status == "ERROR" && now.Sub(updated) <= since:
			reason = "errored recently"
		default:
			continue
		}
		items := make([]string, 0, len(o.Items))
		for _, it := range o.Items {
			sub, matched := byResource[strings.ToLower(it.ResourceID)]
			items = append(items, fmt.Sprintf("%s x%s (%s)", orderItemLabel(it, sub, matched), seatsText(it.Quantity.V, it.Quantity.Set), strings.ToLower(it.Action)))
		}
		var age *int // unknown, never 0, when ION sent no createTime (Codex round 18)
		if !created.IsZero() {
			h := int(now.Sub(created).Hours())
			age = &h
		}
		rows = append(rows, stuckOrderRow{
			OrderID:     o.OrderID,
			CustomerID:  o.CustomerID,
			Customer:    b.CustomerLabel(o.CustomerID, ""),
			Status:      status,
			Reason:      reason,
			AgeHours:    age,
			CreatedAt:   o.CreateTime,
			UpdatedAt:   o.UpdateTime,
			Cancellable: cancellableStatuses[status],
			Items:       items,
			Total:       optPtr(optFloat{V: round2(o.Total.V), Set: o.Total.Set}),
			Currency:    o.CurrencyCode,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := -1, -1 // unknown ages sort last
		if rows[i].AgeHours != nil {
			ai = *rows[i].AgeHours
		}
		if rows[j].AgeHours != nil {
			aj = *rows[j].AgeHours
		}
		if ai != aj {
			return ai > aj
		}
		return rows[i].OrderID < rows[j].OrderID
	})
	return rows
}

func newNovelOrdersStuckCmd(flags *rootFlags) *cobra.Command {
	var olderThan, since, dbPath string

	cmd := &cobra.Command{
		Use:   "stuck",
		Short: "Find orders still NEW, IN_PROGRESS, CONFIRMED or ON_HOLD past an age threshold, plus recent errors",
		Long: "Find orders across the whole account that are still NEW, IN_PROGRESS, CONFIRMED or ON_HOLD after --older-than, " +
			"plus orders that moved to ERROR within --since. An open order ION sent no createTime for is listed with reason " +
			"\"open, age unknown\" (and age_hours null) unless its updateTime already proves it is past the threshold, and an ERROR order " +
			"with no updateTime is listed as \"errored, time unknown\". Each row says whether ION still lets you cancel it " +
			"(only NEW and ON_HOLD orders can be cancelled). Reads the local store; run sync --resources orders,subscriptions,customers first.\n\n" +
			"Use this command for orders that have not finished or that errored. Do NOT use this command for a history " +
			"of completed seat, renewal, or status changes; use 'subscriptions changes' instead.",
		Example: strings.Trim(`
  td-synnex-cli orders stuck --older-than 24h --agent
  td-synnex-cli orders stuck --older-than 2h --since 7d`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--older-than=24h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "orders stuck")
			}
			older, err := cliutil.ParseDurationLoose(olderThan)
			if err != nil || older < 0 {
				return usageErr(fmt.Errorf("--older-than must be a duration like 24h or 2d, got %q", olderThan))
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window < 0 {
				return usageErr(fmt.Errorf("--since must be a duration like 7d, got %q", since))
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "orders", "customers", "subscriptions") // subscriptions name UPDATE items
			if err != nil {
				return err
			}
			defer closeDB()
			rows := make([]stuckOrderRow, 0)
			if book != nil {
				rows = stuckOrders(book, time.Now().UTC(), older, window)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No stuck or errored orders.")
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				cancel := "no"
				if r.Cancellable {
					cancel = "yes"
				}
				table = append(table, []string{r.OrderID, r.Customer, r.Status, r.Reason, ageText(r.AgeHours), cancel, strings.Join(r.Items, "; ")})
			}
			return printTable(cmd.OutOrStdout(), []string{"ORDER", "CUSTOMER", "STATUS", "REASON", "AGE", "CANCELLABLE", "ITEMS"}, table)
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "24h", "Report open orders created at least this long ago")
	cmd.Flags().StringVar(&since, "since", "7d", "Report ERROR orders updated within this window")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
