// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// retentionRow reports a tenant's audit-archive coverage versus ThreatLocker's
// 31-day server-side retention cliff.
type retentionRow struct {
	OrganizationID string  `json:"organizationId"`
	RowCount       int     `json:"rowCount"`
	OldestRecord   string  `json:"oldestRecord"`
	NewestRecord   string  `json:"newestRecord"`
	LastSynced     string  `json:"lastSynced"`
	DaysSinceSync  float64 `json:"daysSinceSync"`
	Status         string  `json:"status"`
}

// ThreatLocker's default ActionLog retention window.
const tlRetentionDays = 31

func newNovelAuditRetentionCheckCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "retention-check",
		Short:       "Per tenant, report how close your audit archive is to ThreatLocker's 31-day retention cliff",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `ThreatLocker retains the Unified Audit (ActionLog) for ~31 days, then discards
it permanently. retention-check inspects the local store and reports, per tenant,
how old your last sync is and how far back your archive reaches — flagging
tenants whose last sync is stale enough that uncaptured records are about to age
off the server forever.

Status: cliff-risk (last sync >= 24d), stale (>= 7d), ok, or no-data.`,
		Example: "  threatlocker-cli audit retention-check --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources audit' first.", err)
			}
			defer db.Close()

			query := `SELECT organization_id, COUNT(*), MIN(date_created), MAX(date_created), MAX(synced_at)
				FROM audit`
			var sqlArgs []any
			if flags.orgID != "" {
				query += ` WHERE organization_id = ?`
				sqlArgs = append(sqlArgs, flags.orgID)
			}
			query += ` GROUP BY organization_id`

			rows, err := db.DB().QueryContext(cmd.Context(), query, sqlArgs...)
			if err != nil {
				return fmt.Errorf("querying audit: %w", err)
			}
			defer rows.Close()

			now := time.Now()
			var result []retentionRow
			for rows.Next() {
				var orgID, oldest, newest, lastSynced *string
				var count int
				if err := rows.Scan(&orgID, &count, &oldest, &newest, &lastSynced); err != nil {
					return fmt.Errorf("scanning retention row: %w", err)
				}
				daysSinceSync := -1.0
				status := "no-data"
				if count > 0 {
					status = "ok"
				}
				if t, ok := parseTLTime(tlString(lastSynced)); ok {
					daysSinceSync = now.Sub(t).Hours() / 24
					switch {
					case daysSinceSync >= tlRetentionDays-7:
						status = "cliff-risk"
					case daysSinceSync >= 7:
						status = "stale"
					default:
						if count > 0 {
							status = "ok"
						}
					}
				}
				result = append(result, retentionRow{
					OrganizationID: tlString(orgID), RowCount: count,
					OldestRecord: tlString(oldest), NewestRecord: tlString(newest),
					LastSynced: tlString(lastSynced), DaysSinceSync: daysSinceSync, Status: status,
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating retention rows: %w", err)
			}
			// Riskiest first.
			rank := map[string]int{"cliff-risk": 0, "stale": 1, "no-data": 2, "ok": 3}
			sort.Slice(result, func(i, j int) bool {
				if rank[result[i].Status] != rank[result[j].Status] {
					return rank[result[i].Status] < rank[result[j].Status]
				}
				return result[i].DaysSinceSync > result[j].DaysSinceSync
			})

			if flags.asJSON {
				return flags.printJSON(cmd, result)
			}
			if len(result) == 0 {
				fmt.Fprintln(out, "No synced audit data. Run: threatlocker-cli sync --resources audit")
				return nil
			}
			headers := []string{"STATUS", "ORG", "ROWS", "LAST_SYNC_DAYS", "OLDEST", "NEWEST"}
			tableRows := make([][]string, 0, len(result))
			for _, r := range result {
				days := "?"
				if r.DaysSinceSync >= 0 {
					days = fmt.Sprintf("%.1f", r.DaysSinceSync)
				}
				tableRows = append(tableRows, []string{r.Status, r.OrganizationID, fmt.Sprintf("%d", r.RowCount), days, r.OldestRecord, r.NewestRecord})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
