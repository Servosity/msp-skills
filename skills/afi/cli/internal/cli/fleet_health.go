// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: fleet-health.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"afi-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type fleetHealthRow struct {
	TenantID       string   `json:"tenant_id"`
	TenantName     string   `json:"tenant_name"`
	TenantKind     string   `json:"tenant_kind"`
	TasksDone      int64    `json:"tasks_done"`
	TasksFailed    int64    `json:"tasks_failed"`
	TasksWarnings  int64    `json:"tasks_warnings"`
	WindowStart    string   `json:"window_start,omitempty"`
	WindowEnd      string   `json:"window_end,omitempty"`
	StaleSnapshot  bool     `json:"stale_snapshot,omitempty"`
	QuotasExceeded []string `json:"quotas_exceeded"`
	Resources      int64    `json:"resources"`
	Protected      int64    `json:"protected"`
	CoveragePct    float64  `json:"coverage_pct"`
}

type fleetHealthView struct {
	Tenants       []fleetHealthRow `json:"tenants"`
	TotalFailed   int64            `json:"total_failed"`
	TotalExceeded int              `json:"total_quotas_exceeded"`
	Note          string           `json:"note,omitempty"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var flagTenants string
	var flagSince string
	var flagFailedOnly bool
	var flagDB string

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "One table across all synced tenants: task success/failure counts, quota breaches, and protection coverage",
		Long: strings.TrimSpace(`
Roll up the locally synced task-status snapshots, quota states, and protection
coverage for every tenant into one fleet table (run 'afi-cli fleet-sync'
first; the task counters cover the sync's lookback window).

Use this command for the fleet-wide nightly backup + quota rollup across all tenants.
Do NOT use this command to drill a single tenant's unprotected resources; use 'coverage-gaps' instead.
Do NOT use this command for one tenant's deep posture card; use 'tenant-scorecard' instead.`),
		Example: strings.Trim(`
  # The Monday sweep: every tenant, one table
  afi-cli fleet-health --json

  # Only tenants with failures, snapshot no older than 24h
  afi-cli fleet-health --failed-only --since 24h --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate local task_stats, quotas, and protections per tenant")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "fleet-health"); err != nil {
				return err
			}
			var maxSnapshotAge time.Duration
			if flagSince != "" {
				d, err := cliutil.ParseDurationLoose(flagSince)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since: %w", err))
				}
				maxSnapshotAge = d
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "task_stats", flags)

			tenantFilter := csvSet(flagTenants)

			// Tenant identity.
			type tenantMeta struct{ name, kind string }
			tenants := map[string]tenantMeta{}
			rows, err := db.DB().QueryContext(cmd.Context(), `
				SELECT id,
				       COALESCE(json_extract(data,'$.name'), ''),
				       COALESCE(json_extract(data,'$.kind'), '')
				FROM resources WHERE resource_type = 'tenants'`)
			if err != nil {
				return fmt.Errorf("querying tenants: %w", err)
			}
			for rows.Next() {
				var id, name, kind string
				if err := rows.Scan(&id, &name, &kind); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning tenant: %w", err)
				}
				tenants[id] = tenantMeta{name: name, kind: kind}
			}
			_ = rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating tenants: %w", err)
			}

			// Per-tenant resource counts.
			resourceCounts := map[string]int64{}
			rcs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.tenant_id'), ''), COUNT(*)
				FROM resources WHERE resource_type = 'resources' GROUP BY 1`)
			if err != nil {
				return fmt.Errorf("counting resources: %w", err)
			}
			for rcs.Next() {
				var tid string
				var n int64
				if err := rcs.Scan(&tid, &n); err != nil {
					_ = rcs.Close()
					return fmt.Errorf("scanning resource count: %w", err)
				}
				resourceCounts[tid] = n
			}
			_ = rcs.Close()
			if err := rcs.Err(); err != nil {
				return fmt.Errorf("iterating resource counts: %w", err)
			}

			// Count DISTINCT protected resources, not protection rows.
			protectedCounts := map[string]int64{}
			prs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.tenant_id'), ''),
				       COUNT(DISTINCT json_extract(data,'$.resource_id'))
				FROM resources WHERE resource_type = 'protections' GROUP BY 1`)
			if err != nil {
				return fmt.Errorf("counting protections: %w", err)
			}
			for prs.Next() {
				var tid string
				var n int64
				if err := prs.Scan(&tid, &n); err != nil {
					_ = prs.Close()
					return fmt.Errorf("scanning protection count: %w", err)
				}
				protectedCounts[tid] = n
			}
			_ = prs.Close()
			if err := prs.Err(); err != nil {
				return fmt.Errorf("iterating protection counts: %w", err)
			}

			// Quota breaches.
			exceeded := map[string][]string{}
			qrs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.tenant_id'), ''),
				       COALESCE(json_extract(data,'$.kind'), '')
				FROM resources
				WHERE resource_type = 'quotas'
				  AND json_extract(data,'$.exceeded') IN (1, 'true')`)
			if err != nil {
				return fmt.Errorf("querying quotas: %w", err)
			}
			for qrs.Next() {
				var tid, kind string
				if err := qrs.Scan(&tid, &kind); err != nil {
					_ = qrs.Close()
					return fmt.Errorf("scanning quota: %w", err)
				}
				exceeded[tid] = append(exceeded[tid], kind)
			}
			_ = qrs.Close()
			if err := qrs.Err(); err != nil {
				return fmt.Errorf("iterating quotas: %w", err)
			}

			// Task snapshots.
			type snapshot struct {
				data                   []byte
				windowStart, windowEnd string
				fetchedAt              sql.NullString
			}
			snaps := map[string]snapshot{}
			srs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.tenant_id'), id),
				       data,
				       COALESCE(json_extract(data,'$.window_start'), ''),
				       COALESCE(json_extract(data,'$.window_end'), ''),
				       json_extract(data,'$.fetched_at')
				FROM resources WHERE resource_type = 'task_stats'`)
			if err != nil {
				return fmt.Errorf("querying task stats: %w", err)
			}
			for srs.Next() {
				var tid, ws, we string
				var data []byte
				var fetched sql.NullString
				if err := srs.Scan(&tid, &data, &ws, &we, &fetched); err != nil {
					_ = srs.Close()
					return fmt.Errorf("scanning task stats: %w", err)
				}
				snaps[tid] = snapshot{data: data, windowStart: ws, windowEnd: we, fetchedAt: fetched}
			}
			_ = srs.Close()
			if err := srs.Err(); err != nil {
				return fmt.Errorf("iterating task stats: %w", err)
			}

			// Union of tenant IDs seen anywhere.
			ids := map[string]bool{}
			for id := range tenants {
				ids[id] = true
			}
			for id := range snaps {
				ids[id] = true
			}
			for id := range resourceCounts {
				if id != "" {
					ids[id] = true
				}
			}

			now := time.Now().UTC()
			view := fleetHealthView{Tenants: make([]fleetHealthRow, 0)}
			staleCount := 0
			for id := range ids {
				if len(tenantFilter) > 0 && !tenantFilter[id] {
					continue
				}
				meta := tenants[id]
				row := fleetHealthRow{
					TenantID:       id,
					TenantName:     meta.name,
					TenantKind:     meta.kind,
					Resources:      resourceCounts[id],
					Protected:      protectedCounts[id],
					QuotasExceeded: append([]string{}, exceeded[id]...),
				}
				if row.Resources > 0 {
					row.CoveragePct = float64(row.Protected) / float64(row.Resources) * 100
				}
				if s, ok := snaps[id]; ok {
					total, _, parsed := parseTaskStats(s.data)
					if parsed {
						row.TasksDone, row.TasksFailed, row.TasksWarnings = total.Done, total.Failed, total.Warnings
					}
					row.WindowStart, row.WindowEnd = s.windowStart, s.windowEnd
					if maxSnapshotAge > 0 && s.fetchedAt.Valid {
						if t, err := time.Parse(time.RFC3339, s.fetchedAt.String); err == nil && now.Sub(t) > maxSnapshotAge {
							row.StaleSnapshot = true
							staleCount++
						}
					}
				}
				if flagFailedOnly && row.TasksFailed == 0 && len(row.QuotasExceeded) == 0 {
					continue
				}
				view.TotalFailed += row.TasksFailed
				view.TotalExceeded += len(row.QuotasExceeded)
				view.Tenants = append(view.Tenants, row)
			}
			sort.Slice(view.Tenants, func(i, j int) bool {
				if view.Tenants[i].TasksFailed != view.Tenants[j].TasksFailed {
					return view.Tenants[i].TasksFailed > view.Tenants[j].TasksFailed
				}
				return view.Tenants[i].TenantID < view.Tenants[j].TenantID
			})
			if len(view.Tenants) == 0 {
				if flagFailedOnly {
					view.Note = "no tenants with failed tasks or quota breaches — fleet is green for the synced window"
				} else {
					view.Note = "no tenants in the local store; run 'afi-cli fleet-sync' to populate it"
				}
			}
			if staleCount > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d tenant task snapshots are older than --since=%s; run 'afi-cli fleet-sync' to refresh\n", staleCount, flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagTenants, "tenant", "", "Comma-separated tenant IDs to include (default all)")
	cmd.Flags().StringVar(&flagSince, "since", "", "Flag task snapshots older than this age (e.g. 24h, 7d) as stale")
	cmd.Flags().BoolVar(&flagFailedOnly, "failed-only", false, "Only show tenants with failed tasks or quota breaches")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
