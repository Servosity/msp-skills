// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type repeatRow struct {
	ResourceID      string `json:"resource_id"`
	TenantID        string `json:"tenant_id"`
	TenantName      string `json:"tenant_name"`
	DistinctFailDay int    `json:"distinct_fail_days"`
	LastFailure     string `json:"last_failure"`
}

type failTask struct {
	resourceID, tenantID, ts string
	isBackup                 bool
}

// computeAlertsRepeat ranks resources by the count of distinct calendar days
// within the window that had a failed/missed task. Backup-typed tasks are
// preferred, but if no backup-typed failures exist in the window the filter is
// relaxed to all task types (lenient).
func computeAlertsRepeat(db *store.Store, days int, now time.Time) ([]repeatRow, error) {
	names := tenantNames(db)
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)

	rows, err := db.Query(`SELECT resource_id, tenant_id, state, result_code, type, created_at, completed_at FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	var fails []failTask
	anyBackup := false
	for rows.Next() {
		var resourceID, tenantID, state, rc, typ, createdAt, completedAt *string
		if rows.Scan(&resourceID, &tenantID, &state, &rc, &typ, &createdAt, &completedAt) != nil {
			continue
		}
		if taskOutcome(deref(state), deref(rc)) != "failed" {
			continue
		}
		// Choose the most relevant timestamp in-window.
		ts := ""
		for _, cand := range []string{deref(completedAt), deref(createdAt)} {
			if t, ok := parseTime(cand); ok && t.After(cutoff) {
				ts = cand
				break
			}
		}
		if ts == "" {
			continue
		}
		isBackup := strings.Contains(strings.ToLower(deref(typ)), "backup")
		if isBackup {
			anyBackup = true
		}
		fails = append(fails, failTask{
			resourceID: deref(resourceID),
			tenantID:   deref(tenantID),
			ts:         ts,
			isBackup:   isBackup,
		})
	}

	type acc struct {
		tenantID string
		days     map[string]bool
		last     string
	}
	groups := map[string]*acc{}
	for _, f := range fails {
		if anyBackup && !f.isBackup {
			continue
		}
		key := f.resourceID
		if key == "" {
			key = f.tenantID
		}
		if key == "" {
			key = "(unknown)"
		}
		g, ok := groups[key]
		if !ok {
			g = &acc{tenantID: f.tenantID, days: map[string]bool{}}
			groups[key] = g
		}
		if g.tenantID == "" {
			g.tenantID = f.tenantID
		}
		day := f.ts
		if t, ok := parseTime(f.ts); ok {
			day = t.Format("2006-01-02")
		}
		g.days[day] = true
		if f.ts > g.last {
			g.last = f.ts
		}
	}

	out := make([]repeatRow, 0, len(groups))
	for key, g := range groups {
		out = append(out, repeatRow{
			ResourceID:      key,
			TenantID:        g.tenantID,
			TenantName:      names[g.tenantID],
			DistinctFailDay: len(g.days),
			LastFailure:     g.last,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DistinctFailDay != out[j].DistinctFailDay {
			return out[i].DistinctFailDay > out[j].DistinctFailDay
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-24s %12s %s\n", "RESOURCE", "TENANT", "FAIL_DAYS", "LAST_FAILURE")
			for _, r := range rowsOut {
				label := r.TenantName
				if label == "" {
					label = r.TenantID
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-24s %12d %s\n", truncate(r.ResourceID, 24), truncate(label, 24), r.DistinctFailDay, r.LastFailure)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDays, "days", "14", "Window in days to scan for repeated failures")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
