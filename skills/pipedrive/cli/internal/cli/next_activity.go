// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: no-next-activity (cold) deals. Hand-authored against the local store.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type coldDeal struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Value            float64 `json:"value"`
	Currency         string  `json:"currency"`
	OwnerID          string  `json:"owner_id,omitempty"`
	OwnerName        string  `json:"owner_name,omitempty"`
	OrgName          string  `json:"org_name,omitempty"`
	StageID          int64   `json:"stage_id"`
	PipelineID       int64   `json:"pipeline_id"`
	AddTime          string  `json:"add_time,omitempty"`
	LastActivityDate string  `json:"last_activity_date,omitempty"`
}

type coldResult struct {
	Count            int        `json:"count"`
	TotalValueAtRisk float64    `json:"total_value_at_risk"`
	Deals            []coldDeal `json:"deals"`
}

// pp:data-source local
func newNovelNextActivityCmd(flags *rootFlags) *cobra.Command {
	var missing bool
	var pipeline int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "next-activity",
		Short: "List open deals with no future activity scheduled, ranked by the value at risk.",
		Long: `Use this command for open deals with NO future activity scheduled (a next-activity-discipline gap — you forgot to plan the next step).
Do NOT use this command for deals that simply haven't been touched recently; use 'stale' instead.`,
		Example: strings.Trim(`
  pipedrive-cli next-activity --missing
  pipedrive-cli next-activity --missing --pipeline 1 --limit 20 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if !missing {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--missing is required (this command currently has one mode)"))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be >= 0"))
			}

			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			res, err := queryColdDeals(cmd.Context(), db.DB(), pipeline, limit)
			if err != nil {
				return err
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if res.Count == 0 {
					fmt.Fprintln(w, "No open deals are missing a future activity. (Run 'sync' if the local store is empty.)")
					return
				}
				fmt.Fprintf(w, "%d open deal(s) with no future activity; %.2f total value at risk:\n\n", res.Count, res.TotalValueAtRisk)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "VALUE\tCUR\tTITLE\tOWNER\tORG\tLAST_ACTIVITY")
				for _, d := range res.Deals {
					fmt.Fprintf(tw, "%.0f\t%s\t%s\t%s\t%s\t%s\n",
						d.Value, d.Currency, truncateRunes(d.Title, 36), d.OwnerName, truncateRunes(d.OrgName, 24), d.LastActivityDate)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().BoolVar(&missing, "missing", false, "Headline mode: open, non-archived deals with no future activity scheduled")
	cmd.Flags().IntVar(&pipeline, "pipeline", 0, "Restrict to a single pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum rows to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

// queryColdDeals returns open, non-archived deals with no future activity
// scheduled (next_activity_date is NULL or empty), ordered by value at risk.
// A pipeline > 0 restricts to that pipeline; a limit > 0 caps the row count.
func queryColdDeals(ctx context.Context, db *sql.DB, pipeline, limit int) (coldResult, error) {
	where := "status='open' AND (is_archived IS NULL OR is_archived=0)" +
		" AND (next_activity_date IS NULL OR next_activity_date='')"
	qargs := []any{}
	if pipeline > 0 {
		where += " AND pipeline_id = ?"
		qargs = append(qargs, pipeline)
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(title,''), COALESCE(value,0), COALESCE(currency,''),
		       COALESCE(user_id,''), COALESCE(owner_name,''), COALESCE(org_name,''),
		       stage_id, pipeline_id,
		       COALESCE(add_time,''), COALESCE(last_activity_date,'')
		FROM deals WHERE %s
		ORDER BY value DESC`, where)
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return coldResult{}, fmt.Errorf("querying cold deals: %w", err)
	}
	defer rows.Close()

	res := coldResult{Deals: make([]coldDeal, 0)}
	for rows.Next() {
		var d coldDeal
		var stage, pipe sql.NullInt64
		if err := rows.Scan(&d.ID, &d.Title, &d.Value, &d.Currency,
			&d.OwnerID, &d.OwnerName, &d.OrgName, &stage, &pipe,
			&d.AddTime, &d.LastActivityDate); err != nil {
			return coldResult{}, fmt.Errorf("scanning deal: %w", err)
		}
		d.StageID = nullI64(stage)
		d.PipelineID = nullI64(pipe)
		res.Deals = append(res.Deals, d)
		res.TotalValueAtRisk += d.Value
	}
	if err := rows.Err(); err != nil {
		return coldResult{}, err
	}
	res.Count = len(res.Deals)
	return res, nil
}
