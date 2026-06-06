// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelAgreementBurnCmd(flags *rootFlags) *cobra.Command {
	var flagPeriod string
	var flagCompany string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "agreement-burn",
		Short: "Hours logged against each agreement vs its allotment (utilization %).",
		Long: strings.Trim(`
Joins synced agreements to logged time entries (by agreement id) and reports
hours used vs the agreement's hour allotment as a utilization percentage with
an over-limit flag. Agreements whose unit is not Hours report used hours only.
Run 'sync finance-agreements time-entries' first.

Use this command to measure hours logged against an agreement's allotment as a
utilization percentage. Do NOT use this command for general company context;
use 'account' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli agreement-burn
  connectwise-manage-cli agreement-burn --company AcmeCorp --period 30d
  connectwise-manage-cli agreement-burn --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			var since time.Time
			if strings.TrimSpace(flagPeriod) != "" {
				t, err := parseSinceDuration(flagPeriod)
				if err != nil {
					return err
				}
				since = t
			}
			headers := []string{"ID", "Agreement", "Company", "Used(h)", "Limit", "Util%"}

			db, err := cwOpenStore(cmd.Context())
			if err != nil {
				return cwNoStoreHint(cmd, flags, []burnRow{}, headers, "finance-agreements time-entries")
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, cwAgreements) {
				hintIfStale(cmd, db, cwAgreements, flags.maxAge)
			}

			agreements, err := cwLoad(cmd.Context(), db, cwAgreements)
			if err != nil {
				return err
			}
			times, err := cwLoad(cmd.Context(), db, cwTimeEntries)
			if err != nil {
				return err
			}
			rows := computeAgreementBurn(agreements, times, flagCompany, since)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				util := ""
				if r.Limit > 0 && strings.EqualFold(r.Units, "Hours") {
					util = cwFtoa(r.Pct) + "%"
					if r.Over {
						util += " OVER"
					}
				}
				limit := ""
				if r.Limit > 0 {
					limit = cwFtoa(r.Limit)
				}
				table = append(table, []string{
					cwItoa(r.AgreementID), cwTrunc(r.Name, 32), r.Company, cwFtoa(r.UsedHours), limit, util,
				})
			}
			return cwEmit(cmd, flags, rows, headers, table)
		},
	}
	cmd.Flags().StringVar(&flagPeriod, "period", "", "Only count time entered within this window (e.g. 30d, 1w)")
	cmd.Flags().StringVar(&flagCompany, "company", "", "Limit to one company (identifier or name)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
