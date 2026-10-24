// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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

// computeCoverage finds paying tenants (>=1 offering item that is active or
// whose status is unknown - an unproven status still gets checked) and whether
// they are protected (>=1 agent whose status proves it online AND >=1 dated,
// explicit backup task that succeeded within withinDays). When
// unprotectedOnly, returns only paying-but-unprotected.
func computeCoverage(db *store.Store, withinDays int, unprotectedOnly bool, now time.Time) ([]coverageRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-time.Duration(withinDays) * 24 * time.Hour)

	offeringCount := map[string]int{}
	oRows, err := db.Query(`SELECT tenants_id, data FROM offering_items`)
	if err != nil {
		return nil, fmt.Errorf("querying offering_items: %w", err)
	}
	for oRows.Next() {
		var tid string
		var data []byte
		if err := oRows.Scan(&tid, &data); err != nil {
			oRows.Close()
			return nil, fmt.Errorf("reading offering_items: %w", err)
		}
		obj, err := decodeRow("offering_items", data)
		if err != nil {
			oRows.Close()
			return nil, err
		}
		if offeringStatusOf(obj) != "inactive" {
			offeringCount[tid]++
		}
	}
	err = oRows.Err()
	oRows.Close()
	if err != nil {
		return nil, fmt.Errorf("reading offering_items: %w", err)
	}

	onlineAgents := map[string]int{}
	aRows, err := db.Query(`SELECT tenant_id, status FROM agent_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying agent_manager: %w", err)
	}
	for aRows.Next() {
		var tid, status *string
		if err := aRows.Scan(&tid, &status); err != nil {
			aRows.Close()
			return nil, fmt.Errorf("reading agent_manager: %w", err)
		}
		if agentOnline(deref(status)) {
			onlineAgents[deref(tid)]++
		}
	}
	err = aRows.Err()
	aRows.Close()
	if err != nil {
		return nil, fmt.Errorf("reading agent_manager: %w", err)
	}

	recentSuccess := map[string]bool{}
	tRows, err := db.Query(`SELECT id, tenant_id, type, state, result_code, ` + taskTimeColumns + ` FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	for tRows.Next() {
		var id string
		var tid, typ, state, rc, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := tRows.Scan(&id, &tid, &typ, &state, &rc, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			tRows.Close()
			return nil, fmt.Errorf("reading task_manager: %w", err)
		}
		at, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			tRows.Close()
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		if taskBackupKind(deref(typ)) != "backup" || taskOutcome(deref(state), deref(rc)) != "ok" {
			continue
		}
		if known && at.After(cutoff) {
			recentSuccess[deref(tid)] = true
		}
	}
	err = tRows.Err()
	tRows.Close()
	if err != nil {
		return nil, fmt.Errorf("reading task_manager: %w", err)
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
