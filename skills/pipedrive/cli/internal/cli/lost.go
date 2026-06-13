// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: lost-deal re-engagement list. Hand-authored against the local store.

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

type lostDeal struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Value      float64 `json:"value"`
	Currency   string  `json:"currency"`
	LostTime   string  `json:"lost_time"`
	LostReason string  `json:"lost_reason,omitempty"`
	PersonID   string  `json:"person_id,omitempty"`
	PersonName string  `json:"person_name,omitempty"`
	OrgID      string  `json:"org_id,omitempty"`
	OrgName    string  `json:"org_name,omitempty"`
	OwnerID    string  `json:"owner_id,omitempty"`
	OwnerName  string  `json:"owner_name,omitempty"`
}

type lostResult struct {
	Since          string     `json:"since"`
	Cutoff         string     `json:"cutoff"`
	Count          int        `json:"count"`
	TotalLostValue float64    `json:"total_lost_value"`
	Deals          []lostDeal `json:"deals"`
}

// pp:data-source local
func newNovelLostCmd(flags *rootFlags) *cobra.Command {
	var since string
	var reason string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "lost",
		Short: "Deals marked Lost in a recent window, with person, org, owner, and lost reason.",
		Long: `Lists deals marked Lost within a recent window, joined to their person,
organization, owner, and lost reason — ready to triage for re-engagement.

Reads the local store, so run 'pipedrive-cli sync' first. Narrow the window
with --since and filter the lost reason with --reason.`,
		Example: strings.Trim(`
  pipedrive-cli lost --since 90d
  pipedrive-cli lost --since 1w --reason budget --json
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
			cutoffT, err := pipeintel.ParseSince(since, time.Now().UTC())
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be >= 0"))
			}
			cutoff := cutoffT.Format("2006-01-02 15:04:05")

			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			res, err := queryLostDeals(cmd.Context(), db.DB(), cutoff, reason, limit)
			if err != nil {
				return err
			}
			res.Since = since
			res.Cutoff = cutoff

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if res.Count == 0 {
					fmt.Fprintf(w, "No deals marked Lost since %s. (Run 'sync' if the local store is empty.)\n", since)
					return
				}
				fmt.Fprintf(w, "%d deal(s) lost since %s; %.2f total lost value:\n\n", res.Count, since, res.TotalLostValue)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "LOST_TIME\tVALUE\tCUR\tTITLE\tPERSON\tORG\tREASON")
				for _, d := range res.Deals {
					fmt.Fprintf(tw, "%s\t%.0f\t%s\t%s\t%s\t%s\t%s\n",
						d.LostTime, d.Value, d.Currency, truncateRunes(d.Title, 30),
						truncateRunes(d.PersonName, 20), truncateRunes(d.OrgName, 20), truncateRunes(d.LostReason, 30))
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&since, "since", "180d", "Window for the lost time, e.g. 180d, 90d, 4w, 24h, or an absolute date like 2026-05-01")
	cmd.Flags().StringVar(&reason, "reason", "", "Case-insensitive substring filter on the lost reason")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

// queryLostDeals returns deals with status='lost' whose lost_time falls on or
// after cutoff's calendar day (date() comparison, so date-only lost_time values
// changes.go does for update_time), joined to person/org/owner names carried on
// the deal row. A non-empty reason applies a case-insensitive substring filter.
func queryLostDeals(ctx context.Context, db *sql.DB, cutoff, reason string, limit int) (lostResult, error) {
	where := "status='lost' AND lost_time IS NOT NULL AND date(lost_time) >= date(?)"
	qargs := []any{cutoff}
	if reason != "" {
		where += " AND lower(COALESCE(lost_reason,'')) LIKE '%'||lower(?)||'%'"
		qargs = append(qargs, reason)
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(title,''), COALESCE(value,0), COALESCE(currency,''),
		       COALESCE(lost_time,''), COALESCE(lost_reason,''),
		       COALESCE(person_id,''), COALESCE(person_name,''),
		       COALESCE(org_id,''), COALESCE(org_name,''),
		       COALESCE(user_id,''), COALESCE(owner_name,'')
		FROM deals WHERE %s
		ORDER BY lost_time DESC`, where)
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return lostResult{}, fmt.Errorf("querying lost deals: %w", err)
	}
	defer rows.Close()

	res := lostResult{Deals: make([]lostDeal, 0)}
	for rows.Next() {
		var d lostDeal
		if err := rows.Scan(&d.ID, &d.Title, &d.Value, &d.Currency,
			&d.LostTime, &d.LostReason, &d.PersonID, &d.PersonName,
			&d.OrgID, &d.OrgName, &d.OwnerID, &d.OwnerName); err != nil {
			return lostResult{}, fmt.Errorf("scanning lost deal: %w", err)
		}
		res.Deals = append(res.Deals, d)
		res.TotalLostValue += d.Value
	}
	if err := rows.Err(); err != nil {
		return lostResult{}, err
	}
	res.Count = len(res.Deals)
	return res, nil
}
