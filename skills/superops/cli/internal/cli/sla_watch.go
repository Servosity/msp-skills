// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelSlaWatchCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var flagWindow string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "sla-watch",
		Short: "See which open tickets are breaching or about to breach SLA, grouped by technician or client.",
		Long: `Join locally synced tickets to their SLA targets, status, and technician to
surface every open ticket that has breached its resolution SLA or is about to
(within --window). Groups by technician (default) or client. No single SuperOps
API call answers this — it requires the local join over the synced mirror.`,
		Example: strings.Trim(`
  superops-cli sla-watch --by tech --window 4h
  superops-cli sla-watch --by client --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window, err := time.ParseDuration(flagWindow)
			if err != nil {
				return fmt.Errorf("invalid --window %q: %w (use forms like 4h, 30m, 24h)", flagWindow, err)
			}
			by := strings.ToLower(strings.TrimSpace(flagBy))
			if by != "client" {
				by = "tech"
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
			rows := computeSLAWatch(tickets, by, window, time.Now().UTC())
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintln(out, "No open tickets breaching or at risk of SLA. (Run 'sync' if this looks empty.)")
				return nil
			}
			fmt.Fprintf(out, "%-12s %-9s %-26s %s\n", "TICKET", "STATE", strings.ToUpper(by), "SUBJECT")
			for _, r := range rows {
				fmt.Fprintf(out, "%-12s %-9s %-26s %s\n", r.DisplayID, r.State, truncate(r.GroupBy, 26), truncate(r.Subject, 50))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "tech", "Group by 'tech' or 'client'")
	cmd.Flags().StringVar(&flagWindow, "window", "4h", "At-risk window before due time (e.g. 4h, 30m)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
