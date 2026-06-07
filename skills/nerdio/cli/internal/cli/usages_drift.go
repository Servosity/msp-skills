// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: usages drift (per-account consumption delta between periods).
// pp:data-source live

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// usageDriftRow is one account's usage movement between the two periods.
type usageDriftRow struct {
	AccountID int64    `json:"account_id"`
	Account   string   `json:"account"`
	FromTotal float64  `json:"from_total"`
	ToTotal   float64  `json:"to_total"`
	DeltaPct  *float64 `json:"delta_pct"` // null when from_total is 0 (new usage, pct undefined)
	Flagged   bool     `json:"flagged"`
}

// usageDriftView is the JSON envelope for usages drift.
type usageDriftView struct {
	FromPeriod    string              `json:"from_period"`
	ToPeriod      string              `json:"to_period"`
	MinPct        float64             `json:"min_pct"`
	Rows          []usageDriftRow     `json:"rows"`
	FlaggedCount  int                 `json:"flagged_count"`
	FetchFailures []fleetFetchFailure `json:"fetch_failures,omitempty"`
	Note          string              `json:"note,omitempty"`
}

func newNovelUsagesDriftCmd(flags *rootFlags) *cobra.Command {
	var flagFrom string
	var flagTo string
	var flagMinPct float64

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Flag accounts whose usage moved beyond a threshold between two periods",
		Long: strings.Trim(`
Fetches /usages for two periods and computes each account's consumption delta,
flagging accounts whose usage grew or shrank beyond --min-pct. Accounts with
usage in only one period are included (delta_pct is null when the baseline is
zero - new usage has no percentage). This is the "which customers spiked
month-over-month" question as one command; the API only returns one window at
a time. For a single-period billed/unpaid rollup use 'fleet billing-rollup'
instead.
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31
  nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31 --min-pct 20 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--from=2026-04-01:2026-04-30;--to=2026-05-01:2026-05-31",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compare per-account usage between the two periods")
				return nil
			}
			if flagFrom == "" || flagTo == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--from and --to are both required (e.g. --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31)"))
			}
			fromStart, fromEnd, err := parsePeriod(flagFrom)
			if err != nil {
				return usageErr(err)
			}
			toStart, toEnd, err := parsePeriod(flagTo)
			if err != nil {
				return usageErr(err)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			view := usageDriftView{FromPeriod: flagFrom, ToPeriod: flagTo, MinPct: flagMinPct, Rows: make([]usageDriftRow, 0)}

			fetchTotals := func(label, start, end string) map[int64]float64 {
				totals := map[int64]float64{}
				data, err := c.Get(ctx, "/rest-api/v1/usages", map[string]string{
					"startDate": start, "endDate": end, "withDetails": "true",
				})
				if err != nil {
					view.FetchFailures = append(view.FetchFailures, fleetFetchFailure{Account: label, Error: err.Error()})
					return totals
				}
				for _, usage := range decodeObjects(data) {
					accountID, ok := extractIntAny(usage, "accountId", "nmmId", "account", "customerId")
					if !ok {
						continue
					}
					if amount, ok := extractNumberAny(usage, "total", "amount", "totalAmount", "cost", "quantity", "usage"); ok {
						totals[accountID] += amount
					}
				}
				return totals
			}

			fromTotals := fetchTotals("from-period", fromStart, fromEnd)
			toTotals := fetchTotals("to-period", toStart, toEnd)
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 2 usage fetches failed; drift is partial\n", len(view.FetchFailures))
			}

			names := map[int64]string{}
			for _, a := range fleetAccountsFromStore(ctx, cmd, flags) {
				names[a.ID] = a.Name
			}

			ids := map[int64]bool{}
			for id := range fromTotals {
				ids[id] = true
			}
			for id := range toTotals {
				ids[id] = true
			}
			ordered := make([]int64, 0, len(ids))
			for id := range ids {
				ordered = append(ordered, id)
			}
			sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

			for _, id := range ordered {
				name := names[id]
				if name == "" {
					name = fmt.Sprintf("account-%d", id)
				}
				row := usageDriftRow{AccountID: id, Account: name, FromTotal: fromTotals[id], ToTotal: toTotals[id]}
				if row.FromTotal != 0 {
					pct := (row.ToTotal - row.FromTotal) / math.Abs(row.FromTotal) * 100
					row.DeltaPct = &pct
					if math.Abs(pct) >= flagMinPct {
						row.Flagged = true
					}
				} else if row.ToTotal != 0 {
					row.Flagged = true // new usage from a zero baseline is always notable
				}
				view.Rows = append(view.Rows, row)
				if row.Flagged {
					view.FlaggedCount++
				}
			}
			if len(view.Rows) == 0 && len(view.FetchFailures) == 0 {
				view.Note = "no usage rows with extractable account totals in either period"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagFrom, "from", "", "Baseline period as <start>:<end> dates (e.g. 2026-04-01:2026-04-30)")
	cmd.Flags().StringVar(&flagTo, "to", "", "Comparison period as <start>:<end> dates (e.g. 2026-05-01:2026-05-31)")
	cmd.Flags().Float64Var(&flagMinPct, "min-pct", 20, "Flag accounts whose absolute usage delta meets or exceeds this percentage")
	return cmd
}
