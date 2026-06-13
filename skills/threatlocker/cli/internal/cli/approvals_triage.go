// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// triageRow is one pending-approval cluster: the same file (by hash) across one
// or more tenants, collapsed into a single ranked row.
type triageRow struct {
	Hash            string   `json:"hash"`
	FileName        string   `json:"fileName"`
	DuplicateCount  int      `json:"duplicateCount"`
	TenantCount     int      `json:"tenantCount"`
	Tenants         []string `json:"tenants"`
	OldestRequested string   `json:"oldestRequested"`
	AgeHours        float64  `json:"ageHours"`
	AgeBucket       string   `json:"ageBucket"`
}

func newNovelApprovalsTriageCmd(flags *rootFlags) *cobra.Command {
	var flagAllTenants bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "triage",
		Short:       "One ranked queue of every pending application approval across all your managed customer tenants",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Triage builds a single cross-tenant queue of pending application approval
requests from the local store, grouping duplicate requests by file hash so the
same blocked file across many customers collapses into one row. Rows are ranked
oldest-first and bucketed by age (>7d, >48h, >24h, <24h).

Sync first: threatlocker-cli sync --resources approvals`,
		Example: strings.Trim(`
  threatlocker-cli approvals triage --all-tenants --agent
  threatlocker-cli approvals triage --all-tenants --select hash,fileName,duplicateCount,ageBucket
`, "\n"),
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
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources approvals' first.", err)
			}
			defer db.Close()

			query := `SELECT organization_id, organization_name, file_name, hash, date_requested
				FROM approvals WHERE status_id = 1`
			var sqlArgs []any
			if !flagAllTenants && flags.orgID != "" {
				query += ` AND organization_id = ?`
				sqlArgs = append(sqlArgs, flags.orgID)
			}

			rows, err := db.DB().QueryContext(cmd.Context(), query, sqlArgs...)
			if err != nil {
				return fmt.Errorf("querying approvals: %w", err)
			}
			defer rows.Close()

			type agg struct {
				fileName  string
				count     int
				tenants   map[string]bool
				oldest    time.Time
				oldestOK  bool
				oldestRaw string
			}
			clusters := map[string]*agg{}
			now := time.Now()
			for rows.Next() {
				var orgID, orgName, fileName, hash, dateReq *string
				if err := rows.Scan(&orgID, &orgName, &fileName, &hash, &dateReq); err != nil {
					return fmt.Errorf("scanning approval: %w", err)
				}
				h := tlString(hash)
				key := h
				if key == "" {
					key = "(no-hash):" + tlString(fileName)
				}
				a := clusters[key]
				if a == nil {
					a = &agg{tenants: map[string]bool{}}
					clusters[key] = a
				}
				a.count++
				if a.fileName == "" {
					a.fileName = tlString(fileName)
				}
				tenant := tlString(orgName)
				if tenant == "" {
					tenant = tlString(orgID)
				}
				if tenant != "" {
					a.tenants[tenant] = true
				}
				if t, ok := parseTLTime(tlString(dateReq)); ok {
					if !a.oldestOK || t.Before(a.oldest) {
						a.oldest = t
						a.oldestOK = true
						a.oldestRaw = tlString(dateReq)
					}
				} else if a.oldestRaw == "" {
					a.oldestRaw = tlString(dateReq)
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating approvals: %w", err)
			}

			result := make([]triageRow, 0, len(clusters))
			for key, a := range clusters {
				tenants := make([]string, 0, len(a.tenants))
				for t := range a.tenants {
					tenants = append(tenants, t)
				}
				sort.Strings(tenants)
				ageHours := -1.0
				bucket := "unknown"
				if a.oldestOK {
					age := now.Sub(a.oldest)
					ageHours = age.Hours()
					bucket = ageBucketOf(age)
				}
				displayHash := key
				if strings.HasPrefix(displayHash, "(no-hash):") {
					displayHash = ""
				}
				result = append(result, triageRow{
					Hash:            displayHash,
					FileName:        a.fileName,
					DuplicateCount:  a.count,
					TenantCount:     len(a.tenants),
					Tenants:         tenants,
					OldestRequested: a.oldestRaw,
					AgeHours:        ageHours,
					AgeBucket:       bucket,
				})
			}
			// Oldest first; ties broken by duplicate count (most-widespread first).
			sort.Slice(result, func(i, j int) bool {
				if result[i].AgeHours != result[j].AgeHours {
					return result[i].AgeHours > result[j].AgeHours
				}
				return result[i].DuplicateCount > result[j].DuplicateCount
			})

			if flags.asJSON {
				return flags.printJSON(cmd, result)
			}
			if len(result) == 0 {
				fmt.Fprintln(out, "No pending approval requests in the local store. Sync first with: threatlocker-cli sync --resources approvals")
				return nil
			}
			headers := []string{"AGE", "BUCKET", "DUPES", "TENANTS", "FILE", "HASH"}
			tableRows := make([][]string, 0, len(result))
			for _, r := range result {
				age := "?"
				if r.AgeHours >= 0 {
					age = fmt.Sprintf("%.0fh", r.AgeHours)
				}
				hash := r.Hash
				if len(hash) > 16 {
					hash = hash[:16] + "…"
				}
				tableRows = append(tableRows, []string{
					age, r.AgeBucket, fmt.Sprintf("%d", r.DuplicateCount),
					fmt.Sprintf("%d", r.TenantCount), r.FileName, hash,
				})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Include pending approvals from every synced organization (default scopes to --org when set)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
