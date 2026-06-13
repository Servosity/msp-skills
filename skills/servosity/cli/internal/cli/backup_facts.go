// Copyright 2026 servosity. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/store"
)

// backupFact is the unified shape rendered across all three engines.
// Fields are nullable because no single engine guarantees every key
// (e.g. restic's size field name differs from classic; missing values
// become "-" in table output and null in JSON).
type backupFact struct {
	CompanyID        *int64 `json:"company_id"`
	CompanyName      string `json:"company_name"`
	Engine           string `json:"engine"`
	ID               string `json:"id"`
	Hostname         string `json:"hostname"`
	LastSuccessfulAt string `json:"last_successful_at"`
	State            string `json:"state"`
	Health           string `json:"health"`
}

// pp:data-source local
func newNovelBackupFactsCmd(flags *rootFlags) *cobra.Command {
	var companyID int
	var engine string
	var status string
	var since string
	var refresh bool

	cmd := &cobra.Command{
		Use:         "backup-facts",
		Short:       "Unified backup view across classic, restic, and DR engines",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Produce a unified row-per-backup view across all three Servosity backup engines
(classic /backups/, restic /restic-backups/, and DR /dr-backups/) from the local
synced store. Optional filters scope by company, engine, health, or freshness.

Health is derived from per-backup freshness (pp_last_success): 'ok' means a
successful backup within the last 7 days, 'stale' means none on record or
older than 7 days, 'unknown' means freshness has not been hydrated yet (run
with --refresh once). The STATE column is the API's lifecycle state (e.g.
active); the partner API exposes no per-backup failure status — use 'issues'
for failures.

Requires that 'sync' has been run for the relevant resources at least once.`,
		Example: `  # All backups across all engines, all companies
  servosity-cli backup-facts

  # One company, all engines, last 7 days only
  servosity-cli backup-facts --company 4421 --since 7d

  # Restic backups with no recent success, JSON for piping
  servosity-cli backup-facts --engine restic --status stale --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			// Validate enums up-front so typos exit cleanly rather than
			// silently producing an empty result via a WHERE clause that
			// never matches.
			engine = strings.ToLower(strings.TrimSpace(engine))
			switch engine {
			case "", "all", "classic", "restic", "dr":
			default:
				return usageErr(fmt.Errorf("invalid --engine %q (want classic|restic|dr|all)", engine))
			}
			status = strings.ToLower(strings.TrimSpace(status))
			switch status {
			case "", "all", "ok", "stale":
			case "fail":
				return usageErr(fmt.Errorf("--status fail is not derivable from the partner API (backups carry no failure status); use 'servosity-cli issues list' or 'triage' for failures"))
			default:
				return usageErr(fmt.Errorf("invalid --status %q (want ok|stale|all)", status))
			}

			var sinceTS string
			if since != "" {
				t, err := parseSinceDuration(since)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --since %q: %w", since, err))
				}
				sinceTS = t.UTC().Format(time.RFC3339)
			}

			dbPath := defaultDBPath("servosity-cli")
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'servosity-cli sync' first.", err)
			}
			defer db.Close()

			// last_successful_at is sourced from pp_last_success. Ensure the
			// table exists so the LEFT JOIN never errors on a store that has
			// never hydrated freshness; --refresh repulls it live.
			if err := ensureLastSuccessTable(cmd.Context(), db); err != nil {
				return err
			}
			if refresh {
				c, cerr := flags.newClient()
				if cerr != nil {
					return cerr
				}
				if _, _, herr := hydrateLastSuccess(cmd.Context(), c, db); herr != nil {
					return fmt.Errorf("refreshing last-success freshness: %w", herr)
				}
			}

			// One SELECT per engine UNIONed together. The three engines do
			// NOT agree on how they reference a company:
			//   - classic  nests it: json_extract(data,'$.company.id'/'.name')
			//   - restic/dr store a URL string ("…/companies/4766/"); the id
			//     is parsed out with substr(... instr('/companies/')+11).
			// Using a single '$.company_id' path (the old code) matched none
			// of them, so every row came back company_id=null / name="". The
			// per-engine company_id expression below is the fix.
			//
			// last_successful_at is LEFT-JOINed from pp_last_success (hydrated
			// from the partner-visible /{engine}-backups/{id}/latest-success/
			// endpoint) since none of the backup blobs carry it inline.
			classicCID := `CAST(json_extract(b.data, '$.company.id') AS INTEGER)`
			urlCID := `CAST(substr(json_extract(b.data, '$.company'), instr(json_extract(b.data, '$.company'), '/companies/') + 11) AS INTEGER)`

			classicLeg := `
SELECT
  ` + classicCID + ` AS company_id,
  COALESCE(c.name, json_extract(b.data, '$.company.name'), '') AS company_name,
  'classic' AS engine,
  b.id AS id,
  COALESCE(json_extract(b.data, '$.hostname'),
           json_extract(b.data, '$.display_name'),
           json_extract(b.data, '$.login'), '') AS hostname,
  COALESCE(ls.last_success, '') AS last_successful_at,
  COALESCE(json_extract(b.data, '$.last_status'),
           json_extract(b.data, '$.status'),
           json_extract(b.data, '$.state'), '') AS status
FROM backups b
LEFT JOIN companies c ON CAST(c.id AS INTEGER) = ` + classicCID + `
LEFT JOIN pp_last_success ls ON ls.engine = 'classic' AND ls.backup_id = b.id
`
			resticLeg := `
SELECT
  ` + urlCID + ` AS company_id,
  COALESCE(c.name, '') AS company_name,
  'restic' AS engine,
  b.id AS id,
  COALESCE(json_extract(b.data, '$.hostname'),
           json_extract(b.data, '$.device_name'),
           json_extract(b.data, '$.display_name'),
           json_extract(b.data, '$.login'), '') AS hostname,
  COALESCE(ls.last_success, '') AS last_successful_at,
  COALESCE(json_extract(b.data, '$.last_status'),
           json_extract(b.data, '$.status'),
           json_extract(b.data, '$.state'), '') AS status
FROM restic_backups b
LEFT JOIN companies c ON CAST(c.id AS INTEGER) = ` + urlCID + `
LEFT JOIN pp_last_success ls ON ls.engine = 'restic' AND ls.backup_id = b.id
`
			drLeg := `
SELECT
  ` + urlCID + ` AS company_id,
  COALESCE(c.name, '') AS company_name,
  'dr' AS engine,
  b.id AS id,
  COALESCE(json_extract(b.data, '$.hostname'),
           json_extract(b.data, '$.device_name'),
           json_extract(b.data, '$.display_name'),
           json_extract(b.data, '$.login'), '') AS hostname,
  COALESCE(ls.last_success, '') AS last_successful_at,
  COALESCE(json_extract(b.data, '$.last_status'),
           json_extract(b.data, '$.status'),
           json_extract(b.data, '$.state'), '') AS status
FROM dr_backups b
LEFT JOIN companies c ON CAST(c.id AS INTEGER) = ` + urlCID + `
LEFT JOIN pp_last_success ls ON ls.engine = 'dr' AND ls.backup_id = b.id
`

			// Freshness hydration state drives the health bucket: an
			// un-hydrated store cannot distinguish ok from stale, so every
			// row reports health=unknown plus a stderr hint.
			freshnessHydrated := false
			if r := db.DB().QueryRowContext(cmd.Context(), `SELECT COUNT(*) FROM pp_last_success`); r != nil {
				var n int
				if r.Scan(&n) == nil && n > 0 {
					freshnessHydrated = true
				}
			}
			if !freshnessHydrated {
				fmt.Fprintln(os.Stderr, "hint: per-backup freshness not hydrated yet — LAST_OK is empty and health=unknown. Run 'servosity-cli backup-facts --refresh' once.")
				if status == "ok" || status == "stale" {
					return fmt.Errorf("--status %s needs hydrated freshness; run with --refresh first", status)
				}
			}

			// Engine selection: build UNION ALL of just the legs we need.
			var legs []string
			switch engine {
			case "classic":
				legs = []string{classicLeg}
			case "restic":
				legs = []string{resticLeg}
			case "dr":
				legs = []string{drLeg}
			default: // "" / "all"
				legs = []string{classicLeg, resticLeg, drLeg}
			}

			unioned := "(" + strings.Join(legs, "\nUNION ALL\n") + ")"

			// Outer wrapper applies post-UNION filters so each engine's
			// json_extract logic stays self-contained and the filters
			// hit normalized column aliases.
			var (
				where []string
				bind  []any
			)
			if companyID > 0 {
				where = append(where, "u.company_id = ?")
				bind = append(bind, int64(companyID))
			}
			if sinceTS != "" {
				where = append(where, "u.last_successful_at >= ?")
				bind = append(bind, sinceTS)
			}

			query := "SELECT u.company_id, u.company_name, u.engine, u.id, u.hostname, u.last_successful_at, u.status FROM " + unioned + " AS u"
			if len(where) > 0 {
				query += " WHERE " + strings.Join(where, " AND ")
			}
			query += " ORDER BY u.company_name, u.engine, u.last_successful_at DESC"

			rows, err := db.DB().QueryContext(cmd.Context(), query, bind...)
			if err != nil {
				return fmt.Errorf("querying backup facts: %w", err)
			}
			defer rows.Close()

			var facts []backupFact
			for rows.Next() {
				var (
					cid      sql.NullInt64
					cname    sql.NullString
					eng      sql.NullString
					id       sql.NullString
					hostname sql.NullString
					lastOK   sql.NullString
					st       sql.NullString
				)
				if err := rows.Scan(&cid, &cname, &eng, &id, &hostname, &lastOK, &st); err != nil {
					return fmt.Errorf("scanning backup-facts row: %w", err)
				}
				f := backupFact{
					CompanyName:      cname.String,
					Engine:           eng.String,
					ID:               id.String,
					Hostname:         hostname.String,
					LastSuccessfulAt: lastOK.String,
					State:            st.String,
					Health:           backupHealth(lastOK.String, freshnessHydrated, time.Now()),
				}
				if cid.Valid {
					v := cid.Int64
					f.CompanyID = &v
				}
				if status == "ok" || status == "stale" {
					if f.Health != status {
						continue
					}
				}
				facts = append(facts, f)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating backup-facts rows: %w", err)
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if facts == nil {
					facts = []backupFact{}
				}
				return enc.Encode(facts)
			}

			if len(facts) == 0 {
				fmt.Fprintln(os.Stderr, "no backup facts matched (try 'sync' first, or relax filters)")
				return nil
			}

			headers := []string{"COMPANY", "ENGINE", "ID", "HOSTNAME", "LAST_OK", "STATE", "HEALTH"}
			rowsOut := make([][]string, 0, len(facts))
			for _, f := range facts {
				company := f.CompanyName
				if f.CompanyID != nil {
					if company == "" {
						company = fmt.Sprintf("(%d)", *f.CompanyID)
					} else {
						company = fmt.Sprintf("%s (%d)", company, *f.CompanyID)
					}
				}
				lastOK := f.LastSuccessfulAt
				if lastOK == "" {
					lastOK = "-"
				}
				st := f.State
				if st == "" {
					st = "-"
				}
				host := f.Hostname
				if host == "" {
					host = "-"
				}
				rowsOut = append(rowsOut, []string{company, f.Engine, f.ID, host, lastOK, st, f.Health})
			}
			return flags.printTable(cmd, headers, rowsOut)
		},
	}

	cmd.Flags().IntVar(&companyID, "company", 0, "Filter to one company ID (0 = all)")
	cmd.Flags().StringVar(&engine, "engine", "all", "Engine to include: classic|restic|dr|all")
	cmd.Flags().StringVar(&status, "status", "all", "Status filter: ok|fail|stale|all")
	cmd.Flags().StringVar(&since, "since", "", "Only backups whose last_successful_at is newer than (e.g. 7d, 24h)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-pull per-backup last-success freshness from the API before reporting (restic/dr)")

	return cmd
}

// backupHealth derives the ok/stale/unknown health bucket from per-backup
// freshness. A backup is ok when its last recorded success is within 7 days,
// stale when the success is older or absent, and unknown when the freshness
// table has never been hydrated (so absence of a row proves nothing).
func backupHealth(lastSuccess string, hydrated bool, now time.Time) string {
	if !hydrated {
		return "unknown"
	}
	if lastSuccess == "" {
		return "stale"
	}
	t, err := parseFlexTime(lastSuccess)
	if err != nil {
		return "stale"
	}
	if now.Sub(t) <= 7*24*time.Hour {
		return "ok"
	}
	return "stale"
}
