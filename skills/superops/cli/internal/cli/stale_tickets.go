// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelStaleTicketsCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale-tickets",
		Short: "Open tickets with no conversation, note, or worklog activity in N days.",
		Long: `Compute, per open ticket, the most recent activity timestamp (the later of the
ticket's updatedTime and any worklog entry logged against it) and surface tickets
whose last activity is older than --days. The SuperOps API will not aggregate
child-activity timestamps; this is computed locally over the synced mirror.`,
		Example: strings.Trim(`
  superops-cli stale-tickets --days 7
  superops-cli stale-tickets --days 14 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagDays <= 0 {
				return fmt.Errorf("--days must be a positive integer")
			}
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			tickets, err := queryRecs(db, "tickets")
			if err != nil {
				return err
			}
			worklogs, err := queryRecs(db, "worklogs")
			if err != nil {
				return err
			}
			rows := computeStaleTickets(tickets, worklogs, flagDays, time.Now().UTC())
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintf(out, "No open tickets idle more than %d days. (Run 'sync' if this looks empty.)\n", flagDays)
				return nil
			}
			fmt.Fprintf(out, "%-12s %5s %-22s %s\n", "TICKET", "IDLE", "TECHNICIAN", "SUBJECT")
			for _, r := range rows {
				fmt.Fprintf(out, "%-12s %4dd %-22s %s\n", r.DisplayID, r.IdleDays, truncate(r.Technician, 22), truncate(r.Subject, 46))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 7, "Idle-days threshold")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
