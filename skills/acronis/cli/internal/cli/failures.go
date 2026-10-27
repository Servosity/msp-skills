// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	TimeUnknown bool   `json:"time_unknown,omitempty"`

	occurred time.Time
}

// computeFailures returns every failed backup task (explicit backup type, or
// no type at all) whose time falls inside [now-since, now], newest first. A
// failed task the mirror cannot date is listed first with time_unknown set: a
// failure is never hidden because its time is missing, and an unrecognized
// timestamp fails the report.
func computeFailures(db *store.Store, since time.Duration, tenantFilter string, now time.Time) ([]failureRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-since)

	rows, err := db.Query(`SELECT id, tenant_id, type, resource_id, state, result_code, ` + taskTimeColumns + ` FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	defer rows.Close()

	out := []failureRow{}
	for rows.Next() {
		var id string
		var tid, typ, resID, state, rc, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := rows.Scan(&id, &tid, &typ, &resID, &state, &rc, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("reading task_manager: %w", err)
		}
		occurred, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		if taskOutcome(deref(state), deref(rc)) != "failed" || taskBackupKind(deref(typ)) == "other" {
			continue
		}
		if tenantFilter != "" && deref(tid) != tenantFilter {
			continue
		}
		if known && occurred.Before(cutoff) {
			continue
		}
		row := failureRow{
			TenantID:    deref(tid),
			TenantName:  names[deref(tid)],
			TaskID:      id,
			Type:        deref(typ),
			ResourceID:  deref(resID),
			State:       deref(state),
			ResultCode:  deref(rc),
			TimeUnknown: !known,
			occurred:    occurred,
		}
		if known {
			row.OccurredAt = occurred.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading task_manager: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TimeUnknown != out[j].TimeUnknown {
			return out[i].TimeUnknown
		}
		return out[i].occurred.After(out[j].occurred)
	})
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
				occurred := r.OccurredAt
				if r.TimeUnknown {
					occurred = "(unknown)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-24s %-22s %-14s %-10s %s\n",
					occurred, truncate(tenant, 24), truncate(r.Type, 22), truncate(r.ResultCode, 14), r.State, r.TaskID)
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
