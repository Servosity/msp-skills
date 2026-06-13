// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/snapshot"
	"servosity-msp-pp-cli/internal/store"
)

// fleetHealthMetrics is the scalar scorecard, also persisted as the
// "fleet-health" snapshot metric so week-over-week deltas compound.
type fleetHealthMetrics struct {
	BackupsTotal   int     `json:"backups_total"`
	Success24hPct  float64 `json:"success_24h_pct"`
	StaleCompanies int     `json:"stale_companies"`
	OpenIssues     int     `json:"open_issues"`
}

type fleetHealthDeltas struct {
	Success24hPct  *float64 `json:"success_24h_pct,omitempty"`
	StaleCompanies *int     `json:"stale_companies,omitempty"`
	OpenIssues     *int     `json:"open_issues,omitempty"`
}

type fleetHealthView struct {
	TakenAt   string              `json:"taken_at"`
	Current   fleetHealthMetrics  `json:"current"`
	WeekAgo   *fleetHealthMetrics `json:"week_ago,omitempty"`
	WeekAgoAt string              `json:"week_ago_at,omitempty"`
	Deltas    *fleetHealthDeltas  `json:"deltas,omitempty"`
	Note      string              `json:"note,omitempty"`
}

// pp:data-source local
func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "One fleet-wide scorecard: 24h success rate, stale companies, open issues, with week-over-week deltas",
		Long: `Compute one fleet-wide scorecard from the local store: the share of backups
with a success in the last 24h, how many companies have at least one stale
backup, and the open-issue count — plus week-over-week deltas from the
snapshots this command persists on every run.

Use this command for one fleet-wide scorecard number set.
Do NOT use it for the per-company ranked list; use 'attention' instead.
Do NOT use it for what-changed detail; use 'drift' instead.

Reads from the local sync store. Run 'servosity-cli sync' and
'servosity-cli stale-backups --refresh' first.`,
		Example: `  # The owner glance
  servosity-cli fleet-health

  # Agent-shaped, success rate only
  servosity-cli fleet-health --json --select current.success_24h_pct`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compute the fleet health scorecard from the local store")
				return nil
			}
			ctx := cmd.Context()

			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "issues") {
				hintIfStale(cmd, db, "issues", flags.maxAge)
			}

			now := time.Now()

			// Freshness: total backups + 24h-success share + stale companies
			// from pp_last_success (hydrated by stale-backups --refresh).
			if err := ensureLastSuccessTable(ctx, db); err != nil {
				return err
			}
			entries, err := deriveStaleEntries(ctx, db, now)
			if err != nil {
				return err
			}
			var cur fleetHealthMetrics
			cur.BackupsTotal = len(entries)
			fresh24 := 0
			staleCos := map[int]bool{}
			for _, e := range entries {
				if e.DaysStale < 1 {
					fresh24++
				}
				if e.DaysStale >= staleDays {
					staleCos[e.CompanyID] = true
				}
			}
			if cur.BackupsTotal > 0 {
				cur.Success24hPct = float64(fresh24) / float64(cur.BackupsTotal) * 100
			}
			cur.StaleCompanies = len(staleCos)

			// Open issues from the local issues table (same predicate as
			// attention's store path).
			row := db.DB().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM issues
				 WHERE lower(COALESCE(state,'')) NOT IN ('closed','resolved','archived','ignored')`)
			if err := row.Scan(&cur.OpenIssues); err != nil {
				cur.OpenIssues = 0
			}

			view := fleetHealthView{TakenAt: now.UTC().Format(time.RFC3339), Current: cur}
			if cur.BackupsTotal == 0 {
				view.Note = "no freshness data cached; run 'servosity-cli stale-backups --refresh' to hydrate per-backup last-success"
			}

			// Week-over-week from the snapshot store this command maintains.
			if err := snapshot.EnsureTable(ctx, db.DB()); err == nil {
				if prior, perr := snapshot.At(ctx, db.DB(), "fleet-health", now.AddDate(0, 0, -7)); perr == nil && prior != nil {
					var old fleetHealthView
					if json.Unmarshal(prior.Data, &old) == nil {
						view.WeekAgo = &old.Current
						view.WeekAgoAt = prior.TakenAt.UTC().Format(time.RFC3339)
						dPct := cur.Success24hPct - old.Current.Success24hPct
						dStale := cur.StaleCompanies - old.Current.StaleCompanies
						dIssues := cur.OpenIssues - old.Current.OpenIssues
						view.Deltas = &fleetHealthDeltas{Success24hPct: &dPct, StaleCompanies: &dStale, OpenIssues: &dIssues}
					}
				}
				if payload, merr := json.Marshal(view); merr == nil {
					_ = snapshot.Save(ctx, db.DB(), "fleet-health", now, payload)
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Fleet health as of %s\n\n", view.TakenAt)
			fmt.Fprintf(out, "  Backups tracked:        %d\n", cur.BackupsTotal)
			fmt.Fprintf(out, "  Success in last 24h:    %.1f%%\n", cur.Success24hPct)
			fmt.Fprintf(out, "  Companies w/ stale:     %d (>= %dd stale)\n", cur.StaleCompanies, staleDays)
			fmt.Fprintf(out, "  Open issues:            %d\n", cur.OpenIssues)
			if view.Deltas != nil {
				fmt.Fprintf(out, "\nWeek-over-week (vs %s):\n", view.WeekAgoAt)
				fmt.Fprintf(out, "  Success 24h:  %+.1f pts\n", *view.Deltas.Success24hPct)
				fmt.Fprintf(out, "  Stale cos:    %+d\n", *view.Deltas.StaleCompanies)
				fmt.Fprintf(out, "  Open issues:  %+d\n", *view.Deltas.OpenIssues)
			} else {
				fmt.Fprintln(out, "\n(no week-old snapshot yet — deltas appear after a week of runs)")
			}
			if view.Note != "" {
				fmt.Fprintln(out, "\nnote: "+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Days without success before a backup counts as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/servosity-cli/data.db)")
	return cmd
}
