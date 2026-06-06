// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/cliutil"
	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type failureRow struct {
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name,omitempty"`
	TaskID      string `json:"task_id"`
	Type        string `json:"type,omitempty"`
	ResourceID  string `json:"resource_id,omitempty"`
	State       string `json:"state"`
	ResultCode  string `json:"result_code,omitempty"`
	OccurredAt  string `json:"occurred_at,omitempty"`

	occurred time.Time
}

// computeFailures returns every failed/missed task whose completion (or
// creation, when never completed) falls inside [now-since, now], newest first.
func computeFailures(db *store.Store, since time.Duration, tenantFilter string, now time.Time) ([]failureRow, error) {
	names := tenantNames(db)
	cutoff := now.Add(-since)

	rows, err := db.Query(`SELECT id, tenant_id, type, resource_id, state, result_code, created_at, completed_at FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	out := []failureRow{}
	for rows.Next() {
		var id string
		var tid, typ, resID, state, rc, createdAt, completedAt *string
		if rows.Scan(&id, &tid, &typ, &resID, &state, &rc, &createdAt, &completedAt) != nil {
			continue
		}
		if taskOutcome(deref(state), deref(rc)) != "failed" {
			continue
		}
		if tenantFilter != "" && deref(tid) != tenantFilter {
			continue
		}
		var occurred time.Time
		for _, ts := range []string{deref(completedAt), deref(createdAt)} {
			if t, ok := parseTime(ts); ok {
				occurred = t
				break
			}
		}
		if occurred.IsZero() || occurred.Before(cutoff) {
			continue
		}
		out = append(out, failureRow{
			TenantID:   deref(tid),
			TenantName: names[deref(tid)],
			TaskID:     id,
			Type:       deref(typ),
			ResourceID: deref(resID),
			State:      deref(state),
			ResultCode: deref(rc),
			OccurredAt: occurred.UTC().Format(time.RFC3339),
			occurred:   occurred,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].occurred.After(out[j].occurred) })
	return out, nil
}

// pp:data-source local
func newNovelFailuresCmd(flags *rootFlags) *cobra.Command {
	var dbPath, sinceStr, tenantFilter string
	var limit int

	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Flat list of every failed or missed backup task across all tenants in a recent window.",
		Long: `List each individual failed/missed backup task across the whole estate
within a recent window, newest first, with tenant, resource, and result code.

Use this for the flat list of individual failed/missed backups in a recent
window. Do NOT use it for per-tenant pass/fail rollup counts; use 'health'.
Do NOT use it to rank chronic repeat-offenders over many days; use
'alerts repeat'.

Reads the local store; run 'acronis-cli sync' first.`,
		Example: `  # Everything that failed in the last 24 hours
  acronis-cli failures

  # Last 7 days for one tenant, agent JSON
  acronis-cli failures --since 7d --tenant TENANT_ID --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			since, err := cliutil.ParseDurationLoose(sinceStr)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since %q: %w", sinceStr, err))
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			rowsOut, err := computeFailures(db, since, tenantFilter, time.Now())
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
				fmt.Fprintf(cmd.OutOrStdout(), "No failed tasks in the last %s.\n", sinceStr)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-24s %-22s %-14s %-10s %s\n", "OCCURRED", "TENANT", "TYPE", "RESULT", "STATE", "TASK_ID")
			for _, r := range rowsOut {
				tenant := r.TenantName
				if tenant == "" {
					tenant = r.TenantID
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-24s %-22s %-14s %-10s %s\n",
					r.OccurredAt, truncate(tenant, 24), truncate(r.Type, 22), truncate(r.ResultCode, 14), r.State, r.TaskID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().StringVar(&sinceStr, "since", "24h", "Window to scan, e.g. 24h, 7d, 1w")
	cmd.Flags().StringVar(&tenantFilter, "tenant", "", "Only failures for this tenant ID")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
