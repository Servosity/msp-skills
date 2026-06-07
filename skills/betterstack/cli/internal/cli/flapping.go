// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): flapping / noisy-monitor ranking from the local mirror.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type flapEntry struct {
	Source    string `json:"source"`
	Incidents int    `json:"incidents"`
	LastSeen  string `json:"last_incident_at,omitempty"`
}

// pp:data-source local
func newNovelFlappingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var top int

	cmd := &cobra.Command{
		Use:         "flapping",
		Short:       "Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.",
		Long:        "Use this command to rank monitors by how many incidents they generated (alert-fatigue sources). Do NOT use it for response-time math; use 'mttr' instead. Counts incidents per source (monitor/heartbeat) over the window from the local mirror and ranks the noisiest. Run `sync` first.",
		Example:     "  betterstack-cli flapping --days 7 --top 10\n  betterstack-cli flapping --days 30 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if days <= 0 {
				days = 7
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "incidents", flags.maxAge)

			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}

			cutoff := time.Now().AddDate(0, 0, -days)
			counts := map[string]int{}
			last := map[string]time.Time{}
			for _, inc := range incidents {
				started, ok := parseTime(inc.StartedAt)
				if !ok || started.Before(cutoff) {
					continue
				}
				counts[inc.Source]++
				if started.After(last[inc.Source]) {
					last[inc.Source] = started
				}
			}

			entries := make([]flapEntry, 0, len(counts))
			for src, n := range counts {
				e := flapEntry{Source: src, Incidents: n}
				if t, ok := last[src]; ok {
					e.LastSeen = t.UTC().Format(time.RFC3339)
				}
				entries = append(entries, e)
			}
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Incidents != entries[j].Incidents {
					return entries[i].Incidents > entries[j].Incidents
				}
				return entries[i].Source < entries[j].Source
			})
			if top > 0 && len(entries) > top {
				entries = entries[:top]
			}

			if flags.asJSON {
				return flags.printJSON(cmd, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No incidents in the last %d days (or local mirror empty — run `sync`).\n", days)
				return nil
			}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				rows = append(rows, []string{truncateField(e.Source, 48), fmt.Sprintf("%d", e.Incidents), e.LastSeen})
			}
			return flags.printTable(cmd, []string{"SOURCE", "INCIDENTS", "LAST INCIDENT"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	cmd.Flags().IntVar(&days, "days", 7, "Window in days to count incidents over")
	cmd.Flags().IntVar(&top, "top", 10, "Show only the top N noisiest sources")
	return cmd
}
