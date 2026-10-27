// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type repeatRow struct {
	ResourceID      string `json:"resource_id"`
	TenantID        string `json:"tenant_id"`
	TenantName      string `json:"tenant_name"`
	DistinctFailDay int    `json:"distinct_fail_days"`
	UndatedFailures int    `json:"undated_failures,omitempty"`
	LastFailure     string `json:"last_failure"`
}

// computeAlertsRepeat ranks resources by the count of distinct calendar days
// (UTC) within the window that had a failed backup task. Only backup tasks
// count (explicit backup type, or no type at all so an untyped failure stays
// visible); restores, replications and other task types never do. A failure
// the mirror cannot date is counted in undated_failures rather than dropped,
// and an unrecognized timestamp fails the report.
func computeAlertsRepeat(db *store.Store, days int, now time.Time) ([]repeatRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)

	rows, err := db.Query(`SELECT id, resource_id, tenant_id, state, result_code, type, ` + taskTimeColumns + ` FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	type acc struct {
		tenantID string
		days     map[string]bool
		undated  int
		last     time.Time
	}
	groups := map[string]*acc{}
	for rows.Next() {
		var id string
		var resourceID, tenantID, state, rc, typ, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := rows.Scan(&id, &resourceID, &tenantID, &state, &rc, &typ, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("reading task_manager: %w", err)
		}
		at, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		if taskOutcome(deref(state), deref(rc)) != "failed" || taskBackupKind(deref(typ)) == "other" {
			continue
		}
		if known && !at.After(cutoff) {
			continue
		}
		key := deref(resourceID)
		if key == "" {
			key = deref(tenantID)
		}
		if key == "" {
			key = "(unknown)"
		}
		g, ok := groups[key]
		if !ok {
			g = &acc{tenantID: deref(tenantID), days: map[string]bool{}}
			groups[key] = g
		}
		if g.tenantID == "" {
			g.tenantID = deref(tenantID)
		}
		if !known {
			g.undated++
			continue
		}
		g.days[at.UTC().Format("2006-01-02")] = true
		if at.After(g.last) {
			g.last = at
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading task_manager: %w", err)
	}

	out := make([]repeatRow, 0, len(groups))
	for key, g := range groups {
		row := repeatRow{
			ResourceID:      key,
			TenantID:        g.tenantID,
			TenantName:      names[g.tenantID],
			DistinctFailDay: len(g.days),
			UndatedFailures: g.undated,
		}
		if !g.last.IsZero() {
			row.LastFailure = g.last.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DistinctFailDay != out[j].DistinctFailDay {
			return out[i].DistinctFailDay > out[j].DistinctFailDay
		}
		if out[i].UndatedFailures != out[j].UndatedFailures {
			return out[i].UndatedFailures > out[j].UndatedFailures
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out, nil
}

// pp:data-source local
func newNovelAlertsRepeatCmd(flags *rootFlags) *cobra.Command {
	var flagDays string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "repeat",
		Short:       "Rank resources and tenants by how many distinct days in a window had a failed or missed backup.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			days := 14
			if flagDays != "" {
				var err error
				days, err = atoiPositive(flagDays)
				if err != nil {
					return fmt.Errorf("invalid --days %q: %w", flagDays, err)
				}
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

			rowsOut, err := computeAlertsRepeat(db, days, time.Now())
			if err != nil {
				return err
			}
			if limit > 0 && len(rowsOut) > limit {
				rowsOut = rowsOut[:limit]
			}

			if wantJSON(flags, cmd) {
				if rowsOut == nil {
					rowsOut = []repeatRow{}
				}
				return encodeJSON(cmd, flags, rowsOut)
			}
			if len(rowsOut) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No repeated failures in window — run 'acronis-cli sync' first if you expected data.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-24s %12s %8s %s\n", "RESOURCE", "TENANT", "FAIL_DAYS", "UNDATED", "LAST_FAILURE")
			for _, r := range rowsOut {
				label := r.TenantName
				if label == "" {
					label = r.TenantID
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-24s %12d %8d %s\n", truncate(r.ResourceID, 24), truncate(label, 24), r.DistinctFailDay, r.UndatedFailures, r.LastFailure)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDays, "days", "14", "Window in days to scan for repeated failures")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
