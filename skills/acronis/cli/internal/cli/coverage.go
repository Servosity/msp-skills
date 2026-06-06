// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type coverageRow struct {
	TenantID      string `json:"tenant_id"`
	Name          string `json:"name"`
	OfferingCount int    `json:"offering_count"`
	OnlineAgents  int    `json:"online_agents"`
	RecentSuccess bool   `json:"recent_success"`
	Protected     bool   `json:"protected"`
}

// computeCoverage finds paying tenants (>=1 offering_items row) and whether
// they are protected (>=1 online agent AND >=1 recent successful task within
// withinDays). When unprotectedOnly, returns only paying-but-unprotected.
func computeCoverage(db *store.Store, withinDays int, unprotectedOnly bool, now time.Time) ([]coverageRow, error) {
	names := tenantNames(db)
	cutoff := now.Add(-time.Duration(withinDays) * 24 * time.Hour)

	offeringCount := map[string]int{}
	if rows, err := db.Query(`SELECT tenants_id, COUNT(*) FROM offering_items GROUP BY tenants_id`); err == nil {
		for rows.Next() {
			var tid string
			var c int
			if rows.Scan(&tid, &c) == nil {
				offeringCount[tid] = c
			}
		}
		rows.Close()
	} else {
		return nil, fmt.Errorf("querying offering_items: %w", err)
	}

	onlineAgents := map[string]int{}
	if rows, err := db.Query(`SELECT tenant_id, status FROM agent_manager`); err == nil {
		for rows.Next() {
			var tid, status *string
			if rows.Scan(&tid, &status) != nil {
				continue
			}
			if agentOnline(deref(status)) {
				onlineAgents[deref(tid)]++
			}
		}
		rows.Close()
	} else {
		return nil, fmt.Errorf("querying agent_manager: %w", err)
	}

	recentSuccess := map[string]bool{}
	if rows, err := db.Query(`SELECT tenant_id, state, result_code, created_at, completed_at FROM task_manager`); err == nil {
		for rows.Next() {
			var tid, state, rc, createdAt, completedAt *string
			if rows.Scan(&tid, &state, &rc, &createdAt, &completedAt) != nil {
				continue
			}
			if taskOutcome(deref(state), deref(rc)) != "ok" {
				continue
			}
			for _, ts := range []string{deref(completedAt), deref(createdAt)} {
				if t, ok := parseTime(ts); ok && t.After(cutoff) {
					recentSuccess[deref(tid)] = true
				}
			}
		}
		rows.Close()
	} else {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}

	out := make([]coverageRow, 0, len(offeringCount))
	for tid, oc := range offeringCount {
		online := onlineAgents[tid]
		recent := recentSuccess[tid]
		protected := online >= 1 && recent
		if unprotectedOnly && protected {
			continue
		}
		out = append(out, coverageRow{
			TenantID:      tid,
			Name:          names[tid],
			OfferingCount: oc,
			OnlineAgents:  online,
			RecentSuccess: recent,
			Protected:     protected,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Protected != out[j].Protected {
			return !out[i].Protected // unprotected first
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out, nil
}

// pp:data-source local
func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagUnprotected bool
	var withinDays int
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "Surface tenants that pay for protection but have no online agent or no recent successful backup.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			rowsOut, err := computeCoverage(db, withinDays, flagUnprotected, time.Now())
			if err != nil {
				return err
			}
			if limit > 0 && len(rowsOut) > limit {
				rowsOut = rowsOut[:limit]
			}

			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, rowsOut)
			}
			if len(rowsOut) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No paying tenants found — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-22s %9s %8s %10s %s\n", "TENANT_ID", "NAME", "OFFERINGS", "ONLINE", "RECENT_OK", "PROTECTED")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-22s %9d %8d %10t %t\n", r.TenantID, truncate(r.Name, 22), r.OfferingCount, r.OnlineAgents, r.RecentSuccess, r.Protected)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagUnprotected, "unprotected", false, "Show only paying-but-unprotected tenants")
	cmd.Flags().IntVar(&withinDays, "within-days", 7, "A successful task must be this recent to count as protection")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tenants to show (0 = all)")
	return cmd
}
