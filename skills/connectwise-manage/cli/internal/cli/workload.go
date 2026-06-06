// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelWorkloadCmd(flags *rootFlags) *cobra.Command {
	var flagBoard string

	cmd := &cobra.Command{
		Use:   "workload",
		Short: "Open ticket count and oldest age per tech, so you route to whoever is lightest.",
		Long: strings.Trim(`
Aggregates open service tickets from the local store by owner, with the count
and the oldest open age per tech — the per-tech load the PSA has no single call
for. Optionally scope to one board. Run 'sync service-tickets' first.

Use this command to see open-ticket load and aging per tech, to decide who
takes the next ticket. Do NOT use this command to view one board's queue
grouped by status; use 'board' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli workload
  connectwise-manage-cli workload --board "Help Desk" --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			now := time.Now()
			headers := []string{"Owner", "OpenTickets", "OldestAge(d)"}

			db, err := cwOpenStore(cmd.Context())
			if err != nil {
				return cwNoStoreHint(cmd, flags, []workloadRow{}, headers, "service-tickets")
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, cwTickets) {
				hintIfStale(cmd, db, cwTickets, flags.maxAge)
			}

			tickets, err := cwLoad(cmd.Context(), db, cwTickets)
			if err != nil {
				return err
			}
			rows := computeWorkload(tickets, flagBoard, now)

			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Owner, cwItoa(r.OpenTickets), cwFtoa(r.OldestDays)})
			}
			return cwEmit(cmd, flags, rows, headers, table)
		},
	}
	cmd.Flags().StringVar(&flagBoard, "board", "", "Limit to a board (name or id)")
	return cmd
}
