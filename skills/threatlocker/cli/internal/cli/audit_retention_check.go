// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/store"
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

// auditCoverage reads, per tenant scope, the window end of the last complete
// audit pull of deny events (deny or any-deny, the default archive). A store
// no complete pull has touched has no coverage table yet: that is no coverage,
// not an error. Hand-wired (class: partial coverage read as fresh).
func auditCoverage(ctx context.Context, db *store.Store) (map[string]time.Time, error) {
	coverage := map[string]time.Time{}
	var present int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tlAuditCoverageTable).Scan(&present); err != nil {
		return nil, fmt.Errorf("reading audit coverage: %w", err)
	}
	if present == 0 {
		return coverage, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT scope, covered_until FROM `+tlAuditCoverageTable+`
		WHERE source = 'audit' AND action IN ('deny', 'any-deny')`)
	if err != nil {
		return nil, fmt.Errorf("reading audit coverage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scope, until string
		if err := rows.Scan(&scope, &until); err != nil {
			return nil, fmt.Errorf("scanning audit coverage: %w", err)
		}
		if t, ok := parseTLTime(until); ok && t.After(coverage[scope]) {
			coverage[scope] = t
		}
	}
	return coverage, rows.Err()
}

// coveredUntil is the newest complete coverage that includes org: a pull of
// every tenant, or one that covered org itself.
func coveredUntil(coverage map[string]time.Time, org string) (time.Time, bool) {
	t := coverage[auditCoverageAllScope]
	if own, ok := coverage[org]; ok && own.After(t) {
		t = own
	}
	return t, !t.IsZero()
}

func newNovelAuditRetentionCheckCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "retention-check",
		Short:       "Per tenant, report how close your audit archive is to ThreatLocker's 31-day retention cliff",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		Long: `ThreatLocker retains the Unified Audit (ActionLog) for ~31 days, then discards
it permanently. retention-check inspects the local store and reports, per tenant,
how old your last sync is and how far back your archive reaches — flagging
tenants whose last sync is stale enough that uncaptured records are about to age
off the server forever.

The last sync is the window end of the last COMPLETE audit pull of deny events
(--action deny or any-deny) that covered the tenant. Rows saved by a pull that
ended early do not count, and a pull whose window starts after the previous
covered end leaves a gap, so it does not advance the last sync.

Status: cliff-risk (last sync >= 24d), unverified (rows, but no complete pull
covers the tenant), stale (>= 7d), ok, or no-data.

--org limits the report to one tenant.`,
		Example: "  threatlocker-cli audit retention-check --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli audit pull --since 30d' first.", err)
			}
			defer db.Close()

			coverage, err := auditCoverage(cmd.Context(), db)
			if err != nil {
				return err
			}

			query := `SELECT organization_id, COUNT(*), MIN(date_created), MAX(date_created)
				FROM audit`
			var sqlArgs []any
			if scope := tlNovelTenantScope(false); scope != "" {
				query += ` WHERE organization_id = ?`
				sqlArgs = append(sqlArgs, scope)
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
				var orgID, oldest, newest *string
				var count int
				if err := rows.Scan(&orgID, &count, &oldest, &newest); err != nil {
					return fmt.Errorf("scanning retention row: %w", err)
				}
				daysSinceSync := -1.0
				status := "no-data"
				if count > 0 {
					status = "unverified"
				}
				lastSynced := ""
				if t, ok := coveredUntil(coverage, tlString(orgID)); ok {
					lastSynced = t.UTC().Format(time.RFC3339)
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
					LastSynced: lastSynced, DaysSinceSync: daysSinceSync, Status: status,
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating retention rows: %w", err)
			}
			// Riskiest first.
			rank := map[string]int{"cliff-risk": 0, "unverified": 1, "stale": 2, "no-data": 3, "ok": 4}
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
				fmt.Fprintln(out, "No audit rows in the local store. Run: threatlocker-cli audit pull --since 30d")
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
