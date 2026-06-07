// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: assisted-sales pipeline rollup over the
// local opportunities table. Originally scaffolded by the CLI Printing Press;
// body is hand-authored.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type pipelineGroup struct {
	Key       string `json:"key"`
	Count     int    `json:"count"`
	OpenCount int    `json:"openCount"`
	Oldest    string `json:"oldest,omitempty"`
	Newest    string `json:"newest,omitempty"`
}

type pipelineView struct {
	GroupBy              string          `json:"group_by"`
	Groups               []pipelineGroup `json:"groups"`
	Total                int             `json:"total"`
	ScannedOpportunities int             `json:"scanned_opportunities"`
	Note                 string          `json:"note,omitempty"`
}

func newNovelPipelineCmd(flags *rootFlags) *cobra.Command {
	var flagGroupBy string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Roll up the assisted-sales pipeline by status or owner with opportunity counts and ages",
		Long: strings.TrimSpace(`
Use this command for a status-and-owner rollup of the assisted-sales pipeline.
It groups the locally synced assisted-sales-v1-opportunities table by
opportunity status (OPEN / CLOSED / PENDING_REVIEW) or by owner, with counts
and oldest/newest creation dates per group.

Do NOT use this command to find inactive opportunities; use 'pipeline stale'
instead. Run 'sync --resources assisted-sales-v1-opportunities' first to
populate the local store.`),
		Example: strings.Trim(`
  # Pipeline counts by status
  appdirect-cli pipeline --group-by status --agent

  # Who owns the open pipeline
  appdirect-cli pipeline --group-by owner --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would roll up local opportunities by status or owner")
				return nil
			}
			if flagGroupBy == "" {
				flagGroupBy = "status"
			}
			if flagGroupBy != "status" && flagGroupBy != "owner" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--group-by must be 'status' or 'owner', got %q", flagGroupBy))
			}

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtOpportunities) {
				hintIfStale(cmd, db, rtOpportunities, flags.maxAge)
			}

			opps, err := loadResourceObjects(cmd.Context(), db, rtOpportunities)
			if err != nil {
				return err
			}

			type agg struct {
				count, open    int
				oldest, newest time.Time
			}
			groups := map[string]*agg{}
			for _, opp := range opps {
				status := strings.ToUpper(novelStr(opp, "status"))
				key := status
				if flagGroupBy == "owner" {
					owner := novelNested(opp, "ownerUser")
					key = novelStr(owner, "email")
					if key == "" {
						key = novelStr(owner, "id")
					}
					if key == "" {
						key = "(unassigned)"
					}
				}
				if key == "" {
					key = "(unknown)"
				}
				g, ok := groups[key]
				if !ok {
					g = &agg{}
					groups[key] = g
				}
				g.count++
				if status == "OPEN" {
					g.open++
				}
				if created, ok := novelEpochMS(opp, "createdOn"); ok {
					if g.oldest.IsZero() || created.Before(g.oldest) {
						g.oldest = created
					}
					if created.After(g.newest) {
						g.newest = created
					}
				}
			}

			out := make([]pipelineGroup, 0, len(groups))
			for k, g := range groups {
				out = append(out, pipelineGroup{
					Key:       k,
					Count:     g.count,
					OpenCount: g.open,
					Oldest:    isoOrEmpty(g.oldest),
					Newest:    isoOrEmpty(g.newest),
				})
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].Count != out[j].Count {
					return out[i].Count > out[j].Count
				}
				return out[i].Key < out[j].Key
			})

			view := pipelineView{
				GroupBy:              flagGroupBy,
				Groups:               out,
				Total:                len(opps),
				ScannedOpportunities: len(opps),
			}
			if len(opps) == 0 {
				view.Note = "no opportunities in the local store; run 'sync --resources assisted-sales-v1-opportunities' first"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagGroupBy, "group-by", "status", "Group rollup by 'status' or 'owner'")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.AddCommand(newNovelPipelineStaleCmd(flags))
	return cmd
}
