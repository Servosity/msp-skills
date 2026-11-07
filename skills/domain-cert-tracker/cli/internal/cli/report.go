// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source local

package cli

import (
	"time"

	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelReportCmd(flags *rootFlags) *cobra.Command {
	var client, days, window string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "See, per client, what expires within N days, which registrations lost their lock, new CT certificates and DNS drift, plus what the last sweep could not check.",
		Long: `Use this command for the per-client summary of what expires within --days, lost
locks, new CT entries, DNS drift and unverified checks from the last sweep. Do
NOT use this command to find certificates renewed but never deployed, or to
separate unexpected issuers from routine renewals; use 'renewals' or 'issuers
audit' instead.

Reads the local store only (run 'sweep' first). Every check whose latest result
failed, or that never ran, is listed under unverified: an empty section means
"checked and clean" only when the check is not listed there.`,
		Example:     "  domain-cert-tracker-cli report --days 30 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			d, err := parseDaysFlag("days", days)
			if err != nil {
				return err
			}
			w, err := parseDaysFlag("window", window)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "report")
			}
			now := time.Now().UTC()
			db, err := openTrackerRead(cmd.Context())
			if err != nil {
				return err
			}
			if db == nil {
				return flags.printJSON(cmd, tracker.Report{Days: d, ChangesFrom: now.AddDate(0, 0, -w).Format(time.RFC3339),
					Clients: []tracker.ClientReport{}, Totals: tracker.EmptyTotals()})
			}
			defer db.Close()
			rep, err := tracker.BuildReport(cmd.Context(), db, client, d, now.AddDate(0, 0, -w), now)
			if err != nil {
				return trackerErr(err)
			}
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "Only this inventoried client")
	cmd.Flags().StringVar(&days, "days", "30", "Expiry horizon in days (30 or 30d)")
	cmd.Flags().StringVar(&window, "window", "7d", "Show CT and DNS changes raised within this many days")
	return cmd
}
