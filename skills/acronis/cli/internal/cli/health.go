// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
	Failed   int    `json:"failed"`
	Running  int    `json:"running"`
	Stale    bool   `json:"stale"`
}

// computeHealth groups task_manager rows by tenant and classifies each task.
// A tenant is "stale" when it has no task with created_at OR completed_at
// within staleDays of now.
func computeHealth(db *store.Store, staleDays int, now time.Time) ([]healthRow, error) {
	names := tenantNames(db)

	rows, err := db.Query(`SELECT tenant_id, state, result_code, created_at, completed_at FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	agg := map[string]*healthRow{}
	recent := map[string]bool{}
	cutoff := now.Add(-time.Duration(staleDays) * 24 * time.Hour)

	for rows.Next() {
		var tenantID, state, resultCode, createdAt, completedAt *string
		if err := rows.Scan(&tenantID, &state, &resultCode, &createdAt, &completedAt); err != nil {
			continue
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
		switch taskOutcome(deref(state), deref(resultCode)) {
		case "ok":
			r.OK++
		case "failed":
			r.Failed++
		case "running":
			r.Running++
		}
		for _, ts := range []string{deref(createdAt), deref(completedAt)} {
			if t, ok := parseTime(ts); ok && t.After(cutoff) {
				recent[tid] = true
			}
		}
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %6s %6s %6s %6s\n", "TENANT_ID", "NAME", "TOTAL", "OK", "FAILED", "STALE")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %6d %6d %6d %6t\n", r.TenantID, truncate(r.Name, 24), r.Total, r.OK, r.Failed, r.Stale)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 1, "A tenant is stale if no task created/completed within this many days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tenants to show (0 = all)")
	return cmd
}
