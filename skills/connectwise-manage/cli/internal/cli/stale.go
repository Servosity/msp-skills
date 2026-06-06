// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var flagBoard string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Open tickets with no update in N days, oldest first.",
		Long: strings.Trim(`
Lists open service tickets from the local store whose last update is older than
--days, oldest first — the daily "what's rotting on my board" pass. Optionally
scope to one board. Run 'sync service-tickets' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli stale --days 5
  connectwise-manage-cli stale --days 3 --board "Help Desk"
  connectwise-manage-cli stale --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagDays <= 0 {
				flagDays = 7
			}
			now := time.Now()
			olderThan := now.AddDate(0, 0, -flagDays)
			headers := []string{"ID", "Age(d)", "Board", "Owner", "Summary"}

			db, err := cwOpenStore(cmd.Context())
			if err != nil {
				return cwNoStoreHint(cmd, flags, []ticketRow{}, headers, "service-tickets")
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, cwTickets) {
				hintIfStale(cmd, db, cwTickets, flags.maxAge)
			}

			tickets, err := cwLoad(cmd.Context(), db, cwTickets)
			if err != nil {
				return err
			}
			rows := computeStale(tickets, olderThan, flagBoard, now)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				owner := r.Owner
				if owner == "" {
					owner = "(unassigned)"
				}
				table = append(table, []string{
					cwItoa(r.ID), cwFtoa(r.AgeDays), r.Board, owner, cwTrunc(r.Summary, 44),
				})
			}
			return cwEmit(cmd, flags, rows, headers, table)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 7, "Tickets with no update in at least this many days")
	cmd.Flags().StringVar(&flagBoard, "board", "", "Limit to a board (name or id)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
