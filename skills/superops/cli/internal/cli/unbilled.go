// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelUnbilledCmd(flags *rootFlags) *cobra.Command {
	var flagClient string
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unbilled",
		Short: "Find logged worklog time that is billable, totaled per client.",
		Long: `Resolve each billable worklog entry to its client (worklog -> ticket -> client)
and total billable minutes per client. Use it at month-end to see where billable
time is concentrated before invoicing.

Note: the SuperOps list API does not expose a per-entry "already billed" flag, so
this surfaces billable logged time per client (the reconciliation target) rather
than a strict worklog-minus-invoice diff. See README "Known Gaps".`,
		Example: strings.Trim(`
  superops-cli unbilled
  superops-cli unbilled --client Acme --since 2026-05-01 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			var since time.Time
			if strings.TrimSpace(flagSince) != "" {
				ts, ok := parseSuperopsTime(flagSince)
				if !ok {
					return fmt.Errorf("invalid --since %q: use ISO-8601 like 2026-05-01 or 2026-05-01T00:00:00", flagSince)
				}
				since = ts
			}
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			worklogs, err := queryRecs(db, "worklogs")
			if err != nil {
				return err
			}
			tickets, err := queryRecs(db, "tickets")
			if err != nil {
				return err
			}
			rows := computeUnbilled(worklogs, tickets, flagClient, since)
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintln(out, "No billable worklog time found. (Run 'sync' if this looks empty.)")
				return nil
			}
			fmt.Fprintf(out, "%-32s %8s %10s\n", "CLIENT", "ENTRIES", "HOURS")
			for _, r := range rows {
				fmt.Fprintf(out, "%-32s %8d %10.1f\n", truncate(r.Client, 32), r.BillableEntries, r.BillableHours)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClient, "client", "", "Limit to a single client (by name)")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only count worklog started on/after this ISO date")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
