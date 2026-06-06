// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelPasswordsStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var flagOrg string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List credentials not updated within --days, grouped by client, oldest-first (metadata only)",
		Long: `List password entries whose last update is older than --days, oldest-first,
for credential-rotation hygiene (SOC2 / cyber-insurance attestation).

Reads only the local store and only password metadata — the secret value is
never read or printed. A live fleet-wide scan would hit IT Glue's
3000-requests/5-minute ceiling; this is rate-ceiling-proof. Entries with no
recorded update time are surfaced first as unknown-age.

Use this command for credential rotation/hygiene — passwords not updated in N
days, fleet-wide. Do NOT use it for a general "what changed recently" sweep;
use 'changes' instead.`,
		Example: `  # Credentials not rotated in a year
  itglue-cli passwords stale --days 365 --agent

  # One client's six-month rotation audit
  itglue-cli passwords stale --days 180 --org 12345`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagDays < 0 {
				return usageErr(fmt.Errorf("--days must be >= 0"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "passwords") {
				hintIfStale(cmd, db, "passwords", flags.maxAge)
			}

			recs, err := listITGRecords(db, "passwords")
			if err != nil {
				return apiErr(fmt.Errorf("reading passwords: %w", err))
			}

			now := time.Now()
			cutoff := now.Add(-time.Duration(flagDays) * 24 * time.Hour)

			type staleRow struct {
				row     map[string]any
				days    int
				unknown bool
			}
			var stale []staleRow
			for _, rec := range recs {
				if flagOrg != "" && rec.orgID() != flagOrg {
					continue
				}
				ts, ok := rec.updatedAt()
				row := rec.summary()
				if !ok {
					row["days_since_update"] = nil
					stale = append(stale, staleRow{row: row, unknown: true})
					continue
				}
				if !ts.Before(cutoff) {
					continue // fresher than the threshold
				}
				days := int(now.Sub(ts).Hours() / 24)
				row["days_since_update"] = days
				stale = append(stale, staleRow{row: row, days: days})
			}

			sort.SliceStable(stale, func(i, j int) bool {
				if stale[i].unknown != stale[j].unknown {
					return stale[i].unknown // unknown-age first
				}
				return stale[i].days > stale[j].days // oldest first
			})

			out := make([]map[string]any, 0, len(stale))
			for _, s := range stale {
				out = append(out, s.row)
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 365, "Flag passwords not updated within this many days")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Limit to a single organization id")
	return cmd
}
