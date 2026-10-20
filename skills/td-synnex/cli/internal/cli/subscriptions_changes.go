// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: decoded subscription change feed from synced orders.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
)

type subChangeRow struct {
	When           string   `json:"when"`      // the order's createTime, else its updateTime; empty when ION sent neither
	WhenFrom       string   `json:"when_from"` // created, updated or unknown
	CustomerID     string   `json:"customer_id"`
	Customer       string   `json:"customer"`
	Subscription   string   `json:"subscription"`
	SubscriptionID string   `json:"subscription_id"` // ION subscription id when the order item matched a synced subscription
	ResourceID     string   `json:"resource_id"`     // vendor subscription id from the order item
	Change         string   `json:"change"`          // new, seats, renewal, status, tier, friendly_name
	Value          string   `json:"value"`
	Quantity       *float64 `json:"quantity"`       // seat count the order item carried; null when ION sent none
	PreviousSeats  *float64 `json:"previous_seats"` // seat count before this order when the synced history shows it, else null
	OrderID        string   `json:"order_id"`
	OrderStatus    string   `json:"order_status"`
	Applied        bool     `json:"applied"` // the order completed; otherwise the change is only requested
	By             string   `json:"by"`
}

// decodeChanges maps one order item to the changes it requested, using the
// documented UPDATE-order attribute recipes (docs.streamone.cloud#update-order-*).
// Every updateSubscription item sets the seat count, so it always yields a
// seats row, also when it changes the renewal setting or name in the same order;
// that seats value can equal the previous count when only the other fields changed.
func decodeChanges(it ionOrderItem) [][2]string {
	qty := seatsText(it.Quantity.V, it.Quantity.Set) // "unknown" when the item carried no count, never 0 (Codex round 17, P1)
	if strings.EqualFold(it.Action, "CREATE") {
		return [][2]string{{"new", qty}}
	}
	out := make([][2]string, 0, 2)
	tier := strings.EqualFold(it.attr("operations"), "changeSubscription")
	if tier {
		out = append(out, [2]string{"tier", strings.Trim(it.SkuID+" / "+it.PlanID, " /")})
	}
	status := it.attr("statusUpdate")
	if status != "" {
		out = append(out, [2]string{"status", status})
	}
	if !tier && status == "" && it.Quantity.Set && it.Quantity.V > 0 {
		out = append(out, [2]string{"seats", qty})
	}
	if v := it.attr("renewalSetting"); v != "" {
		out = append(out, [2]string{"renewal", v})
	}
	if v := it.attr("friendlyName"); v != "" {
		out = append(out, [2]string{"friendly_name", v})
	}
	if len(out) == 0 {
		out = append(out, [2]string{"seats", qty})
	}
	return out
}

