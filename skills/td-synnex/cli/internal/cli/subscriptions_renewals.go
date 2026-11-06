// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: book-wide renewal window over synced subscriptions.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
)

type renewalRow struct {
	CustomerID     string  `json:"customer_id"`
	Customer       string  `json:"customer"`
	SubscriptionID string  `json:"subscription_id"`
	Subscription   string  `json:"subscription"`
	SKU            string  `json:"sku"`
	Seats          float64 `json:"seats"`
	SeatsKnown     bool    `json:"seats_known"` // false when ION reported no licence count; seats is then 0
	Status         string  `json:"status"`
	RenewsOn       string  `json:"renews_on"`
	DaysLeft       int     `json:"days_left"`
	AutoRenew      string  `json:"auto_renew"`
	Term           string  `json:"term"`
	MonthlyPrice   float64 `json:"monthly_price"`
	PriceKnown     bool    `json:"price_known"` // false when ION reported no price or only part of it, or the billing cycle is not recognized; monthly_price is then 0
	MonthlyCost    float64 `json:"monthly_cost"`
	CostKnown      bool    `json:"cost_known"` // false when ION reported no cost or only part of it, or the billing cycle is not recognized; monthly_cost is then 0
	Currency       string  `json:"currency"`
}

// renewalsWithin returns active subscriptions renewing or ending in [now, now+within],
// soonest first. autoRenew filters on "on", "off" or "" (any).
func renewalsWithin(b *ionBook, now time.Time, within time.Duration, autoRenew, customerID string) []renewalRow {
	rows := make([]renewalRow, 0)
	horizon := now.Add(within)
	today := now.Truncate(24 * time.Hour) // date-only renewals parse as midnight UTC; keep the ones due today
	for _, s := range b.Subs {
		if s.Inactive() {
			continue
		}
		if customerID != "" && s.CustomerID != customerID {
			continue
		}
		when := s.RenewOn()
		if when.IsZero() || when.Before(today) || when.After(horizon) {
			continue
		}
		state := s.AutoRenewState()
		if autoRenew != "" && state != autoRenew {
			continue
		}
		m := s.MonthlyMoney()
		rows = append(rows, renewalRow{
			CustomerID:     s.CustomerID,
			Customer:       b.CustomerLabel(s.CustomerID, s.CustomerName),
			SubscriptionID: s.ID,
			Subscription:   s.Label(),
			SKU:            s.SKUKey(),
			Seats:          s.Seats.V,
			SeatsKnown:     s.Seats.Set,
			Status:         s.Status,
			RenewsOn:       when.Format("2006-01-02"),
			DaysLeft:       int(math.Ceil(when.Sub(now).Hours() / 24)),
			AutoRenew:      state,
			Term:           strings.TrimSpace(s.BillingTerm + " " + s.BillingCycle),
			MonthlyPrice:   round2(m.Price),
			PriceKnown:     m.PriceKnown,
			MonthlyCost:    round2(m.Cost),
			CostKnown:      m.CostKnown,
			Currency:       m.Currency,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].DaysLeft != rows[j].DaysLeft {
			return rows[i].DaysLeft < rows[j].DaysLeft
		}
		return rows[i].Customer < rows[j].Customer
	})
	return rows
}

func newNovelSubscriptionsRenewalsCmd(flags *rootFlags) *cobra.Command {
	var within, autoRenew, customer, dbPath string

	cmd := &cobra.Command{
		Use:   "renewals",
		Short: "See every subscription renewing or ending in the next N days across the whole book, soonest first",
		Long: "See every subscription renewing or ending in the next N days across the whole book, soonest first, " +
			"with auto-renew state, seats and monthly price and cost. A subscription with no readable renewal or end date cannot be placed in a window and is not listed. Reads the local store; run sync first.\n\n" +
			"Use this command for subscriptions that will renew or end in a coming window. Do NOT use this command " +
			"for changes that already happened; use 'subscriptions changes' instead.",
		Example: strings.Trim(`
  td-synnex-cli subscriptions renewals --within 30d --agent
  td-synnex-cli subscriptions renewals --within 45d --auto-renew off
  td-synnex-cli subscriptions renewals --within 90d --customer 108536 --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--within=30d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "subscriptions renewals")
			}
			window, err := cliutil.ParseDurationLoose(within)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--within must be a positive duration like 30d or 2w, got %q", within))
			}
			autoRenew = strings.ToLower(strings.TrimSpace(autoRenew))
			if autoRenew == "any" {
				autoRenew = ""
			}
			if autoRenew != "" && autoRenew != "on" && autoRenew != "off" {
				return usageErr(fmt.Errorf("--auto-renew must be on, off or any, got %q", autoRenew))
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "subscriptions", "customers")
			if err != nil {
				return err
			}
			defer closeDB()
			rows := make([]renewalRow, 0)
			if book != nil {
				rows = renewalsWithin(book, time.Now().UTC(), window, autoRenew, customer)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No subscriptions renew or end in the next %s.\n", within)
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{strconv.Itoa(r.DaysLeft), r.RenewsOn, r.Customer, r.Subscription,
					seatsText(r.Seats, r.SeatsKnown), r.AutoRenew, moneyText(r.MonthlyPrice, r.PriceKnown) + " " + r.Currency})
			}
			return printTable(cmd.OutOrStdout(), []string{"DAYS", "RENEWS", "CUSTOMER", "SUBSCRIPTION", "SEATS", "AUTO-RENEW", "MONTHLY"}, table)
		},
	}
	cmd.Flags().StringVar(&within, "within", "30d", "Window to look ahead, for example 30d, 6w or 90d")
	cmd.Flags().StringVar(&autoRenew, "auto-renew", "any", "Filter by auto-renew state: on, off or any")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer ID")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
