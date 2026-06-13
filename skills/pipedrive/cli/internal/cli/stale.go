// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: stale-deal detection. Hand-authored against the local store.

package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

type staleDeal struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Value      float64 `json:"value"`
	Currency   string  `json:"currency"`
	QuietDays  int     `json:"quiet_days"`
	LastTouch  string  `json:"last_touch"`
	StageID    int64   `json:"stage_id"`
	PipelineID int64   `json:"pipeline_id"`
	OwnerID    string  `json:"owner_id"`
	OwnerName  string  `json:"owner_name"`
}

type staleOrgGroup struct {
	OrgID       string  `json:"org_id"`
	OrgName     string  `json:"org_name"`
	DealCount   int     `json:"deal_count"`
	ValueAtRisk float64 `json:"value_at_risk"`
}

type staleResult struct {
	QuietDaysThreshold int             `json:"quiet_days_threshold"`
	Count              int             `json:"count"`
	TotalValueAtRisk   float64         `json:"total_value_at_risk"`
	GroupBy            string          `json:"group_by,omitempty"`
	Deals              []staleDeal     `json:"deals"`
	Organizations      []staleOrgGroup `json:"organizations"`
}

// staleTouch is the SQL expression for a deal's "last touched" timestamp.
const staleTouch = "COALESCE(last_activity_date, stage_change_time, add_time)"

// pp:data-source local
func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var quietDays int
	var groupBy string
	var pipeline int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List open deals nobody has touched in N days, ranked by the dollar value at risk.",
		Long: `Surfaces the open deals that are silently dying: those with no recorded
activity in the last N days, ranked by the value at risk. "Last touch" uses the
deal's last activity, falling back to its stage-change time, then its add time.

Reads the local store, so run 'pipedrive-cli sync' first. Use --group-by org
to roll the value at risk up per organization.`,
		Example: strings.Trim(`
  pipedrive-cli stale --quiet-days 14
  pipedrive-cli stale --quiet-days 30 --group-by org --json
  pipedrive-cli stale --quiet-days 7 --pipeline 1 --limit 20 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if quietDays < 0 {
				return fmt.Errorf("--quiet-days must be >= 0")
			}
			if groupBy != "" && groupBy != "org" {
				return fmt.Errorf("--group-by only supports 'org' (got %q)", groupBy)
			}
			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			where := "status='open' AND (is_archived IS NULL OR is_archived=0) AND " + staleTouch + " IS NOT NULL" +
				" AND julianday('now') - julianday(" + staleTouch + ") >= ?"
			qargs := []any{quietDays}
			if pipeline > 0 {
				where += " AND pipeline_id = ?"
				qargs = append(qargs, pipeline)
			}

			if groupBy == "org" {
				return runStaleByOrg(cmd, flags, db.DB(), where, qargs, quietDays, limit)
			}

			query := fmt.Sprintf(`
				SELECT id, COALESCE(title,''), COALESCE(value,0), COALESCE(currency,''),
				       CAST(julianday('now') - julianday(%s) AS INTEGER) AS quiet_days,
				       %s AS last_touch,
				       stage_id, pipeline_id, COALESCE(user_id,''), COALESCE(owner_name,'')
				FROM deals WHERE %s
				ORDER BY value DESC`, staleTouch, staleTouch, where)
			if limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", limit)
			}
			rows, err := db.DB().QueryContext(cmd.Context(), query, qargs...)
			if err != nil {
				return fmt.Errorf("querying stale deals: %w", err)
			}
			defer rows.Close()

			res := staleResult{QuietDaysThreshold: quietDays, Deals: []staleDeal{}}
			for rows.Next() {
				var d staleDeal
				var stage, pipe sql.NullInt64
				if err := rows.Scan(&d.ID, &d.Title, &d.Value, &d.Currency, &d.QuietDays,
					&d.LastTouch, &stage, &pipe, &d.OwnerID, &d.OwnerName); err != nil {
					return fmt.Errorf("scanning deal: %w", err)
				}
				d.StageID = nullI64(stage)
				d.PipelineID = nullI64(pipe)
				res.Deals = append(res.Deals, d)
				res.TotalValueAtRisk += d.Value
			}
			if err := rows.Err(); err != nil {
				return err
			}
			res.Count = len(res.Deals)

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if res.Count == 0 {
					fmt.Fprintf(w, "No open deals quiet for %d+ days. (Run 'sync' if the local store is empty.)\n", quietDays)
					return
				}
				fmt.Fprintf(w, "%d stale deal(s); %.2f total value at risk (quiet >= %d days):\n\n", res.Count, res.TotalValueAtRisk, quietDays)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "QUIET\tVALUE\tCUR\tTITLE\tOWNER\tLAST_TOUCH")
				for _, d := range res.Deals {
					fmt.Fprintf(tw, "%d\t%.0f\t%s\t%s\t%s\t%s\n", d.QuietDays, d.Value, d.Currency, truncateRunes(d.Title, 40), d.OwnerName, d.LastTouch)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().IntVar(&quietDays, "quiet-days", 14, "Days with no activity before a deal counts as stale")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Roll up results: 'org' groups value at risk per organization")
	cmd.Flags().IntVar(&pipeline, "pipeline", 0, "Restrict to a single pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

func runStaleByOrg(cmd *cobra.Command, flags *rootFlags, db *sql.DB, where string, qargs []any, quietDays, limit int) error {
	// #nosec G201 -- `where` is assembled only from package-internal string
	// literals (see newNovelStaleCmd); every user-supplied value (quiet-days,
	// pipeline) is bound through `?` placeholders in qargs, never interpolated.
	query := fmt.Sprintf(`
		SELECT COALESCE(org_id,''), COALESCE(org_name,'(no organization)'),
		       COUNT(*) AS deal_count, COALESCE(SUM(value),0) AS value_at_risk
		FROM deals WHERE %s
		GROUP BY org_id
		ORDER BY value_at_risk DESC`, where)
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.QueryContext(cmd.Context(), query, qargs...)
	if err != nil {
		return fmt.Errorf("querying stale deals by org: %w", err)
	}
	defer rows.Close()

	res := staleResult{QuietDaysThreshold: quietDays, GroupBy: "org", Organizations: []staleOrgGroup{}}
	for rows.Next() {
		var g staleOrgGroup
		if err := rows.Scan(&g.OrgID, &g.OrgName, &g.DealCount, &g.ValueAtRisk); err != nil {
			return fmt.Errorf("scanning org group: %w", err)
		}
		res.Organizations = append(res.Organizations, g)
		res.Count += g.DealCount
		res.TotalValueAtRisk += g.ValueAtRisk
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return emitNovel(cmd, flags, res, func(w io.Writer) {
		if len(res.Organizations) == 0 {
			fmt.Fprintf(w, "No organizations with deals quiet for %d+ days.\n", quietDays)
			return
		}
		fmt.Fprintf(w, "Stale value at risk by organization (quiet >= %d days):\n\n", quietDays)
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "VALUE_AT_RISK\tDEALS\tORGANIZATION")
		for _, g := range res.Organizations {
			fmt.Fprintf(tw, "%.0f\t%d\t%s\n", g.ValueAtRisk, g.DealCount, g.OrgName)
		}
		_ = tw.Flush()
	})
}

// truncateRunes shortens s to at most n runes, appending an ellipsis.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
