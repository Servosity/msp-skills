// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type stalledDoc struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	DateModified string `json:"date_modified"`
	IdleDays     int    `json:"idle_days"`
}

// newNovelStalledCmd implements the "stalled" transcendence command:
// documents sent or viewed but not completed within N days, ranked by idle time.
// pp:data-source local
func newNovelStalledCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var statusCSV string
	var limit int
	cmd := &cobra.Command{
		Use:         "stalled",
		Short:       "Find documents that were sent but never completed within N days — the deals quietly dying.",
		Long:        "Find documents that were sent but never completed within N days — the deals quietly dying.\n\nUse this command for documents SENT and never completed within N days. Do NOT use it for time-in-current-status bucketing; use 'aging' instead. Do NOT use it when you need recipient emails attached for outreach; use 'followup' instead.\n\nReads the local store — run `sync` first.",
		Example:     "  pandadoc-cli stalled --status sent,viewed --days 5 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pandadoc-cli")
			}
			docs, err := loadDocumentsHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			want := parseStatusList(statusCSV)
			cutoff := time.Now().UTC().AddDate(0, 0, -days)
			results := make([]stalledDoc, 0)
			for _, d := range docs {
				if len(want) > 0 && !want[normalizeStatus(d.Status)] {
					continue
				}
				if isTerminalStatus(d.Status) {
					continue
				}
				activity := d.lastActivity()
				if activity.IsZero() || activity.After(cutoff) {
					continue
				}
				results = append(results, stalledDoc{
					ID:           d.ID,
					Name:         d.Name,
					Status:       shortStatus(d.Status),
					DateModified: d.DateModified.Format(time.RFC3339),
					IdleDays:     int(time.Since(activity).Hours() / 24),
				})
			}
			sort.Slice(results, func(i, j int) bool { return results[i].IdleDays > results[j].IdleDays })
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, results, flags)
			}
			if len(results) == 0 {
				fmt.Fprintf(w, "No stalled documents (status in [%s] idle > %d days).\n", statusCSV, days)
				return nil
			}
			fmt.Fprintf(w, "Stalled documents (idle > %d days):\n\n", days)
			for _, r := range results {
				fmt.Fprintf(w, "  %4dd  %-12s %s  %s\n", r.IdleDays, r.Status, r.ID, r.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&days, "days", 14, "Idle-days threshold: documents not modified in this many days")
	cmd.Flags().StringVar(&statusCSV, "status", "sent,viewed", "Comma-separated statuses to consider stalled (e.g. sent,viewed)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum results to return (0 = no limit)")
	return cmd
}
