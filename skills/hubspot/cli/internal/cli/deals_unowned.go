// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: open deals with no owner or a deactivated owner, with $ exposure.

package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelDealsUnownedCmd(flags *rootFlags) *cobra.Command {
	var pipeline string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "unowned",
		Short:       "Open deals with no owner or a deactivated owner, with dollar exposure",
		Long:        "Anti-join open deals against the synced owners table to find deals owned by nobody or by a deactivated/archived owner, with per-stage dollar exposure.\n\nUse this command to find orphaned or dead-owner deals.\nDo NOT use this command for per-rep load on owned deals; use 'owner-load' instead.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli deals unowned --pipeline default
  hubspot-cli deals unowned --limit 25`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("hubspot-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "hubspot-deals-crm") {
				hintIfStale(cmd, db, "hubspot-deals-crm", flags.maxAge)
			}

			view, err := queryDealsUnowned(cmd, db, pipeline, limit)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"id", "name", "amount", "stage", "owner_id", "reason"}
			rows := make([][]string, 0, len(view.Deals))
			for _, d := range view.Deals {
				rows = append(rows, []string{
					d.ID, d.Name, formatAmount(d.Amount), d.Stage, d.OwnerID, d.Reason,
				})
			}
			if err := flags.printTabular(cmd, headers, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "total_exposure: %s across %d deals\n",
				formatAmount(view.TotalExposure), view.Count)
			return nil
		},
	}
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "Restrict to a single pipeline id")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type unownedDeal struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Amount  float64 `json:"amount"`
	Stage   string  `json:"stage"`
	OwnerID string  `json:"owner_id"`
	Reason  string  `json:"reason"`
}

type unownedView struct {
	Deals         []unownedDeal `json:"deals"`
	TotalExposure float64       `json:"total_exposure"`
	Count         int64         `json:"count"`
}

func queryDealsUnowned(cmd *cobra.Command, db *store.Store, pipeline string, limit int) (unownedView, error) {
	view := unownedView{Deals: []unownedDeal{}}

	// Open deals whose owner is unassigned ('' or NULL) OR whose owner_id is not
	// found among the ACTIVE owners (mirror owner_load.go's hubspot_owners_crm
	// table; active = COALESCE(archived,0)=0). A non-empty owner that isn't an
	// active owner is "owner-not-found" (deactivated/archived/missing).
	q := `
SELECT id, name, amount, stage, owner_id,
  CASE WHEN owner_id = '' THEN 'unassigned' ELSE 'owner-not-found' END AS reason
FROM (
  SELECT d.id AS id,
    COALESCE(json_extract(d.data, '$.properties.dealname'), '') AS name,
    CAST(json_extract(d.data, '$.properties.amount') AS REAL) AS amount,
    COALESCE(json_extract(d.data, '$.properties.dealstage'), '') AS stage,
    COALESCE(json_extract(d.data, '$.properties.hubspot_owner_id'), '') AS owner_id
  FROM hubspot_deals_crm d
  WHERE COALESCE(d.archived, 0) = 0
    AND COALESCE(json_extract(d.data, '$.properties.dealstage'), '') NOT IN ('closedwon', 'closedlost')
    AND (? = '' OR json_extract(d.data, '$.properties.pipeline') = ?)
)
WHERE owner_id = ''
   OR owner_id NOT IN (
     SELECT id FROM hubspot_owners_crm WHERE COALESCE(archived, 0) = 0
   )
ORDER BY amount DESC`
	rows, err := db.DB().QueryContext(cmd.Context(), q, pipeline, pipeline)
	if err != nil {
		return view, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	all := []unownedDeal{}
	var total float64
	for rows.Next() {
		var d unownedDeal
		var amt sql.NullFloat64
		if err := rows.Scan(&d.ID, &d.Name, &amt, &d.Stage, &d.OwnerID, &d.Reason); err != nil {
			return view, err
		}
		d.Amount = nullF(amt)
		all = append(all, d)
		total += d.Amount
	}
	if err := rows.Err(); err != nil {
		return view, fmt.Errorf("iterating unowned deal rows: %w", err)
	}

	// Summary covers the full population; the limit caps only the rendered rows.
	view.TotalExposure = total
	view.Count = int64(len(all))
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if len(all) > 0 {
		view.Deals = all
	}
	return view, nil
}
