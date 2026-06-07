// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: coverage-gaps.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type coverageGapItem struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	ExternalID string `json:"external_id"`
	TenantID   string `json:"tenant_id"`
	Archived   bool   `json:"archived"`
}

type coverageGapsView struct {
	Items            []coverageGapItem `json:"items"`
	TotalResources   int               `json:"total_resources"`
	UnprotectedCount int               `json:"unprotected_count"`
	Note             string            `json:"note,omitempty"`
}

func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var flagTenant string
	var flagKind string
	var flagIncludeArchived bool
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "coverage-gaps",
		Short: "Find resources in a tenant that have no backup protection applied — the backup blind spots",
		Long: strings.TrimSpace(`
Find resources with NO backup protection applied, by joining locally synced
resources against protections (run 'afi-cli fleet-sync' first).

Use this command to list resources that lack any protection at all.
Do NOT use this command for protected resources whose backups have gone stale; use 'backup-stale' instead.
Do NOT use this command for the all-tenants health rollup; use 'fleet-health' instead.`),
		Example: strings.Trim(`
  # All unprotected resources across the synced fleet
  afi-cli coverage-gaps --json

  # One tenant, users only, agent-shaped output
  afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --kind user --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would query local store for resources with no protection row")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "coverage-gaps"); err != nil {
				return err
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "resources", flags)

			where := []string{"r.resource_type = 'resources'"}
			params := []any{}
			if flagTenant != "" {
				where = append(where, "json_extract(r.data,'$.tenant_id') = ?")
				params = append(params, flagTenant)
			}
			if flagKind != "" {
				where = append(where, "json_extract(r.data,'$.kind') = ?")
				params = append(params, flagKind)
			}
			if !flagIncludeArchived {
				where = append(where, "COALESCE(json_extract(r.data,'$.archived'), 0) NOT IN (1, 'true')")
			}

			var total int
			countQ := `SELECT COUNT(*) FROM resources r WHERE ` + strings.Join(where, " AND ")
			if err := db.DB().QueryRowContext(cmd.Context(), countQ, params...).Scan(&total); err != nil {
				return fmt.Errorf("counting resources: %w", err)
			}

			query := `
				SELECT r.id,
				       COALESCE(json_extract(r.data,'$.name'), ''),
				       COALESCE(json_extract(r.data,'$.kind'), ''),
				       COALESCE(json_extract(r.data,'$.external_id'), ''),
				       COALESCE(json_extract(r.data,'$.tenant_id'), ''),
				       COALESCE(json_extract(r.data,'$.archived'), 0)
				FROM resources r
				WHERE ` + strings.Join(where, " AND ") + `
				  AND NOT EXISTS (
				      SELECT 1 FROM resources p
				      WHERE p.resource_type = 'protections'
				        AND json_extract(p.data,'$.resource_id') = r.id)
				ORDER BY json_extract(r.data,'$.tenant_id'), r.id
				LIMIT ?`
			// Fetch limit+1 so the "capped" note only fires on real truncation,
			// not when exactly --limit gaps exist. --limit <= 0 means no cap
			// (SQLite treats a negative LIMIT as unlimited), matching
			// backup-stale's semantics.
			sqlLimit := flagLimit + 1
			if flagLimit <= 0 {
				sqlLimit = -1
			}
			rows, err := db.DB().QueryContext(cmd.Context(), query, append(params, sqlLimit)...)
			if err != nil {
				return fmt.Errorf("querying coverage gaps: %w", err)
			}
			defer rows.Close()

			items := make([]coverageGapItem, 0)
			for rows.Next() {
				var it coverageGapItem
				var archived sql.NullString
				if err := rows.Scan(&it.ResourceID, &it.Name, &it.Kind, &it.ExternalID, &it.TenantID, &archived); err != nil {
					return fmt.Errorf("scanning coverage gap row: %w", err)
				}
				it.Archived = archived.String == "1" || archived.String == "true"
				items = append(items, it)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating coverage gaps: %w", err)
			}

			truncated := false
			if flagLimit > 0 && len(items) > flagLimit {
				items = items[:flagLimit]
				truncated = true
			}
			view := coverageGapsView{Items: items, TotalResources: total, UnprotectedCount: len(items)}
			if total == 0 {
				view.Note = "no resources in the local store match the filters; run 'afi-cli fleet-sync' to populate it"
			} else if len(items) == 0 {
				view.Note = "every matching resource has at least one protection"
			} else if truncated {
				view.Note = fmt.Sprintf("output capped at --limit=%d; more gaps may exist", flagLimit)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Restrict to one tenant ID")
	cmd.Flags().StringVar(&flagKind, "kind", "", "Restrict to one resource kind (e.g. user, site, group, drive)")
	cmd.Flags().BoolVar(&flagIncludeArchived, "include-archived", false, "Include resources already archived in Afi")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum gap rows to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