func subscriptionChanges(b *ionBook, now time.Time, since time.Duration, customerID string, includeFailed bool) []subChangeRow {
	byResource := subsByResource(b)
	// Walk every synced order oldest first so each seats row can be compared with
	// the seat count the last completed order set for that subscription. An
	// order's time is its createTime, else its updateTime. An order with
	// neither is still listed (with an unknown time, whatever the window), and
	// because its place in the history is unknown, no previous_seats is stated
	// for its subscription (adversarial round 17, P1: such orders vanished and
	// left a stale count).
	when := func(o ionOrder) (time.Time, string) {
		if t := o.Created(); !t.IsZero() {
			return t, "created"
		}
		if t := o.Updated(); !t.IsZero() {
			return t, "updated"
		}
		return time.Time{}, "unknown"
	}
	orders := append([]ionOrder(nil), b.Orders...)
	undated := map[string]bool{} // resources a completed order of unknown time changed
	for _, o := range orders {
		if t, _ := when(o); t.IsZero() && strings.EqualFold(o.Status, "COMPLETED") {
			for _, it := range o.Items {
				if it.ResourceID != "" {
					undated[strings.ToLower(it.ResourceID)] = true
				}
			}
		}
	}
	sort.SliceStable(orders, func(i, j int) bool {
		ti, _ := when(orders[i])
		tj, _ := when(orders[j])
		return ti.Before(tj)
	})
	lastSeats, setAt := map[string]float64{}, map[string]time.Time{}
	// A completed item of unknown resource (other than a new purchase) may have
	// changed any of its customer's subscriptions ("" = any customer). A count
	// set at S is trusted at T only if no such change falls in [S, T]; a change
	// with no time at all blinds its customer for good. Times, not walk order,
	// decide, since same-instant and undated orders have no order (adversarial
	// rounds 21-23, Codex round 25).
	blindTimes, blindForever := map[string][]time.Time{}, map[string]bool{}
	for _, o := range orders {
		if !strings.EqualFold(o.Status, "COMPLETED") {
			continue
		}
		t, _ := when(o)
		for _, it := range o.Items {
			if it.ResourceID != "" || strings.EqualFold(it.Action, "CREATE") {
				continue
			}
			if t.IsZero() {
				blindForever[o.CustomerID] = true
			} else {
				blindTimes[o.CustomerID] = append(blindTimes[o.CustomerID], t)
			}
		}
	}
	blinded := func(customer string, from, to time.Time) bool {
		if blindForever[customer] || blindForever[""] {
			return true
		}
		customers := []string{customer, ""}
		if customer == "" { // an order of unknown customer may share any customer's history (adversarial round 24)
			customers = customers[:0]
			for c := range blindTimes {
				customers = append(customers, c)
			}
			for c := range blindForever {
				if blindForever[c] {
					return true
				}
			}
		}
		for _, c := range customers {
			for _, b := range blindTimes[c] {
				if !b.Before(from) && !b.After(to) {
					return true
				}
			}
		}
		return false
	}
	// Completed items of one subscription at one time (in one order or in
	// several) cannot be ordered, so none of them, nor the next change, gets a
	// previous_seats; nor does a failed or cancelled item at the instant of a
	// completed one (Codex rounds 24 and 28, adversarial round 26).
	tieKey := func(res string, t time.Time) string { return res + "\x00" + t.UTC().Format(time.RFC3339Nano) }
	completedAt := map[string]int{}
	for _, o := range orders {
		t, _ := when(o)
		if t.IsZero() || !strings.EqualFold(o.Status, "COMPLETED") {
			continue
		}
		for _, it := range o.Items {
			if r := strings.ToLower(it.ResourceID); r != "" {
				completedAt[tieKey(r, t)]++
			}
		}
	}
	rows := make([]subChangeRow, 0)
	cutoff := now.Add(-since)
	for _, o := range orders {
		created, from := when(o)
		status := strings.ToUpper(o.Status)
		applied := status == "COMPLETED"
		emit := (created.IsZero() || !created.Before(cutoff)) && (customerID == "" || o.CustomerID == customerID) &&
			(includeFailed || (status != "ERROR" && status != "CANCELED" && status != "CANCELLED"))
		for _, it := range o.Items {
			res := strings.ToLower(it.ResourceID)
			sub, matched := byResource[res]
			changes := decodeChanges(it)
			tie := false
			if !created.IsZero() && res != "" {
				n := completedAt[tieKey(res, created)]
				tie = (applied && n > 1) || (!applied && !openOrderStatuses[status] && n > 0)
			}
			prev, known := lastSeats[res]
			// Judge the blind for the order's customer and the subscription's
			// owner both, so data that disagrees on the owner cannot hide a blind.
			// A subscription the book does not hold has no known owner, so any
			// customer's change of unknown subscription blinds it (adversarial
			// round 27).
			owners := []string{""}
			if matched && sub.CustomerID != "" { // an ownerless synced subscription has an unknown owner too (adversarial round 28)
				owners = []string{o.CustomerID}
				if sub.CustomerID != "" {
					owners = append(owners, sub.CustomerID)
				}
				if owners[0] == "" && len(owners) > 1 {
					owners = owners[1:]
				}
			}
			for _, owner := range owners {
				known = known && !blinded(owner, setAt[res], created)
			}
			known = known && !undated[res] && !tie
			if openOrderStatuses[status] {
				// An order still open has not changed anything yet: compare with the
				// synced count, or state none when the subscription is not synced
				// (adversarial round 27, P1). A failed or cancelled order is history,
				// compared with the count before it like a completed one (Codex
				// round 27, P1).
				prev, known = sub.Seats.V, matched && sub.Seats.Set
			}
			if emit {
				label := orderItemLabel(it, sub, matched)
				for _, ch := range changes {
					if ch[0] == "seats" && len(changes) > 1 && known && it.Quantity.Set && prev == it.Quantity.V {
						continue // a renewal or name change that resent the unchanged seat count
					}
					row := subChangeRow{
						WhenFrom:     from,
						CustomerID:   o.CustomerID,
						Customer:     b.CustomerLabel(o.CustomerID, ""),
						Subscription: label,
						ResourceID:   it.ResourceID,
						Change:       ch[0],
						Value:        ch[1],
						Quantity:     optPtr(it.Quantity),
						OrderID:      o.OrderID,
						OrderStatus:  status,
						Applied:      applied,
						By:           o.UserName,
					}
					if !created.IsZero() {
						row.When = created.Format(time.RFC3339)
					}
					if ch[0] == "seats" && known {
						p := prev
						row.PreviousSeats = &p
					}
					if matched {
						row.SubscriptionID = sub.ID
					}
					rows = append(rows, row)
				}
			}
			switch {
			case !applied:
			case res == "": // judged by blinded, from the pre-scan
			case tie:
				delete(lastSeats, res)
			case it.Quantity.Set && it.Quantity.V > 0:
				lastSeats[res], setAt[res] = it.Quantity.V, created // every completed item carries the resulting seat count, tier changes included
			default:
				// No count, or a 0 no live subscription holds: the count after this
				// order is unknown (Codex rounds 18 and 19, P1: a stale count read as
				// previous_seats).
				delete(lastSeats, res)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].When != rows[j].When {
			return rows[i].When > rows[j].When
		}
		return rows[i].OrderID < rows[j].OrderID
	})
	return rows
}

