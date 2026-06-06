// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelBoardCmd(flags *rootFlags) *cobra.Command {
	var flagUnassigned bool
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "board [board-id-or-name]",
		Short: "Grouped triage view of a service board's open tickets (age, owner, priority).",
		Long: strings.Trim(`
Shows the open tickets on a service board from the local store, oldest first,
with each ticket's age, owner, status, and priority — joined to the synced
reference data. Pass a board id or name; omit it to see every open ticket.
Run 'sync service-tickets' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli board 2
  connectwise-manage-cli board "Help Desk" --unassigned
  connectwise-manage-cli board 2 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			board := ""
			if len(args) > 0 {
				board = args[0]
			}
			now := time.Now()
			headers := []string{"ID", "Status", "Age(d)", "Owner", "Priority", "Summary"}

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
			rows := computeBoard(tickets, board, flagUnassigned, now)
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
					cwItoa(r.ID), r.Status, cwFtoa(r.AgeDays), owner, r.Priority, cwTrunc(r.Summary, 40),
				})
			}
			return cwEmit(cmd, flags, rows, headers, table)
		},
	}
	cmd.Flags().BoolVar(&flagUnassigned, "unassigned", false, "Only show tickets with no owner")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
