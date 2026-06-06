// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: contacts on Closed Won deals gone cold — the post-win
// re-engagement list.

package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelContactsWinBackCmd(flags *rootFlags) *cobra.Command {
	var owner string
	var coldDays int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "win-back",
		Short:       "Contacts on Closed Won deals gone cold — the post-win re-engagement list",
		Long:        "Join Closed Won deals to their associated contacts and surface those with no engagement in N days — the customer-expansion / re-engage motion.\n\nUse this command for post-win customer re-engagement/expansion.\nDo NOT use this command for open-deal cold contacts; use 'nurture-mine' instead.\nDo NOT use this command for generic no-engagement detection; use 'stale' instead.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli contacts win-back --cold-days 90
  hubspot-cli contacts win-back --owner me --cold-days 120 --limit 25`,
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
			if !hintIfUnsynced(cmd, db, "hubspot-contacts-crm") {
				hintIfStale(cmd, db, "hubspot-contacts-crm", flags.maxAge)
			}
			ownerID, err := resolveOwnerArg(db, owner)
			if err != nil {
				return err
			}

			items, err := queryContactsWinBack(cmd, db, ownerID, coldDays, limit)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return flags.printJSON(cmd, items)
			}
			headers := []string{"id", "name", "email", "owner_id", "won_deal_id", "won_deal_name", "won_amount", "idle_days"}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				rows = append(rows, []string{
					it.ID, it.Name, it.Email, it.OwnerID,
					it.WonDealID, it.WonDealName, formatAmount(it.WonAmount),
					fmt.Sprintf("%d", it.IdleDays),
				})
			}
			return flags.printTabular(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Filter by owner id, email, or 'me'")
	cmd.Flags().IntVar(&coldDays, "cold-days", 90, "Minimum idle days since last contact")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type winBackRow struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Email       string  `json:"email"`
	OwnerID     string  `json:"owner_id"`
	WonDealID   string  `json:"won_deal_id"`
	WonDealName string  `json:"won_deal_name"`
	WonAmount   float64 `json:"won_amount"`
	IdleDays    int64   `json:"idle_days"`
}

func queryContactsWinBack(cmd *cobra.Command, db *store.Store, ownerID string, coldDays, limit int) ([]winBackRow, error) {
	items := []winBackRow{}

	// Requires the association table to know which contacts are attached to a
	// Closed Won deal. Absence is tolerated (error-ignored COUNT, mirroring
	// nurture_mine.go); with no associations there is nothing to surface.
	var assocCount int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM hubspot_associations`).Scan(&assocCount)
	if assocCount == 0 {
		return items, nil
	}

	// One row per (contact, closedwon deal). idle_days computed via stale.go's
	// notes_last_contacted COALESCE chain. When a contact is associated with
	// several won deals, each association emits its own row.
	q := `
SELECT * FROM (
  SELECT c.id AS contact_id,
    TRIM(COALESCE(json_extract(c.data, '$.properties.firstname'), '') || ' ' ||
         COALESCE(json_extract(c.data, '$.properties.lastname'), '')) AS name,
    COALESCE(json_extract(c.data, '$.properties.email'), '') AS email,
    COALESCE(json_extract(c.data, '$.properties.hubspot_owner_id'), '') AS owner_id,
    d.id AS won_deal_id,
    COALESCE(json_extract(d.data, '$.properties.dealname'), '') AS won_deal_name,
    CAST(json_extract(d.data, '$.properties.amount') AS REAL) AS won_amount,
    CAST((julianday('now') - julianday(COALESCE(
      json_extract(c.data, '$.properties.notes_last_contacted'),
      json_extract(c.data, '$.properties.notes_last_updated'),
      json_extract(c.data, '$.properties.hs_lastmodifieddate'),
      c.created_at
    ))) AS INTEGER) AS idle_days
  FROM hubspot_contacts_crm c
  JOIN hubspot_associations a
    ON a.from_type = 'contacts' AND a.from_id = c.id
   AND a.to_type = 'deals'
  JOIN hubspot_deals_crm d
    ON d.id = a.to_id
   AND COALESCE(d.archived, 0) = 0
   AND json_extract(d.data, '$.properties.dealstage') = 'closedwon'
  WHERE COALESCE(c.archived, 0) = 0
    AND (? = '' OR json_extract(c.data, '$.properties.hubspot_owner_id') = ?)
)
WHERE idle_days >= ?
ORDER BY idle_days DESC
LIMIT ?`
	sqlLimit := limit
	if sqlLimit <= 0 {
		sqlLimit = -1
	}
	rows, err := db.DB().QueryContext(cmd.Context(), q, ownerID, ownerID, coldDays, sqlLimit)
	if err != nil {
		return items, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r winBackRow
		var amt sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Name, &r.Email, &r.OwnerID, &r.WonDealID, &r.WonDealName, &amt, &r.IdleDays); err != nil {
			return items, err
		}
		r.WonAmount = nullF(amt)
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return items, fmt.Errorf("iterating win-back rows: %w", err)
	}
	return items, nil
}
