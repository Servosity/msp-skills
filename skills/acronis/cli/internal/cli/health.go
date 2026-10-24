// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type healthRow struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Total    int    `json:"total"`
	OK       int    `json:"ok"`
	Warning  int    `json:"warning"`
	Failed   int    `json:"failed"`
	Running  int    `json:"running"`
	Stale    bool   `json:"stale"`
}

// computeHealth reports every mirrored tenant (a tenant with no tasks is a
// stale row, never an absent one) plus any tenant id that only task rows name.
// Only backup tasks count; an untyped task is counted, but only an explicit
// backup task can be a success or make a tenant fresh. A tenant is "stale" when
// it has no dated backup task within staleDays of now.
func computeHealth(db *store.Store, staleDays int, now time.Time) ([]healthRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}

	agg := map[string]*healthRow{}
	for tid, name := range names {
		agg[tid] = &healthRow{TenantID: tid, Name: name}
	}
	recent := map[string]bool{}
	cutoff := now.Add(-time.Duration(staleDays) * 24 * time.Hour)

	rows, err := db.Query(`SELECT id, tenant_id, type, state, result_code, ` + taskTimeColumns + ` FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var tenantID, typ, state, resultCode, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := rows.Scan(&id, &tenantID, &typ, &state, &resultCode, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("reading task_manager: %w", err)
		}
		kind := taskBackupKind(deref(typ))
		if kind == "other" {
			continue
		}
		at, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		tid := deref(tenantID)
		if tid == "" {
			tid = "(unknown)"
		}
		r, ok := agg[tid]
		if !ok {
			r = &healthRow{TenantID: tid, Name: names[tid]}
			agg[tid] = r
		}
		r.Total++
		outcome := taskOutcome(deref(state), deref(resultCode))
		switch {
		case outcome == "ok" && kind == "backup":
			r.OK++
		case outcome == "warning" && kind == "backup":
			r.Warning++
		case outcome == "failed":
			r.Failed++
		case outcome == "running":
			r.Running++
		}
		if kind == "backup" && known && at.After(cutoff) {
			recent[tid] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading task_manager: %w", err)
	}

	out := make([]healthRow, 0, len(agg))
	for tid, r := range agg {
		r.Stale = !recent[tid]
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Failed != out[j].Failed {
			return out[i].Failed > out[j].Failed
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out, nil
}

// pp:data-source local
func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var staleDays int
	var limit int

	cmd := &cobra.Command{
		Use:         "health",
		Short:       "See backup success / failure / stale across your entire book of customer tenants in one table.",
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

			rowsOut, err := computeHealth(db, staleDays, time.Now())
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
				fmt.Fprintln(cmd.OutOrStdout(), "No task data — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %6s %6s %6s %6s %6s\n", "TENANT_ID", "NAME", "TOTAL", "OK", "WARN", "FAILED", "STALE")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %6d %6d %6d %6d %6t\n", r.TenantID, truncate(r.Name, 24), r.Total, r.OK, r.Warning, r.Failed, r.Stale)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 1, "A tenant is stale if no dated backup task falls within this many days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tenants to show (0 = all)")
	return cmd
}
