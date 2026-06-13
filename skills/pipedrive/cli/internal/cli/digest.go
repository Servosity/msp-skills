// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: daily standup digest. Hand-authored against the local store.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type digestItem struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Value float64 `json:"value"`
	When  string  `json:"when,omitempty"`
}

type digestSection struct {
	Count int          `json:"count"`
	Value float64      `json:"value"`
	Items []digestItem `json:"items,omitempty"`
}

type digestResult struct {
	GeneratedFor string        `json:"generated_for"`
	Since        string        `json:"since"`
	QuietDays    int           `json:"quiet_days"`
	NewDeals     digestSection `json:"new_deals"`
	GoneStale    digestSection `json:"gone_stale"`
	Overdue      digestSection `json:"overdue_activities"`
	DueToday     digestSection `json:"due_today"`
	Won          digestSection `json:"won"`
	Lost         digestSection `json:"lost"`
}

// pp:data-source local
func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var forMe bool
	var owner string
	var quietDays int
	var since string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "One-shot standup rollup: new deals, deals gone stale, overdue/due-today activities, and deals won or lost.",
		Long:  "Use this command for a human-readable morning standup rollup (new/stale/won/lost + today's activities), optionally narrowed to you with --for-me.\nDo NOT use this command for a machine-readable per-entity diff to feed a script; use 'changes' instead.",
		Example: strings.Trim(`
  pipedrive-cli digest
  pipedrive-cli digest --since 24h --quiet-days 14 --json
  pipedrive-cli digest --for-me --owner 12345 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			cutoff, err := pipeintel.ParseSince(since, time.Now().UTC())
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			cutoffStr := cutoff.Format("2006-01-02 15:04:05")

			ownerClause := ""
			var ownerArgs []any
			generatedFor := "team"
			if owner != "" {
				ownerClause = " AND user_id = ?"
				ownerArgs = []any{owner}
				generatedFor = "owner:" + owner
			} else if forMe {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: --for-me has no --owner to scope by; showing the team-wide digest. Pass --owner <your user id>.")
			}

			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			run := func(where, whenExpr string, extra ...any) (digestSection, error) {
				return digestQuery(cmd.Context(), db.DB(), where+ownerClause, whenExpr, append(append([]any{}, extra...), ownerArgs...), limit)
			}

			res := digestResult{GeneratedFor: generatedFor, Since: since, QuietDays: quietDays}
			if res.NewDeals, err = run("add_time >= ?", "add_time", cutoffStr); err != nil {
				return err
			}
			staleWhere := "status='open' AND (is_archived IS NULL OR is_archived=0) AND " + staleTouch +
				" IS NOT NULL AND julianday('now') - julianday(" + staleTouch + ") >= ?"
			if res.GoneStale, err = run(staleWhere, staleTouch, quietDays); err != nil {
				return err
			}
			if res.Overdue, err = run("status='open' AND next_activity_date IS NOT NULL AND next_activity_date < date('now')", "next_activity_date"); err != nil {
				return err
			}
			if res.DueToday, err = run("status='open' AND next_activity_date = date('now')", "next_activity_date"); err != nil {
				return err
			}
			if res.Won, err = run("status='won' AND won_time >= ?", "won_time", cutoffStr); err != nil {
				return err
			}
			if res.Lost, err = run("status='lost' AND lost_time >= ?", "lost_time", cutoffStr); err != nil {
				return err
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				fmt.Fprintf(w, "Pipedrive digest (%s, since %s):\n\n", res.GeneratedFor, since)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "SECTION\tCOUNT\tVALUE")
				fmt.Fprintf(tw, "New deals\t%d\t%.0f\n", res.NewDeals.Count, res.NewDeals.Value)
				fmt.Fprintf(tw, "Gone stale (>=%dd)\t%d\t%.0f\n", res.QuietDays, res.GoneStale.Count, res.GoneStale.Value)
				fmt.Fprintf(tw, "Overdue activities\t%d\t%.0f\n", res.Overdue.Count, res.Overdue.Value)
				fmt.Fprintf(tw, "Due today\t%d\t%.0f\n", res.DueToday.Count, res.DueToday.Value)
				fmt.Fprintf(tw, "Won\t%d\t%.0f\n", res.Won.Count, res.Won.Value)
				fmt.Fprintf(tw, "Lost\t%d\t%.0f\n", res.Lost.Count, res.Lost.Value)
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().BoolVar(&forMe, "for-me", false, "Scope to your own deals (uses --owner; team-wide if --owner is unset)")
	cmd.Flags().StringVar(&owner, "owner", "", "Scope the digest to a single owner user ID")
	cmd.Flags().IntVar(&quietDays, "quiet-days", 14, "Days with no activity before an open deal counts as gone-stale")
	cmd.Flags().StringVar(&since, "since", "24h", "Window for new/won/lost sections: 24h, 7d, 2w, or a date")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum items listed per section (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

// digestQuery runs one digest section: COUNT(*) + SUM(value) over deals matching
// where, plus a limited item listing using whenExpr as the relevant timestamp.
func digestQuery(ctx context.Context, db *sql.DB, where, whenExpr string, args []any, limit int) (digestSection, error) {
	var sec digestSection
	var count int
	var sum sql.NullFloat64
	if err := db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*), COALESCE(SUM(value),0) FROM deals WHERE %s", where), args...,
	).Scan(&count, &sum); err != nil {
		return sec, fmt.Errorf("digest count: %w", err)
	}
	sec.Count = count
	sec.Value = nullF64(sum)
	sec.Items = []digestItem{}
	if count == 0 {
		return sec, nil
	}
	// #nosec G201 -- `whenExpr` and `where` are package-internal string literals
	// built by newNovelDigestCmd (column names / constant clauses); every
	// user-supplied value (owner, since, quiet-days) is bound via `?` in args.
	q := fmt.Sprintf(`SELECT id, COALESCE(title,''), COALESCE(value,0), COALESCE(%s,'') AS whenv
		FROM deals WHERE %s ORDER BY value DESC`, whenExpr, where)
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return sec, fmt.Errorf("digest list: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it digestItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Value, &it.When); err != nil {
			return sec, fmt.Errorf("digest scan: %w", err)
		}
		sec.Items = append(sec.Items, it)
	}
	return sec, rows.Err()
}
