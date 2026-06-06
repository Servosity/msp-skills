// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelAlertCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagClient string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "alert-coverage",
		Short: "Partition alerts into open (uncovered) vs resolved, grouped by client.",
		Long: `Group synced alerts by client (via alert -> asset -> client) and split them into
resolved ("covered") vs unresolved ("uncovered / needs action") so you can see
which clients have alerts still sitting unhandled.

Note: SuperOps does not expose the exact alert -> ticket linkage on its list
payloads, so resolved-vs-open status is the synced proxy for coverage rather than
a literal "did this alert become a ticket" check. See README "Known Gaps".`,
		Example: strings.Trim(`
  superops-cli alert-coverage
  superops-cli alert-coverage --client Acme --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			alerts, err := queryRecs(db, "alerts")
			if err != nil {
				return err
			}
			assets, err := queryRecs(db, "assets")
			if err != nil {
				return err
			}
			rows := computeAlertCoverage(alerts, assets, flagClient)
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintln(out, "No alerts found. (Run 'sync' if this looks empty.)")
				return nil
			}
			fmt.Fprintf(out, "%-32s %6s %9s %6s\n", "CLIENT", "OPEN", "RESOLVED", "TOTAL")
			for _, r := range rows {
				fmt.Fprintf(out, "%-32s %6d %9d %6d\n", truncate(r.Client, 32), r.Open, r.Resolved, r.Total)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClient, "client", "", "Limit to a single client (by name)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
