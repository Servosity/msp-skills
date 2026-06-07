// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: tenant-scorecard.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type scorecardQuota struct {
	Kind     string `json:"kind"`
	Used     string `json:"used"`
	Limit    string `json:"limit"`
	Units    string `json:"units,omitempty"`
	Exceeded bool   `json:"exceeded"`
}

type tenantScorecardView struct {
	TenantID          string           `json:"tenant_id"`
	TenantName        string           `json:"tenant_name"`
	TenantKind        string           `json:"tenant_kind"`
	Region            string           `json:"region,omitempty"`
	OrgID             string           `json:"org_id,omitempty"`
	Resources         int64            `json:"resources"`
	ArchivedResources int64            `json:"archived_resources"`
	Protected         int64            `json:"protected"`
	CoveragePct       float64          `json:"coverage_pct"`
	Policies          int64            `json:"policies"`
	Archives          int64            `json:"archives"`
	NewestArchiveAt   string           `json:"newest_archive_at,omitempty"`
	OldestArchiveAt   string           `json:"oldest_archive_at,omitempty"`
	ArchiveSizeKB     int64            `json:"archive_size_kb"`
	TasksDone         int64            `json:"tasks_done"`
	TasksFailed       int64            `json:"tasks_failed"`
	TasksWarnings     int64            `json:"tasks_warnings"`
	TaskWindowStart   string           `json:"task_window_start,omitempty"`
	TaskWindowEnd     string           `json:"task_window_end,omitempty"`
	Quotas            []scorecardQuota `json:"quotas"`
	QuotasExceeded    []string         `json:"quotas_exceeded"`
	SyncedAt          string           `json:"synced_at,omitempty"`
	Note              string           `json:"note,omitempty"`
}

