// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelAuditExportCmd(flags *rootFlags) *cobra.Command {
	var flagAllTenants bool
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "export",
		Short:       "Export the Unified Audit log per-tenant or across all tenants to JSONL/CSV and persist it locally",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Export dumps the locally synced Unified Audit (ActionLog) records, which the
local store retains BEYOND ThreatLocker's 31-day server-side cliff. Default
output is JSONL (one record per line) for SIEM ingestion; --csv emits the
promoted columns, --agent/--json emits a JSON array.

Sync first: threatlocker-cli sync --resources audit`,
		Example: strings.Trim(`
  threatlocker-cli audit export --all-tenants --since 2026-04-01 > audit.jsonl
  threatlocker-cli audit export --all-tenants --since 7d --csv > audit.csv
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			cutoff, ok := resolveSince(flagSince, time.Now())
			if !ok {
				return fmt.Errorf("invalid --since %q: use a window like 7d/12h or a date like 2026-04-01", flagSince)
			}

			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources audit' first.", err)
			}
			defer db.Close()

			query := `SELECT data, organization_id, computer_name, hostname, full_path, process_path, action, action_id, hash, date_created, username
				FROM audit`
			var clauses []string
			var sqlArgs []any
			if !flagAllTenants && flags.orgID != "" {
				clauses = append(clauses, "organization_id = ?")
				sqlArgs = append(sqlArgs, flags.orgID)
			}
			query += whereClause(clauses)
			query += ` ORDER BY date_created DESC`

			rows, err := db.DB().QueryContext(cmd.Context(), query, sqlArgs...)
			if err != nil {
				return fmt.Errorf("querying audit: %w", err)
			}
			defer rows.Close()

			type auditRow struct {
				raw         json.RawMessage
				orgID       string
				computer    string
				hostname    string
				fullPath    string
				processPath string
				action      string
				actionID    *int
				hash        string
				dateCreated string
				username    string
			}
			var records []auditRow
			for rows.Next() {
				var raw []byte
				var orgID, computer, hostname, fullPath, processPath, action, hash, dateCreated, username *string
				var actionID *int
				if err := rows.Scan(&raw, &orgID, &computer, &hostname, &fullPath, &processPath, &action, &actionID, &hash, &dateCreated, &username); err != nil {
					return fmt.Errorf("scanning audit row: %w", err)
				}
				// Client-side date filter (date formats vary; parse defensively).
				if !cutoff.IsZero() {
					if t, ok := parseTLTime(tlString(dateCreated)); ok && t.Before(cutoff) {
						continue
					}
				}
				records = append(records, auditRow{
					raw: json.RawMessage(raw), orgID: tlString(orgID), computer: tlString(computer),
					hostname: tlString(hostname), fullPath: tlString(fullPath), processPath: tlString(processPath),
					action: tlString(action), actionID: actionID, hash: tlString(hash),
					dateCreated: tlString(dateCreated), username: tlString(username),
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating audit rows: %w", err)
			}

			switch {
			case flags.asJSON:
				arr := make([]json.RawMessage, len(records))
				for i, r := range records {
					arr[i] = r.raw
				}
				return flags.printJSON(cmd, arr)
			case flags.csv:
				w := csv.NewWriter(out)
				defer w.Flush()
				_ = w.Write([]string{"date_created", "organization_id", "computer_name", "hostname", "action", "full_path", "process_path", "hash", "username"})
				for _, r := range records {
					_ = w.Write([]string{r.dateCreated, r.orgID, r.computer, r.hostname, r.action, r.fullPath, r.processPath, r.hash, r.username})
				}
				return nil
			default:
				// JSONL: one record's raw payload per line.
				for _, r := range records {
					fmt.Fprintln(out, string(r.raw))
				}
				return nil
			}
		},
	}
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Export from every synced organization (default scopes to --org when set)")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only export records on/after this point (window like 7d/12h or a date like 2026-04-01)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// whereClause joins clauses into a SQL WHERE fragment (empty when no clauses).
func whereClause(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}
