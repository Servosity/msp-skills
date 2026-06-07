// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): mine past resolutions for a
// service or query. Matches Rootly's remote suggest_solutions, offline, emitting
// raw pipe-friendly rows.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFixedLastTimeCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "fixed-last-time <service-or-query>",
		Short: "Mine resolutions and action items from a service's past incidents.",
		Long: `Surface what actually resolved this class of problem before. Matches past
resolved incidents by service name or free-text query, then emits each one's
resolution/mitigation message and its action items from the local mirror —
offline, no summarization, raw rows you can pipe.`,
		Example: `  rootly-cli fixed-last-time checkout-api --limit 10
  rootly-cli fixed-last-time "database timeout" --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			query := strings.ToLower(strings.TrimSpace(args[0]))

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}

			type match struct {
				ID          string   `json:"id"`
				Title       string   `json:"title"`
				ResolvedAt  string   `json:"resolved_at,omitempty"`
				Severity    string   `json:"severity,omitempty"`
				Resolution  string   `json:"resolution,omitempty"`
				ActionItems []string `json:"action_items"`
			}
			var matches []match
			for _, r := range incidents {
				// Only learn from incidents that actually concluded.
				resolvedAt, ok := incidentResolved(r)
				if !ok {
					continue
				}
				hay := strings.ToLower(strings.Join(append(incidentServiceNames(r),
					incidentTitle(r), recStr(r.Attrs, "summary")), " "))
				if !strings.Contains(hay, query) {
					continue
				}
				resolution := strings.TrimSpace(recStr(r.Attrs, "resolution_message", "mitigation_message"))
				ais := collectAllActionItems(db, r.ID)
				if resolution == "" && len(ais) == 0 {
					continue // nothing to teach
				}
				matches = append(matches, match{
					ID:          r.ID,
					Title:       incidentTitle(r),
					ResolvedAt:  resolvedAt.Format("2006-01-02"),
					Severity:    incidentSeverity(r),
					Resolution:  resolution,
					ActionItems: ais,
				})
			}
			// Most recent first.
			sort.Slice(matches, func(i, j int) bool { return matches[i].ResolvedAt > matches[j].ResolvedAt })
			if limit > 0 && len(matches) > limit {
				matches = matches[:limit]
			}

			out := struct {
				Query   string  `json:"query"`
				Matches []match `json:"matches"`
			}{Query: args[0], Matches: matches}
			if out.Matches == nil {
				out.Matches = []match{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if len(out.Matches) == 0 {
					fmt.Fprintf(w, "No resolved incidents matched %q (run 'rootly-cli sync' to populate history).\n", args[0])
					return
				}
				fmt.Fprintf(w, "What fixed %q last time (%d resolved incidents):\n\n", args[0], len(out.Matches))
				for _, m := range out.Matches {
					fmt.Fprintf(w, "• %s  [%s]  %s\n", m.ResolvedAt, dash(m.Severity), m.Title)
					if m.Resolution != "" {
						fmt.Fprintf(w, "    resolution: %s\n", truncate(m.Resolution, 200))
					}
					for _, a := range m.ActionItems {
						fmt.Fprintf(w, "    action: %s\n", truncate(a, 120))
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum resolved incidents to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