func newNovelTenantScorecardCmd(flags *rootFlags) *cobra.Command {
	var flagDB string

	cmd := &cobra.Command{
		Use:   "tenant-scorecard <tenant-id>",
		Short: "One tenant's full backup posture: coverage percentage, task status, archive ages, and quota state",
		Long: strings.TrimSpace(`
Roll up locally synced resources, protections, policies, archives, quotas, and
the task-status snapshot for one tenant into a single posture card (run
'afi-cli fleet-sync' first).

Use this command for one tenant's full backup posture (coverage %, last run, quota, archive age).
Do NOT use this command for the all-tenants rollup; use 'fleet-health' instead.`),
		Example: strings.Trim(`
  # The per-customer card for a QBR or ticket
  afi-cli tenant-scorecard 01F000000000000411Z1101G1Y --json

  # Narrow to the fields an agent needs
  afi-cli tenant-scorecard 01F000000000000411Z1101G1Y --agent --select tenant_id,coverage_pct,quotas_exceeded
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "tenant-id=example-tenant-id",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would roll up local store data for the tenant")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "tenant-scorecard"); err != nil {
				return err
			}
			if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a tenant ID is required"))
			}
			tid := strings.TrimSpace(args[0])
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "tenants", flags)

			view := tenantScorecardView{TenantID: tid, Quotas: make([]scorecardQuota, 0), QuotasExceeded: make([]string, 0)}

			// Tenant identity.
			var name, kind, region, orgID, syncedAt sql.NullString
			err = db.DB().QueryRowContext(cmd.Context(), `
				SELECT json_extract(data,'$.name'),
				       json_extract(data,'$.kind'),
				       json_extract(data,'$.region'),
				       json_extract(data,'$.org_id'),
				       synced_at
				FROM resources WHERE resource_type = 'tenants' AND id = ?`, tid).
				Scan(&name, &kind, &region, &orgID, &syncedAt)
			switch {
			case err == sql.ErrNoRows:
				view.Note = "tenant not found in the local store; run 'afi-cli fleet-sync' or check the ID with 'afi-cli resolve'"
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			case err != nil:
				return fmt.Errorf("querying tenant: %w", err)
			}
			view.TenantName, view.TenantKind, view.Region, view.OrgID, view.SyncedAt =
				name.String, kind.String, region.String, orgID.String, syncedAt.String

			// Resource counts.
			if err := db.DB().QueryRowContext(cmd.Context(), `
				SELECT COUNT(*),
				       COALESCE(SUM(CASE WHEN json_extract(data,'$.archived') IN (1,'true') THEN 1 ELSE 0 END), 0)
				FROM resources
				WHERE resource_type = 'resources' AND json_extract(data,'$.tenant_id') = ?`, tid).
				Scan(&view.Resources, &view.ArchivedResources); err != nil {
				return fmt.Errorf("counting resources: %w", err)
			}

			// Protected count (distinct resources).
			if err := db.DB().QueryRowContext(cmd.Context(), `
				SELECT COUNT(DISTINCT json_extract(data,'$.resource_id'))
				FROM resources
				WHERE resource_type = 'protections' AND json_extract(data,'$.tenant_id') = ?`, tid).
				Scan(&view.Protected); err != nil {
				return fmt.Errorf("counting protections: %w", err)
			}
			if view.Resources > 0 {
				view.CoveragePct = float64(view.Protected) / float64(view.Resources) * 100
			}

			// Policies.
			if err := db.DB().QueryRowContext(cmd.Context(), `
				SELECT COUNT(*)
				FROM resources
				WHERE resource_type = 'policies' AND json_extract(data,'$.tenant_id') = ?`, tid).
				Scan(&view.Policies); err != nil {
				return fmt.Errorf("counting policies: %w", err)
			}

			// Archives: count, age range, total size (size is a string-encoded
			// uint64 of KB — CAST handles both numeric and string forms).
			var newest, oldest sql.NullString
			if err := db.DB().QueryRowContext(cmd.Context(), `
				SELECT COUNT(*),
				       MAX(json_extract(data,'$.created_at')),
				       MIN(json_extract(data,'$.created_at')),
				       COALESCE(SUM(CAST(COALESCE(json_extract(data,'$.stats.size'), '0') AS INTEGER)), 0)
				FROM resources
				WHERE resource_type = 'archives' AND json_extract(data,'$.tenant_id') = ?`, tid).
				Scan(&view.Archives, &newest, &oldest, &view.ArchiveSizeKB); err != nil {
				return fmt.Errorf("aggregating archives: %w", err)
			}
			view.NewestArchiveAt, view.OldestArchiveAt = newest.String, oldest.String

			// Quotas.
			qrs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.kind'), ''),
				       COALESCE(json_extract(data,'$.used'), ''),
				       COALESCE(json_extract(data,'$.limit'), ''),
				       COALESCE(json_extract(data,'$.units'), ''),
				       COALESCE(json_extract(data,'$.exceeded'), 0)
				FROM resources
				WHERE resource_type = 'quotas' AND json_extract(data,'$.tenant_id') = ?`, tid)
			if err != nil {
				return fmt.Errorf("querying quotas: %w", err)
			}
			for qrs.Next() {
				var q scorecardQuota
				var exceeded sql.NullString
				if err := qrs.Scan(&q.Kind, &q.Used, &q.Limit, &q.Units, &exceeded); err != nil {
					_ = qrs.Close()
					return fmt.Errorf("scanning quota: %w", err)
				}
				q.Exceeded = exceeded.String == "1" || exceeded.String == "true"
				view.Quotas = append(view.Quotas, q)
				if q.Exceeded {
					view.QuotasExceeded = append(view.QuotasExceeded, q.Kind)
				}
			}
			_ = qrs.Close()
			if err := qrs.Err(); err != nil {
				return fmt.Errorf("iterating quotas: %w", err)
			}

			// Task snapshot.
			var statsData []byte
			var ws, we sql.NullString
			err = db.DB().QueryRowContext(cmd.Context(), `
				SELECT data,
				       json_extract(data,'$.window_start'),
				       json_extract(data,'$.window_end')
				FROM resources WHERE resource_type = 'task_stats' AND id = ?`, tid).
				Scan(&statsData, &ws, &we)
			if err == nil {
				if total, _, ok := parseTaskStats(statsData); ok {
					view.TasksDone, view.TasksFailed, view.TasksWarnings = total.Done, total.Failed, total.Warnings
				}
				view.TaskWindowStart, view.TaskWindowEnd = ws.String, we.String
			} else if err != sql.ErrNoRows {
				return fmt.Errorf("querying task snapshot: %w", err)
			}

			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
