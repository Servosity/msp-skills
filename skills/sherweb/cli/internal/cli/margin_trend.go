// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelMarginTrendCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer string
	var flagLast int

	cmd := &cobra.Command{
		Use:   "margin-trend",
		Short: "See each customer's margin across the last N monthly closes so profitability slides surface early.",
		Long: `Use this command for a per-customer margin time-series across multiple monthly snapshots.
Do NOT use this command for a single month's margin; use 'margin' instead.
Do NOT use this command for line-item charge changes; use 'drift' instead.

Each month's margin pairs the latest payable snapshot with the latest receivable
snapshot from that month (both written by 'sherweb-cli deep-sync'). Months
missing either fleet-wide snapshot are skipped so an unrecorded snapshot never
reads as a loss; within a kept month, a customer absent from one side still
shows that side as 0 (same semantics as 'margin'). Rows sort
steepest-decline-first.`,
		Example:     "  sherweb-cli margin-trend --last 6 --agent\n  sherweb-cli margin-trend --customer 0aa00000-0000-0000-0000-000000000000 --last 6",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			s, err := openInsightStore(cmd.Context())
			if err != nil {
				return err
			}
			defer s.Close()

			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}

			payIDs, payByID, err := loadPayableSnapshots(s)
			if err != nil {
				return fmt.Errorf("loading payable snapshots: %w", err)
			}
			recvIDs, recvByID, err := loadReceivableSnapshots(s)
			if err != nil {
				return fmt.Errorf("loading receivable snapshots: %w", err)
			}
			payByMonth := latestPerMonth(payIDs, payByID)
			recvByMonth := latestPerMonth(recvIDs, recvByID)

			var months []string
			for m := range payByMonth {
				if _, ok := recvByMonth[m]; ok {
					months = append(months, m)
				}
			}
			sort.Strings(months)
			if flagLast > 0 && len(months) > flagLast {
				months = months[len(months)-flagLast:]
			}

			rows := insights.ComputeMarginTrend(months, payByMonth, recvByMonth)
			if flagCustomer != "" {
				filtered := make([]insights.MarginTrendRow, 0, len(rows))
				for _, r := range rows {
					if r.CustomerID == flagCustomer || r.CustomerName == flagCustomer {
						filtered = append(filtered, r)
					}
				}
				rows = filtered
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no margin history — each 'sherweb-cli deep-sync' run appends one payable + receivable snapshot; at least one month needs both")
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Limit to a single customer (id or display name)")
	cmd.Flags().IntVar(&flagLast, "last", 0, "Only include the most recent N months (0 = all)")
	return cmd
}
