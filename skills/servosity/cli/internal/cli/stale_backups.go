// Copyright 2026 servosity. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/store"
)

// neverStaleDays is the synthetic age assigned to a backup that has never
// reported a success. It sorts such backups to the top of the sweep and lets
// them clear any reasonable --days floor without inventing a fake date.
const neverStaleDays = 9999

// deriveStaleEntries turns the hydrated pp_last_success rows into stale-backup
// entries. Each restic/dr backup contributes one row; "never succeeded" rows
// are flagged with last_backup_at="never" and a sentinel age so they always
// surface. Classic backups are absent (no latest-success endpoint exists).
func deriveStaleEntries(ctx context.Context, db *store.Store, now time.Time) ([]staleBackupEntry, error) {
	rows, err := db.DB().QueryContext(ctx, `
		SELECT engine, backup_id, COALESCE(company_id, 0),
		       COALESCE(hostname, ''), COALESCE(last_success, '')
		  FROM pp_last_success`)
	if err != nil {
		return nil, fmt.Errorf("reading freshness table: %w", err)
	}
	defer rows.Close()
	var entries []staleBackupEntry
	for rows.Next() {
		var (
			engine, backupID, hostname, lastSuccess string
			companyID                               int
		)
		if err := rows.Scan(&engine, &backupID, &companyID, &hostname, &lastSuccess); err != nil {
			continue
		}
		e := staleBackupEntry{
			CompanyID: companyID,
			Engine:    engine,
			BackupID:  backupID,
			Hostname:  hostname,
		}
		days, never := staleDaysFrom(lastSuccess, now)
		if never {
			e.LastBackupAt = "never"
			e.DaysStale = neverStaleDays
		} else {
			e.LastBackupAt = lastSuccess
			e.DaysStale = days
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// staleBackupEntry is the per-row shape we expose externally. The upstream
// /reports/stale-backup-sets/ endpoint shape is unspecified, so we probe
// both `{"results": [...]}` and `[...]` and tolerate either snake_case or
// camelCase keys on individual entries.
type staleBackupEntry struct {
	CompanyID    int    `json:"company_id"`
	CompanyName  string `json:"company_name,omitempty"`
	Engine       string `json:"engine"`
	BackupID     string `json:"backup_id"`
	Hostname     string `json:"hostname"`
	LastBackupAt string `json:"last_backup_at"`
	DaysStale    int    `json:"days_stale"`
}

// pp:data-source auto
func newNovelStaleBackupsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var engine string
	var company int
	var refresh bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "stale-backups",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "--refresh=true"},
		Short:       "Friday sweep: companies with stale backups, sliced by age/engine",
		Long: `Slice the stale-backup-sets report by company, age window, and backup engine.

Reads from a local cache by default (pp_stale_snapshot). Pass --refresh to
re-pull the live report and replace the cache before slicing. Joins with
the local companies table to render company names.`,
		Example: `  # Use cached snapshot (run --refresh first if empty)
  servosity-cli stale-backups --days 3

  # Pull live and slice in one go
  servosity-cli stale-backups --refresh --engine restic

  # Single client, restic engine, 7+ days stale
  servosity-cli stale-backups --company 4421 --engine restic --days 7`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			switch engine {
			case "all", "classic", "restic", "dr":
			default:
				return usageErr(fmt.Errorf("invalid --engine %q: must be one of classic, restic, dr, all", engine))
			}

			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}

			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			// v0.2: derive staleness from local backup tables instead of the
			// admin-only /reports/stale-backup-sets/ report (403 for partner
			// tokens). Freshness lives in pp_last_success, hydrated from the
			// partner-visible /{engine}-backups/{id}/latest-success/ endpoint.
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

			now := time.Now()
			entries, err := deriveStaleEntries(cmd.Context(), db, now)
			if err != nil {
				return err
			}
			if len(entries) == 0 && !refresh {
				return fmt.Errorf("no freshness data cached yet. Run with --refresh first " +
					"(pulls per-backup last-success from the partner-visible latest-success endpoint)")
			}

			companyNames := loadCompanyNames(db)

			filtered := make([]staleBackupEntry, 0, len(entries))
			for _, e := range entries {
				if e.DaysStale < days {
					continue
				}
				if engine != "all" && !engineMatches(e.Engine, engine) {
					continue
				}
				if company != 0 && e.CompanyID != company {
					continue
				}
				if name, ok := companyNames[e.CompanyID]; ok {
					e.CompanyName = name
				}
				filtered = append(filtered, e)
			}

			// Worst-first: longest-stale at the top of the sweep.
			sort.SliceStable(filtered, func(i, j int) bool {
				if filtered[i].DaysStale != filtered[j].DaysStale {
					return filtered[i].DaysStale > filtered[j].DaysStale
				}
				return filtered[i].CompanyID < filtered[j].CompanyID
			})

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(filtered)
			}

			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No stale backups match the filter.")
				return nil
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "COMPANY\tENGINE\tBACKUP_ID\tHOSTNAME\tLAST_OK\tDAYS_STALE")
			for _, e := range filtered {
				companyCell := fmt.Sprintf("%s (%d)", coalesceCompany(e.CompanyName), e.CompanyID)
				host := e.Hostname
				if host == "" {
					host = "-"
				}
				daysCell := fmt.Sprintf("%d", e.DaysStale)
				if e.LastBackupAt == "never" {
					daysCell = "never"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					companyCell, e.Engine, e.BackupID, host, e.LastBackupAt, daysCell)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().IntVar(&days, "days", 3, "Only include backups stale for at least N days")
	cmd.Flags().StringVar(&engine, "engine", "all", "Backup engine filter: classic, restic, dr, all")
	cmd.Flags().IntVar(&company, "company", 0, "Only include this company ID (0 = all)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-pull the live /reports/stale-backup-sets/ and replace the local cache")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/servosity-cli/data.db)")

	return cmd
}

// normalizeEngineLabel maps any of the upstream engine spellings down to
// one of our flag values: classic | restic | dr.
func normalizeEngineLabel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "restic", "restic-backup":
		return "restic"
	case "dr", "dr-backup", "disaster_recovery", "disasterrecovery":
		return "dr"
	case "classic", "spx", "backup":
		return "classic"
	}
	return strings.ToLower(s)
}

func engineMatches(rowEngine, filter string) bool {
	return normalizeEngineLabel(rowEngine) == filter
}

// parseFlexTime tries RFC3339 first, then a few common report-time spellings.
func parseFlexTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, l := range layouts {
		t, err := time.Parse(l, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

// loadCompanyNames pulls id -> name from the companies table (best-effort).
// Returns an empty map on any failure so the display still works with
// just numeric IDs.
func loadCompanyNames(db *store.Store) map[int]string {
	out := map[int]string{}
	rows, err := db.Query(`SELECT id, name FROM companies WHERE name IS NOT NULL AND name != ''`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var idStr, name string
		if err := rows.Scan(&idStr, &name); err != nil {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			out[id] = name
		}
	}
	return out
}

func coalesceCompany(name string) string {
	if name == "" {
		return "(unknown)"
	}
	return name
}
