// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): MTTR / MTTA analytics computed
// from synced incident timestamps. The Rootly spec has no analytics endpoint —
// this is the weekly ops-review spreadsheet as one local query.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelMttrCmd(flags *rootFlags) *cobra.Command {
	var by string
	var since string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "mttr",
		Short: "Compute MTTR / MTTA from incident timestamps, optionally grouped.",
		Long: `Compute mean-time-to-acknowledge (MTTA) and mean-time-to-resolve (MTTR) from
synced incident timestamps. Group by service, team, or severity, and limit to a
recent window with --since. Aggregation over the local mirror — there is no
analytics endpoint in the Rootly API.`,
		Example: `  rootly-cli mttr --by service --since 30d
  rootly-cli mttr --by severity --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			switch by {
			case "", "service", "team", "severity":
			default:
				return usageErr(fmt.Errorf("--by must be one of: service, team, severity (got %q)", by))
			}

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}

			var cutoff time.Time
			if d, ok := parseWindowDuration(since); ok {
				cutoff = time.Now().Add(-d)
			}

			// accumulator per group key
			type acc struct {
				count  int
				ttrSum time.Duration
				ttrN   int
				ttaSum time.Duration
				ttaN   int
			}
			groups := map[string]*acc{}
			keyOf := func(r record) []string {
				switch by {
				case "service":
					n := incidentServiceNames(r)
					if len(n) == 0 {
						return []string{"(no service)"}
					}
					return n
				case "team":
					n := incidentTeamNames(r)
					if len(n) == 0 {
						return []string{"(no team)"}
					}
					return n
				case "severity":
					s := incidentSeverity(r)
					if s == "" {
						s = "(no severity)"
					}
					return []string{s}
				default:
					return []string{"all"}
				}
			}

			for _, r := range incidents {
				start, hasStart := incidentStart(r)
				if !hasStart {
					continue
				}
				if !cutoff.IsZero() && start.Before(cutoff) {
					continue
				}
				var ttr, tta time.Duration
				hasTTR, hasTTA := false, false
				if resolved, ok := incidentResolved(r); ok && resolved.After(start) {
					ttr = resolved.Sub(start)
					hasTTR = true
				}
				if ack, ok := recTime(r.Attrs, "acknowledged_at"); ok && !ack.Before(start) {
					tta = ack.Sub(start)
					hasTTA = true
				}
				for _, k := range keyOf(r) {
					a := groups[k]
					if a == nil {
						a = &acc{}
						groups[k] = a
					}
					a.count++
					if hasTTR {
						a.ttrSum += ttr
						a.ttrN++
					}
					if hasTTA {
						a.ttaSum += tta
						a.ttaN++
					}
				}
			}

			type row struct {
				Key         string `json:"key"`
				Incidents   int    `json:"incidents"`
				Resolved    int    `json:"resolved"`
				MTTRMinutes int    `json:"mttr_minutes"`
				MTTAMinutes int    `json:"mtta_minutes"`
				MTTRHuman   string `json:"mttr_human"`
				MTTAHuman   string `json:"mtta_human"`
			}
			var rows []row
			for k, a := range groups {
				r := row{Key: k, Incidents: a.count, Resolved: a.ttrN}
				if a.ttrN > 0 {
					avg := a.ttrSum / time.Duration(a.ttrN)
					r.MTTRMinutes = roundMinutes(avg)
					r.MTTRHuman = humanDuration(avg)
				}
				if a.ttaN > 0 {
					avg := a.ttaSum / time.Duration(a.ttaN)
					r.MTTAMinutes = roundMinutes(avg)
					r.MTTAHuman = humanDuration(avg)
				}
				rows = append(rows, r)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Incidents == rows[j].Incidents {
					return rows[i].Key < rows[j].Key
				}
				return rows[i].Incidents > rows[j].Incidents
			})

			out := struct {
				By     string `json:"by,omitempty"`
				Since  string `json:"since,omitempty"`
				Groups []row  `json:"groups"`
			}{By: by, Since: since, Groups: rows}
			if out.Groups == nil {
				out.Groups = []row{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if len(rows) == 0 {
					fmt.Fprintln(w, "No incidents with usable timestamps found (run 'rootly-cli sync').")
					return
				}
				label := by
				if label == "" {
					label = "ALL"
				}
				tw := newTabWriter(w)
				fmt.Fprintf(tw, "%s\tINCIDENTS\tRESOLVED\tMTTA\tMTTR\n", strings.ToUpper(label))
				for _, r := range rows {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n", r.Key, r.Incidents, r.Resolved, dash(r.MTTAHuman), dash(r.MTTRHuman))
				}
				flushHuman(cmd, tw)
			})
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Group by: service, team, or severity (default: overall)")
	cmd.Flags().StringVar(&since, "since", "", "Only incidents started within this window, e.g. 30d, 12h")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