// changeDate is the YYYY-MM-DD of a change, or "unknown" for an undated order
// (adversarial round 18 and Codex round 20, P1: the table sliced an empty when).
func changeDate(when string) string {
	if len(when) < 10 {
		return "unknown"
	}
	return when[:10]
}

func newNovelSubscriptionsChangesCmd(flags *rootFlags) *cobra.Command {
	var since, customer, dbPath string
	var includeFailed bool

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List seat, renewal, status, tier and friendly-name changes made through ION orders in a past window",
		Long: "List subscription changes requested through ION orders in a past window, newest first: new purchases, seat " +
			"counts, renewal settings, suspend/reactivate/delete status updates, tier changes and friendly names. Each " +
			"change is decoded from the order item's attributes and joined to the synced subscription by its vendor " +
			"subscription ID. A seats row shows the count an order set, with previous_seats when the synced order history shows the count before it, or, for an order still open, the synced subscription's current count (none when the subscription is not synced or its vendor ID is held by more than one synced subscription, which then joins neither); an order that changed only the renewal setting or name and resent the same count gets no seats row when that is known. quantity carries the item's seat count (null when ION sent none). when is the order's createTime, else its updateTime (when_from says which); an order with neither is listed with an empty when, and no previous_seats is stated for its subscription. No previous_seats is stated either for completed orders of one subscription at the same time (or the change after them), or across a completed change (other than a new purchase) of the same customer (any customer, for a subscription that is not synced or has no owner) whose subscription is unknown: a count set before or at the time of such a change is not trusted after it, and an undated one leaves that customer's counts unknown. applied is true only when the order COMPLETED; ERROR and CANCELED orders are left out unless " +
			"--include-failed. Changes made outside ION (for example directly in Partner Center) are not visible. Reads " +
			"the local store; run sync --resources orders,subscriptions first.\n\n" +
			"Use this command for subscription changes that already happened in a past window. Do NOT use this command " +
			"for upcoming renewals; use 'subscriptions renewals' instead. Do NOT use it for orders that have not " +
			"finished; use 'orders stuck' instead.",
		Example: strings.Trim(`
  td-synnex-cli subscriptions changes --since 7d --agent
  td-synnex-cli subscriptions changes --since 30d --customer 108536 --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--since=7d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "subscriptions changes")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive duration like 7d or 24h, got %q", since))
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "orders", "subscriptions", "customers")
			if err != nil {
				return err
			}
			defer closeDB()
			rows := make([]subChangeRow, 0)
			if book != nil {
				rows = subscriptionChanges(book, time.Now().UTC(), window, customer, includeFailed)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No subscription changes in the last %s.\n", since)
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{changeDate(r.When), r.Customer, r.Subscription, r.Change, r.Value, r.OrderStatus, r.OrderID})
			}
			return printTable(cmd.OutOrStdout(), []string{"DATE", "CUSTOMER", "SUBSCRIPTION", "CHANGE", "VALUE", "ORDER STATUS", "ORDER"}, table)
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "How far back to look, for example 7d, 24h or 4w")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer ID")
	cmd.Flags().BoolVar(&includeFailed, "include-failed", false, "Also list changes from ERROR and CANCELED orders")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
