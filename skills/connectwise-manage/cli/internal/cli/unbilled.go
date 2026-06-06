// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelUnbilledCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagMember string
	var flagThreshold float64
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "unbilled",
		Short: "Find tickets touched in a window with zero or under-threshold time logged.",
		Long: strings.Trim(`
Cross-references locally synced service tickets against logged time entries
(joining time_entries.chargeToId to the ticket id) and lists tickets whose
total logged hours are at or below a threshold — the unbilled work the PSA has
no single call for. Run 'sync service-tickets time-entries' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli unbilled --since 7d
  connectwise-manage-cli unbilled --member jdoe --since 1d --agent
  connectwise-manage-cli unbilled --threshold 0.25 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			var since time.Time
			if strings.TrimSpace(flagSince) != "" {
				t, err := parseSinceDuration(flagSince)
				if err != nil {
					return err
				}
				since = t
			}
			now := time.Now()

			headers := []string{"ID", "Summary", "Company", "Owner", "Hours"}
			db, err := cwOpenStore(cmd.Context())
			if err != nil {
				return cwNoStoreHint(cmd, flags, []ticketRow{}, headers, "service-tickets time-entries")
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, cwTickets) {
				hintIfStale(cmd, db, cwTickets, flags.maxAge)
			}

			tickets, err := cwLoad(cmd.Context(), db, cwTickets)
			if err != nil {
				return err
			}
			times, err := cwLoad(cmd.Context(), db, cwTimeEntries)
			if err != nil {
				return err
			}

			rows := computeUnbilled(tickets, times, since, flagMember, flagThreshold, now)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{
					cwItoa(r.ID), cwTrunc(r.Summary, 48), r.Company, r.Owner, cwFtoa(r.HoursLogged),
				})
			}
			return cwEmit(cmd, flags, rows, headers, table)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only tickets updated within this window (e.g. 7d, 24h, 1w)")
	cmd.Flags().StringVar(&flagMember, "member", "", "Limit to tickets owned by this member identifier")
	cmd.Flags().Float64Var(&flagThreshold, "threshold", 0, "Flag tickets at or below this many logged hours (default 0 = no time logged)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
