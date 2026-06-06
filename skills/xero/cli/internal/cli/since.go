// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/cliutil"
	"xero-pp-cli/internal/xfin"
)

// sinceResourceChange is the per-entity delta for the `since` command.
type sinceResourceChange struct {
	ResourceType string   `json:"resource_type"`
	ChangedCount int      `json:"changed_count"`
	ChangedIDs   []string `json:"changed_ids"`
}

type sinceReport struct {
	Since        string                `json:"since"`
	TotalChanged int                   `json:"total_changed"`
	Resources    []sinceResourceChange `json:"resources"`
}

// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var maxIDs int

	cmd := &cobra.Command{
		Use:   "since <when>",
		Short: "List records across every synced entity whose last-modified timestamp is newer than a given time",
		Long: "List records across all synced entities whose UpdatedDateUTC is newer than <when> — read\n" +
			"entirely from the local store, zero API calls. <when> is a YYYY-MM-DD date or a duration\n" +
			"ago (e.g. 24h, 7d, 1w). The org delta an agent can poll cheaply between syncs.",
		Example:     "  xero-cli since 2026-05-01 --json\n  xero-cli since 7d",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 || args[0] == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<when> is required"))
			}
			cutoff, err := parseSinceArg(args[0])
			if err != nil {
				return err
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openXeroStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			report := sinceReport{Since: cutoff.UTC().Format(time.RFC3339)}
			for _, rt := range []string{"accounts", "bank-transactions", "contacts", "invoices", "items", "journals", "payments"} {
				rows, err := loadRowsWithID(db, rt)
				if err != nil {
					return fmt.Errorf("loading %s: %w", rt, err)
				}
				change := sinceResourceChange{ResourceType: rt, ChangedIDs: []string{}}
				for _, r := range rows {
					if t, ok := xfin.UpdatedDate(r.Data); ok && t.After(cutoff) {
						change.ChangedCount++
						if len(change.ChangedIDs) < maxIDs {
							change.ChangedIDs = append(change.ChangedIDs, r.ID)
						}
					}
				}
				report.TotalChanged += change.ChangedCount
				report.Resources = append(report.Resources, change)
			}

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			out := [][]string{}
			for _, r := range report.Resources {
				out = append(out, []string{r.ResourceType, strconv.Itoa(r.ChangedCount)})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d record(s) changed since %s\n", report.TotalChanged, report.Since)
			return flags.printTable(cmd, []string{"resource", "changed"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	cmd.Flags().IntVar(&maxIDs, "max-ids", 50, "Maximum changed IDs to list per resource in JSON output")
	return cmd
}

// parseSinceArg accepts a YYYY-MM-DD date or a Go duration ("24h", "168h") and
// returns the absolute cutoff time. Durations are subtracted from now.
func parseSinceArg(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	if d, err := cliutil.ParseDurationLoose(s); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid <when> %q: use a YYYY-MM-DD date or a duration like 24h, 7d, 1w", s)
}
