// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): the signal-to-noise view that
// finds a flapping integration. Ranks alert sources and services by volume,
// repeat-fire rate, and alert→incident conversion — no Rootly view exposes
// noise ratios.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelAlertNoiseCmd(flags *rootFlags) *cobra.Command {
	var days int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "alert-noise",
		Short: "Rank alert sources by volume, repeat-fire rate, and alert-to-incident conversion.",
		Long: `Find what is drowning on-call. Reads synced alerts from the local mirror,
groups them by source over the lookback window, measures repeat-fire (the same
normalized alert text firing 2+ times), and computes how many alerts ever
became incidents. A source with high volume, high repeats, and low conversion
is the flapping integration to fix. Offline — run
'rootly-cli sync --resources alerts,alert-sources' first.`,
		Example: `  rootly-cli alert-noise --days 14
  rootly-cli alert-noise --days 7 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if days <= 0 {
				days = 14
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "alerts")
			if err != nil {
				return err
			}
			defer db.Close()

			alerts, err := novelLoad(db, novelResolveType(db, "alerts"))
			if err != nil {
				return err
			}

			now := time.Now()
			windowStart := now.Add(-time.Duration(days) * 24 * time.Hour)

			type srcAgg struct {
				Source       string  `json:"source"`
				Alerts       int     `json:"alerts"`
				RepeatGroups int     `json:"repeat_groups"`
				MaxRepeat    int     `json:"max_repeat"`
				Converted    int     `json:"converted_to_incidents"`
				Conversion   float64 `json:"conversion_pct"`
				keys         map[string]int
			}
			bySource := map[string]*srcAgg{}
			scanned := 0
			for _, a := range alerts {
				created, ok := recTime(a.Attrs, "created_at", "started_at", "triggered_at")
				if !ok || created.Before(windowStart) || created.After(now) {
					continue
				}
				scanned++
				source := firstNonEmpty(
					recName(a.Attrs["alert_source"]), recName(a.Rels["alert_source"]),
					recStr(a.Attrs, "source", "alert_source_name", "integration"),
					"(unknown)")
				agg := bySource[source]
				if agg == nil {
					agg = &srcAgg{Source: source, keys: map[string]int{}}
					bySource[source] = agg
				}
				agg.Alerts++
				if key := alertNoiseKey(recStr(a.Attrs, "summary", "title", "description")); key != "" {
					agg.keys[key]++
				}
				converted := relID(a, "incident") != "" || recStr(a.Attrs, "incident_id") != ""
				if !converted {
					if ids := recIDs(a.Attrs["incidents"]); len(ids) > 0 {
						converted = true
					}
				}
				if converted {
					agg.Converted++
				}
			}

			rows := make([]srcAgg, 0, len(bySource))
			for _, agg := range bySource {
				for _, n := range agg.keys {
					if n > 1 {
						agg.RepeatGroups++
						if n > agg.MaxRepeat {
							agg.MaxRepeat = n
						}
					}
				}
				if agg.Alerts > 0 {
					agg.Conversion = float64(int(float64(agg.Converted)/float64(agg.Alerts)*1000)) / 10
				}
				rows = append(rows, *agg)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Alerts == rows[j].Alerts {
					return rows[i].Source < rows[j].Source
				}
				return rows[i].Alerts > rows[j].Alerts
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			if rows == nil {
				rows = []srcAgg{}
			}

			out := struct {
				WindowDays    int      `json:"window_days"`
				ScannedAlerts int      `json:"scanned_alerts"`
				Sources       []srcAgg `json:"sources"`
				Note          string   `json:"note,omitempty"`
			}{WindowDays: days, ScannedAlerts: scanned, Sources: rows}
			if scanned == 0 {
				out.Note = fmt.Sprintf("no alerts in the last %d days found in the local store; run 'rootly-cli sync --resources alerts' first", days)
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "alert noise, last %d days (%d alerts)\n", days, scanned)
				if scanned == 0 {
					fmt.Fprintf(w, "  %s\n", out.Note)
					return
				}
				for _, r := range rows {
					fmt.Fprintf(w, "  %-30.30s %5d alerts  %3d repeat-groups (max %d)  %5.1f%% became incidents\n",
						r.Source, r.Alerts, r.RepeatGroups, r.MaxRepeat, r.Conversion)
				}
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Lookback window in days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum sources to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
