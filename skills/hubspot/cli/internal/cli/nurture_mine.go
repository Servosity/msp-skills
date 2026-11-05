// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: cold-contact-with-open-deal query.

package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelNurtureMineCmd(flags *rootFlags) *cobra.Command {
	var owner string
	var staleDays int
	var stageUnder string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "nurture-mine",
		Short:       "Cold contacts owned by you that still have open deals",
		Long:        `Surface contacts who haven't been contacted in N days but still have an open deal. Reads from the local SQLite mirror. The open-deal part needs contact-to-deal associations, which this build's sync does not collect: without them every cold contact is listed, not_computed names open_deals, and deal_count, latest_deal_stage and top_deal_amount are null (unknown, not zero).`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     `  hubspot-cli nurture-mine --owner me --stale-days 14`,
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

			rows, source, err := queryNurtureMine(cmd, db, ownerID, staleDays, stageUnder, limit)
			if err != nil {
				return err
			}
			gaps := newHSReportGaps()
			if source == nurtureNoAssociations {
				gaps.associationGap(cmd, "open_deals", "every cold contact is listed whether or not it has an open deal, and deal_count, latest_deal_stage and top_deal_amount are null")
			}

			if flags.asJSON {
				return flags.printJSON(cmd, gaps.envelope(map[string]any{
					"data_source": source,
					"results":     rows,
				}))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "data_source: %s\n", source)
			headers := []string{"contact_id", "name", "email", "last_contacted", "days_stale", "deal_count", "latest_deal_stage"}
			out := make([][]string, 0, len(rows))
			for _, r := range rows {
				dealCount, stage := "unknown", "unknown"
				if r.DealCount != nil {
					dealCount = fmt.Sprintf("%d", *r.DealCount)
				}
				if r.LatestDealStage != nil {
					stage = *r.LatestDealStage
				}
				out = append(out, []string{
					r.ContactID, r.Name, r.Email, r.LastContacted,
					fmt.Sprintf("%d", r.DaysStale),
					dealCount, stage,
				})
			}
			return flags.printTabular(cmd, headers, out)
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "me", "Filter by owner id, email, or 'me'")
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Minimum days since last contact")
	cmd.Flags().StringVar(&stageUnder, "stage-under", "closedwon", "Also exclude deals in exactly this stage id (closed deals are always excluded)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// nurtureNoAssociations is the data_source of the degraded path: no
// association rows, so the deal fields are unknown (nil), never zero.
const nurtureNoAssociations = "local-no-associations"

type nurtureMineRow struct {
	ContactID       string   `json:"contact_id"`
	Name            string   `json:"name"`
	Email           string   `json:"email"`
	LastContacted   string   `json:"last_contacted"`
	DaysStale       int64    `json:"days_stale"`
	DealCount       *int64   `json:"deal_count"`
	LatestDealStage *string  `json:"latest_deal_stage"`
	TopDealAmount   *float64 `json:"top_deal_amount"`
}

func queryNurtureMine(cmd *cobra.Command, db *store.Store, ownerID string, staleDays int, stageUnder string, limit int) ([]nurtureMineRow, string, error) {
	if err := requireKnownDealStates(cmd.Context(), db, "nurture-mine", ""); err != nil {
		return nil, "", err
	}
	if err := hsRequireCollected(cmd.Context(), db, "nurture-mine", "hubspot-contacts-crm"); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
		return nil, "", err
	}
	if err := hsRequireCollected(cmd.Context(), db, "nurture-mine", "hubspot-deals-crm"); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
		return nil, "", err
	}
	assocCount, err := hsAssociationCount(db)
	if err != nil {
		return nil, "", err
	}
	source := "local"

	if assocCount > 0 {
		q := `
SELECT * FROM (
  SELECT c.id,
    TRIM(COALESCE(json_extract(c.data, '$.properties.firstname'), '') || ' ' ||
         COALESCE(json_extract(c.data, '$.properties.lastname'), '')) AS name,
    COALESCE(json_extract(c.data, '$.properties.email'), '') AS email,
    json_extract(c.data, '$.properties.notes_last_contacted') AS last_contacted,
    CAST((julianday('now') - julianday(COALESCE(
      json_extract(c.data, '$.properties.notes_last_contacted'),
      json_extract(c.data, '$.properties.notes_last_updated'),
      json_extract(c.data, '$.properties.hs_lastmodifieddate'),
      c.created_at
    ))) AS INTEGER) AS days_stale,
    (SELECT COUNT(*) FROM hubspot_associations a
     JOIN hubspot_deals_crm d ON a.from_type = 'contacts' AND a.to_type = 'deals'
     WHERE a.from_id = c.id AND a.to_id = d.id
       AND COALESCE(d.archived, 0) = 0
       AND NOT ` + dealClosedSQL("d") + `
       AND COALESCE(json_extract(d.data, '$.properties.dealstage'), '') NOT IN (?)
    ) AS deal_count,
    (SELECT json_extract(d.data, '$.properties.dealstage') FROM hubspot_associations a
     JOIN hubspot_deals_crm d ON a.from_type = 'contacts' AND a.to_type = 'deals'
     WHERE a.from_id = c.id AND a.to_id = d.id
       AND COALESCE(d.archived, 0) = 0
       AND NOT ` + dealClosedSQL("d") + `
       AND COALESCE(json_extract(d.data, '$.properties.dealstage'), '') NOT IN (?)
     ORDER BY julianday(d.updated_at) DESC LIMIT 1
    ) AS latest_deal_stage,
    COALESCE((SELECT MAX(CAST(json_extract(d.data, '$.properties.amount') AS REAL)) FROM hubspot_associations a
     JOIN hubspot_deals_crm d ON a.from_type = 'contacts' AND a.to_type = 'deals'
     WHERE a.from_id = c.id AND a.to_id = d.id
       AND COALESCE(d.archived, 0) = 0
       AND NOT ` + dealClosedSQL("d") + `
       AND COALESCE(json_extract(d.data, '$.properties.dealstage'), '') NOT IN (?)
    ), 0) AS top_deal_amount
  FROM hubspot_contacts_crm c
  WHERE COALESCE(c.archived, 0) = 0
    AND (? = '' OR json_extract(c.data, '$.properties.hubspot_owner_id') = ?)
)
WHERE days_stale >= ? AND deal_count > 0
ORDER BY days_stale DESC
LIMIT ?`
		rows, err := db.DB().QueryContext(cmd.Context(), q, stageUnder, stageUnder, stageUnder, ownerID, ownerID, staleDays, limit)
		if err != nil {
			return nil, source, fmt.Errorf("query: %w", err)
		}
		defer rows.Close()
		items := []nurtureMineRow{}
		for rows.Next() {
			var r nurtureMineRow
			var lc, stage sql.NullString
			var amt sql.NullFloat64
			var dealCount int64
			if err := rows.Scan(&r.ContactID, &r.Name, &r.Email, &lc, &r.DaysStale, &dealCount, &stage, &amt); err != nil {
				return nil, source, err
			}
			r.LastContacted = nullStr(lc)
			latest, top := nullStr(stage), nullF(amt)
			r.DealCount, r.LatestDealStage, r.TopDealAmount = &dealCount, &latest, &top
			items = append(items, r)
		}
		if err := rows.Err(); err != nil {
			return nil, source, fmt.Errorf("iterating nurture rows: %w", err)
		}
		return items, source, nil
	}

	source = nurtureNoAssociations
	q := `
SELECT * FROM (
  SELECT c.id,
    TRIM(COALESCE(json_extract(c.data, '$.properties.firstname'), '') || ' ' ||
         COALESCE(json_extract(c.data, '$.properties.lastname'), '')) AS name,
    COALESCE(json_extract(c.data, '$.properties.email'), '') AS email,
    json_extract(c.data, '$.properties.notes_last_contacted') AS last_contacted,
    CAST((julianday('now') - julianday(COALESCE(
      json_extract(c.data, '$.properties.notes_last_contacted'),
      json_extract(c.data, '$.properties.notes_last_updated'),
      json_extract(c.data, '$.properties.hs_lastmodifieddate'),
      c.created_at
    ))) AS INTEGER) AS days_stale
  FROM hubspot_contacts_crm c
  WHERE COALESCE(c.archived, 0) = 0
    AND (? = '' OR json_extract(c.data, '$.properties.hubspot_owner_id') = ?)
)
WHERE days_stale >= ?
ORDER BY days_stale DESC
LIMIT ?`
	rows, err := db.DB().QueryContext(cmd.Context(), q, ownerID, ownerID, staleDays, limit)
	if err != nil {
		return nil, source, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	items := []nurtureMineRow{}
	for rows.Next() {
		var r nurtureMineRow
		var lc sql.NullString
		if err := rows.Scan(&r.ContactID, &r.Name, &r.Email, &lc, &r.DaysStale); err != nil {
			return nil, source, err
		}
		r.LastContacted = nullStr(lc)
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, source, fmt.Errorf("iterating nurture rows: %w", err)
	}
	return items, source, nil
}
