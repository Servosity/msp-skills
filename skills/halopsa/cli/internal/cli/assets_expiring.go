// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"halopsa-pp-cli/internal/store"
)

type assetExpiryRow struct {
	AssetID      string `json:"asset_id"`
	AssetName    string `json:"asset_name"`
	Client       string `json:"client"`
	ContractRef  string `json:"contract_ref,omitempty"`
	EndDate      string `json:"contract_end_date"`
	DaysToExpiry int    `json:"days_to_expiry"`
}

type assetExpiryView struct {
	WithinDays int              `json:"within_days"`
	Rows       []assetExpiryRow `json:"rows"`
	Note       string           `json:"note,omitempty"`
}

// pp:data-source local
func newNovelAssetsExpiringCmd(flags *rootFlags) *cobra.Command {
	var within int
	var client string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "expiring",
		Short: "Assets whose linked contract ends in the next N days, tenant-wide",
		Long: `List assets whose contract_end_date falls within the next N days, joined
to the owning client and sorted by days-to-expiry — the renewal-prep and
proactive-replacement sweep no single portal view provides.

Reads the local sync store. Run 'halopsa-cli sync' first.`,
		Example: `  # Renewal prep: everything expiring in the next 60 days
  halopsa-cli assets expiring --within 60

  # One client, JSON
  halopsa-cli assets expiring --within 90 --client "Acme" --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list assets with contracts expiring in the window")
				return nil
			}
			ctx := cmd.Context()
			if within <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--within must be > 0 days, got %d", within))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("halopsa-cli")
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer func() { _ = db.Close() }()
			if !hintIfUnsynced(cmd, db, "asset") {
				hintIfStale(cmd, db, "asset", flags.maxAge)
			}

			q := `SELECT a.id,
				COALESCE(NULLIF(json_extract(a.data,'$.inventory_number'),''), NULLIF(json_extract(a.data,'$.key_field'),''), a.id) AS name,
				COALESCE(NULLIF(json_extract(a.data,'$.client_name'),''), NULLIF(c.name,''), '(no client)') AS client,
				COALESCE(json_extract(a.data,'$.contract_ref'),'') AS contract_ref,
				COALESCE(NULLIF(a.contract_end_date,''), NULLIF(json_extract(a.data,'$.contract_end_date'),''), '') AS end_date
			FROM asset a
			LEFT JOIN clients c ON CAST(c.id AS INTEGER) = COALESCE(a.client_id, json_extract(a.data,'$.client_id'))
			WHERE COALESCE(NULLIF(a.contract_end_date,''), NULLIF(json_extract(a.data,'$.contract_end_date'),''), '') != ''
			  AND datetime(COALESCE(NULLIF(a.contract_end_date,''), json_extract(a.data,'$.contract_end_date'))) BETWEEN datetime('now') AND datetime('now', '+' || ? || ' days')`
			binds := []any{within}
			if client != "" {
				q += ` AND (json_extract(a.data,'$.client_name') LIKE ? OR c.name LIKE ?)`
				binds = append(binds, "%"+client+"%", "%"+client+"%")
			}
			q += ` ORDER BY end_date ASC`

			rows, err := db.DB().QueryContext(ctx, q, binds...)
			if err != nil {
				return fmt.Errorf("assets-expiring query: %w", err)
			}
			defer rows.Close()
			now := time.Now()
			view := assetExpiryView{WithinDays: within, Rows: []assetExpiryRow{}}
			for rows.Next() {
				var r assetExpiryRow
				var ref sql.NullString
				if rows.Scan(&r.AssetID, &r.AssetName, &r.Client, &ref, &r.EndDate) != nil {
					continue
				}
				r.ContractRef = ref.String
				if t, perr := time.Parse(time.RFC3339, r.EndDate); perr == nil {
					r.DaysToExpiry = int(t.Sub(now).Hours() / 24)
				} else if t, perr := time.Parse("2006-01-02", r.EndDate[:min(10, len(r.EndDate))]); perr == nil {
					r.DaysToExpiry = int(t.Sub(now).Hours() / 24)
				}
				view.Rows = append(view.Rows, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("assets-expiring rows: %w", err)
			}
			if len(view.Rows) == 0 {
				view.Note = fmt.Sprintf("no assets with a contract_end_date in the next %d days (assets without the field are not shown); run sync or widen --within", within)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Assets with contracts expiring within %d days\n\n", within)
			for _, r := range view.Rows {
				fmt.Fprintf(out, "%-28s %-28s %-14s %4dd  %s\n", r.AssetName, r.Client, r.ContractRef, r.DaysToExpiry, r.EndDate)
			}
			if view.Note != "" {
				fmt.Fprintln(out, "note: "+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&within, "within", 60, "Days ahead to scan for contract end dates")
	cmd.Flags().StringVar(&client, "client", "", "Filter by client name substring")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
