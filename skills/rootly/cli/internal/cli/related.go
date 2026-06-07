// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): offline TF-IDF similarity over
// the synced incident corpus. Matches Rootly's remote find_related_incidents
// with zero per-call API cost.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelRelatedCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "related <incident-id>",
		Short: "Find the past incidents most similar to a given one, ranked.",
		Long: `Rank previously synced incidents by similarity to a target incident using
local TF-IDF over each incident's title and summary. Runs entirely offline
against the local mirror — no remote ML service, no per-call API cost.

The argument may be an incident id, slug, or sequential id.`,
		Example: `  rootly-cli related INC-1234 --limit 5
  rootly-cli related 01890-abcd --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			target := args[0]

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			rt := novelResolveType(db, "incidents")
			incidents, err := novelLoad(db, rt)
			if err != nil {
				return err
			}

			var targetRec *record
			for i := range incidents {
				r := incidents[i]
				if r.ID == target ||
					recStr(r.Attrs, "slug") == target ||
					recStr(r.Attrs, "sequential_id") == target {
					targetRec = &incidents[i]
					break
				}
			}
			if targetRec == nil {
				return notFoundErr(fmt.Errorf("incident %q not found in the local mirror (run 'rootly-cli sync' or check the id)", target))
			}

			textByID := make(map[string]string, len(incidents))
			byID := make(map[string]record, len(incidents))
			for _, r := range incidents {
				textByID[r.ID] = incidentTitle(r) + " " + recStr(r.Attrs, "summary")
				byID[r.ID] = r
			}
			corpus := newTFIDF(textByID)
			tv := corpus.vec(targetRec.ID)

			var ranked []scored
			for id := range textByID {
				if id == targetRec.ID {
					continue
				}
				if s := cosine(tv, corpus.vec(id)); s > 0 {
					ranked = append(ranked, scored{id: id, score: s})
				}
			}
			sortScoredDesc(ranked)
			if limit > 0 && len(ranked) > limit {
				ranked = ranked[:limit]
			}

			type relatedItem struct {
				ID         string  `json:"id"`
				Title      string  `json:"title"`
				Severity   string  `json:"severity,omitempty"`
				Status     string  `json:"status,omitempty"`
				Score      float64 `json:"similarity"`
				Resolution string  `json:"resolution,omitempty"`
			}
			out := struct {
				Target  map[string]string `json:"target"`
				Related []relatedItem     `json:"related"`
			}{
				Target:  map[string]string{"id": targetRec.ID, "title": incidentTitle(*targetRec)},
				Related: []relatedItem{},
			}
			for _, s := range ranked {
				r := byID[s.id]
				out.Related = append(out.Related, relatedItem{
					ID:         r.ID,
					Title:      incidentTitle(r),
					Severity:   incidentSeverity(r),
					Status:     recStr(r.Attrs, "status"),
					Score:      float64(int(s.score*1000)) / 1000,
					Resolution: strings.TrimSpace(recStr(r.Attrs, "resolution_message", "mitigation_message")),
				})
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Incidents similar to %s — %s\n\n", targetRec.ID, incidentTitle(*targetRec))
				if len(out.Related) == 0 {
					fmt.Fprintln(w, "No similar incidents found (need at least two incidents with overlapping terms; run 'rootly-cli sync').")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "SIM\tID\tSEV\tSTATUS\tTITLE")
				for _, it := range out.Related {
					fmt.Fprintf(tw, "%.3f\t%s\t%s\t%s\t%s\n", it.Score, it.ID, it.Severity, it.Status, truncate(it.Title, 60))
				}
				flushHuman(cmd, tw)
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum similar incidents to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
