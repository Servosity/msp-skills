// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: backup-stale.
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"afi-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// Codex r1 (afi 4.34.2 review) class archive-creation-read-as-backup-time:
// spec.json v1Archive is a backup sequence (a full backup and the incremental
// backups after it) and its created_at dates the archive, not its latest
// backup; the public API has no per-resource last-backup time. So a protected
// resource is flagged only on what the store shows: no archive at all
// (never_archived), or a tenant whose synced task statistics record no
// finished task (done or warnings) in their window (no_finished_task_in_window).
type backupStaleItem struct {
	ResourceID      string `json:"resource_id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	TenantID        string `json:"tenant_id"`
	NewestArchiveAt string `json:"newest_archive_at,omitempty"` // when the newest backup sequence began
	NeverArchived   bool   `json:"never_archived"`
	Reason          string `json:"reason"`
	Detail          string `json:"detail,omitempty"`
}

type backupStaleView struct {
	Items            []backupStaleItem `json:"items"`
	ProtectedTotal   int               `json:"protected_total"`
	ProtectedScanned int               `json:"protected_scanned"`
	Note             string            `json:"note,omitempty"`
	Incomplete       []string          `json:"incomplete,omitempty"`
}

func newNovelBackupStaleCmd(flags *rootFlags) *cobra.Command {
	var flagMaxAge string
	var flagTenants string
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "backup-stale",
		Short: "Flag protected resources that are not getting backed up: no archive at all, or no backup task finished in their tenant",
		Long: strings.TrimSpace(`
Read the local store (run 'afi-cli fleet-sync' first) and flag protected
resources with no archive at all (never_archived), and the protected
resources of a tenant whose synced task statistics record no finished task
(done or warnings) in their window (no_finished_task_in_window; set the window
with 'fleet-sync --task-window'). Afi's API has no last-backup time per
resource: an archive's created_at is when its backup sequence began, and
incremental backups extend it, so archive age is reported, never judged.

Use this command to find protected resources that are not getting backed up.
Do NOT use this command to find resources with NO protection at all; use 'coverage-gaps' instead.`),
		Example: strings.Trim(`
  # Protected resources with no archive, or in a tenant with no finished backup task
  afi-cli backup-stale --json

  # One tenant, agent-shaped
  afi-cli backup-stale --tenant 01F000000000000411Z1101G1Y --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan local archives and task statistics for protected resources that are not getting backed up")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "backup-stale"); err != nil {
				return err
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "archives", flags)

			tenantFilter := csvSet(flagTenants)
			idle, err := tenantsWithNoFinishedTask(cmd.Context(), db)
			if err != nil {
				return err
			}

			// True fleet denominator, independent of the output cap.
			var protectedTotal int
			if err := db.DB().QueryRowContext(cmd.Context(), `
				SELECT COUNT(DISTINCT json_extract(data,'$.resource_id'))
				FROM resources WHERE resource_type = 'protections'`).Scan(&protectedTotal); err != nil {
				return fmt.Errorf("counting protections: %w", err)
			}

			// Protected resources joined to their newest archive. Both the
			// protections rows and archives rows carry tenant_id (injected at
			// fleet-sync time when the API omits it).
			query := `
				SELECT pr.rid,
				       COALESCE(json_extract(r.data,'$.name'), ''),
				       COALESCE(json_extract(r.data,'$.kind'), ''),
				       COALESCE(json_extract(r.data,'$.tenant_id'), pr.tid, ''),
				       MAX(json_extract(a.data,'$.created_at'))
				FROM (
				    SELECT DISTINCT json_extract(data,'$.resource_id') AS rid,
				           json_extract(data,'$.tenant_id') AS tid
				    FROM resources WHERE resource_type = 'protections'
				) pr
				LEFT JOIN resources r
				       ON r.resource_type = 'resources' AND r.id = pr.rid
				LEFT JOIN resources a
				       ON a.resource_type = 'archives'
				      AND json_extract(a.data,'$.resource_id') = pr.rid
				WHERE pr.rid IS NOT NULL
				GROUP BY pr.rid`
			rows, err := db.DB().QueryContext(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("querying backup staleness: %w", err)
			}
			defer rows.Close()

			items := make([]backupStaleItem, 0)
			scanned := 0
			capped := false
			for rows.Next() {
				// One uniform cap for every append path (never_archived,
				// unparseable, and stale alike); --limit <= 0 means no cap.
				if flagLimit > 0 && len(items) >= flagLimit {
					capped = true
					break
				}
				var it backupStaleItem
				var newest sql.NullString
				if err := rows.Scan(&it.ResourceID, &it.Name, &it.Kind, &it.TenantID, &newest); err != nil {
					return fmt.Errorf("scanning staleness row: %w", err)
				}
				scanned++
				if len(tenantFilter) > 0 && !tenantFilter[it.TenantID] {
					continue
				}
				it.NewestArchiveAt = newest.String
				switch window, quiet := idle[it.TenantID]; {
				case !newest.Valid || newest.String == "":
					it.NeverArchived, it.Reason = true, "never_archived"
					it.Detail = "protected, with no archive"
				case quiet:
					it.Reason = "no_finished_task_in_window"
					it.Detail = "no task finished in this tenant " + window
				default:
					continue
				}
				items = append(items, it)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating staleness rows: %w", err)
			}

			view := backupStaleView{Items: items, ProtectedTotal: protectedTotal, ProtectedScanned: scanned}
			if protectedTotal == 0 {
				view.Note = "no protections in the local store; run 'afi-cli fleet-sync' to populate it"
			} else if len(items) == 0 && !capped {
				view.Note = fmt.Sprintf("all %d protected resources have an archive, and no tenant's synced task statistics are without a finished task", protectedTotal)
			} else if capped {
				view.Note = fmt.Sprintf("output capped at --limit=%d; more stale resources may exist", flagLimit)
			}
			if flagMaxAge != "" {
				ignored := "--max-age is ignored: an archive's created_at is when its backup sequence began and incremental backups extend it, so archive age does not date a backup"
				if view.Note != "" {
					ignored = view.Note + "; " + ignored
				}
				view.Note = ignored
			}
			if err := withFleetGaps(cmd.Context(), db, csvSet(flagTenants), &view.Incomplete, &view.Note); err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	// Kept for old scripts; hidden and ignored (see the note it adds).
	cmd.Flags().StringVar(&flagMaxAge, "max-age", "", "Ignored: archive age does not date a backup")
	_ = cmd.Flags().MarkHidden("max-age")
	cmd.Flags().StringVar(&flagTenants, "tenant", "", "Comma-separated tenant IDs to include (default all)")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum stale rows to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}

// tenantsWithNoFinishedTask maps each tenant whose stored task statistics
// record no done or warnings task to a description of their window.
func tenantsWithNoFinishedTask(ctx context.Context, db *store.Store) (map[string]string, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources WHERE resource_type = 'task_stats'`)
	if err != nil {
		return nil, fmt.Errorf("reading task statistics: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("reading task statistics: %w", err)
		}
		total, _, ok := parseTaskStats(data)
		if !ok || total.Done+total.Warnings > 0 {
			continue
		}
		var w struct {
			TenantID string `json:"tenant_id"`
			Start    string `json:"window_start"`
			End      string `json:"window_end"`
		}
		_ = json.Unmarshal(data, &w)
		if w.TenantID == "" {
			w.TenantID = id
		}
		out[w.TenantID] = fmt.Sprintf("between %s and %s (failed: %d)", w.Start, w.End, total.Failed)
	}
	return out, rows.Err()
}
