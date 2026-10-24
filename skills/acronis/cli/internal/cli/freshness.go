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

type freshnessRow struct {
	TenantID    string  `json:"tenant_id"`
	Name        string  `json:"name,omitempty"`
	LastSuccess string  `json:"last_success,omitempty"`
	AgeHours    float64 `json:"age_hours,omitempty"`
	Breached    bool    `json:"breached"`
}

// computeFreshness reports, per tenant, the time since the last SUCCESSFUL
// backup: an explicit backup task whose result is ok and whose time is known.
// Tenants with no such task are breached by definition. Breach threshold is
// sla. An unrecognized timestamp fails the report.
func computeFreshness(db *store.Store, sla time.Duration, breachedOnly bool, tenantFilter string, now time.Time) ([]freshnessRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}

	lastSuccess := map[string]time.Time{}
	rows, err := db.Query(`SELECT id, tenant_id, type, state, result_code, ` + taskTimeColumns + ` FROM task_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying task_manager: %w", err)
	}
	for rows.Next() {
		var id string
		var tid, typ, state, rc, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := rows.Scan(&id, &tid, &typ, &state, &rc, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reading task_manager: %w", err)
		}
		at, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("task %s: %w", id, err)
		}
		if taskBackupKind(deref(typ)) != "backup" || taskOutcome(deref(state), deref(rc)) != "ok" || !known {
			continue
		}
		if at.After(lastSuccess[deref(tid)]) {
			lastSuccess[deref(tid)] = at
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("reading task_manager: %w", err)
	}

	// Report over all known tenants so never-backed-up tenants surface too.
	// Fall back to tenants seen in task data when the tenants table is empty.
	ids := make([]string, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		for id := range lastSuccess {
			ids = append(ids, id)
		}
	}

	out := []freshnessRow{}
	for _, id := range ids {
		if tenantFilter != "" && id != tenantFilter {
			continue
		}
		r := freshnessRow{TenantID: id, Name: names[id]}
		if t, ok := lastSuccess[id]; ok {
			age := now.Sub(t)
			r.LastSuccess = t.UTC().Format(time.RFC3339)
			r.AgeHours = float64(int(age.Hours()*10)) / 10
			r.Breached = age > sla
		} else {
			r.Breached = true
		}
		if breachedOnly && !r.Breached {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Breached != out[j].Breached {
			return out[i].Breached // breached first
		}
		if out[i].AgeHours != out[j].AgeHours {
			return out[i].AgeHours > out[j].AgeHours
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out, nil
}

// pp:data-source local
func newNovelFreshnessCmd(flags *rootFlags) *cobra.Command {
	var dbPath, slaStr, tenantFilter string
	var breachedOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "freshness",
		Short: "Time since the last successful backup per tenant, flagged against an SLA threshold.",
		Long: `Report, for every tenant, how long ago the last SUCCESSFUL backup task
completed, and flag tenants past the SLA threshold (including tenants that
have never had a successful backup).

Use this for time-since-last-SUCCESSFUL-backup vs an SLA. Do NOT use it to
find agents that stopped checking in; use 'agents stale'. Do NOT use it for
the paying-but-unprotected liability view; use 'coverage'.

Reads the local store; run 'acronis-cli sync' first.`,
		Example: `  # Everything older than the default 24h SLA, breaches first
  acronis-cli freshness

  # Only breaches against a 48h SLA, agent JSON
  acronis-cli freshness --sla 48h --breached --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			sla, err := cliutil.ParseDurationLoose(slaStr)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --sla %q: %w", slaStr, err))
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			rowsOut, err := computeFreshness(db, sla, breachedOnly, tenantFilter, time.Now())
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
				fmt.Fprintln(cmd.OutOrStdout(), "No tenants to report — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %-22s %10s %9s\n", "TENANT_ID", "NAME", "LAST_SUCCESS", "AGE_HOURS", "BREACHED")
			for _, r := range rowsOut {
				last := r.LastSuccess
				if last == "" {
					last = "(never)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %-22s %10.1f %9t\n", r.TenantID, truncate(r.Name, 24), last, r.AgeHours, r.Breached)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().StringVar(&slaStr, "sla", "24h", "Freshness SLA threshold, e.g. 24h, 48h, 7d")
	cmd.Flags().BoolVar(&breachedOnly, "breached", false, "Only show tenants past the SLA")
	cmd.Flags().StringVar(&tenantFilter, "tenant", "", "Only this tenant ID")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
