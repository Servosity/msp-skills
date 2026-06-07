// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: backup-stale.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"afi-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type backupStaleItem struct {
	ResourceID      string  `json:"resource_id"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	TenantID        string  `json:"tenant_id"`
	NewestArchiveAt string  `json:"newest_archive_at,omitempty"`
	AgeHours        float64 `json:"age_hours,omitempty"`
	NeverArchived   bool    `json:"never_archived"`
}

type backupStaleView struct {
	Items            []backupStaleItem `json:"items"`
	ProtectedTotal   int               `json:"protected_total"`
	ProtectedScanned int               `json:"protected_scanned"`
	MaxAge           string            `json:"max_age"`
	Note             string            `json:"note,omitempty"`
}

func newNovelBackupStaleCmd(flags *rootFlags) *cobra.Command {
	var flagMaxAge string
	var flagTenants string
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "backup-stale",
		Short: "Flag protected resources whose most recent backup archive is older than a threshold — protected-but-silently-failing",
		Long: strings.TrimSpace(`
Group locally synced archives by resource, take the newest created_at, and
flag protected resources whose newest backup exceeds --max-age (run
'afi-cli fleet-sync' first). Resources with a protection but no archive at
all are reported as never_archived.

Use this command to find protected resources whose most recent backup is older than the threshold.
Do NOT use this command to find resources with NO protection at all; use 'coverage-gaps' instead.`),
		Example: strings.Trim(`
  # Anything protected whose newest backup is older than 48 hours
  afi-cli backup-stale --max-age 48h --json

  # One tenant, agent-shaped
  afi-cli backup-stale --max-age 7d --tenant 01F000000000000411Z1101G1Y --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan local archives for protected resources with stale newest backup")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "backup-stale"); err != nil {
				return err
			}
			maxAge, err := cliutil.ParseDurationLoose(flagMaxAge)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-age: %w", err))
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "archives", flags)

			tenantFilter := csvSet(flagTenants)
			cutoff := time.Now().UTC().Add(-maxAge)

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

			now := time.Now().UTC()
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
				if !newest.Valid || newest.String == "" {
					it.NeverArchived = true
					items = append(items, it)
					continue
				}
				t, err := time.Parse(time.RFC3339, newest.String)
				if err != nil {
					// Unparseable timestamp: surface it rather than hide it.
					it.NewestArchiveAt = newest.String
					it.NeverArchived = false
					items = append(items, it)
					continue
				}
				if t.Before(cutoff) {
					it.NewestArchiveAt = newest.String
					it.AgeHours = now.Sub(t).Hours()
					items = append(items, it)
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating staleness rows: %w", err)
			}

			view := backupStaleView{Items: items, ProtectedTotal: protectedTotal, ProtectedScanned: scanned, MaxAge: flagMaxAge}
			if protectedTotal == 0 {
				view.Note = "no protections in the local store; run 'afi-cli fleet-sync' to populate it"
			} else if len(items) == 0 && !capped {
				view.Note = fmt.Sprintf("all %d protected resources have an archive newer than %s", protectedTotal, flagMaxAge)
			} else if capped {
				view.Note = fmt.Sprintf("output capped at --limit=%d; more stale resources may exist", flagLimit)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagMaxAge, "max-age", "48h", "Maximum acceptable age of the newest archive (e.g. 48h, 7d, 1w)")
	cmd.Flags().StringVar(&flagTenants, "tenant", "", "Comma-separated tenant IDs to include (default all)")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum stale rows to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
