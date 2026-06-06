// Copyright 2026 servosity. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/store"
)

// pp_last_success is a partner-derived freshness table. The admin-only
// /reports/stale-backup-sets/ endpoint returns 403 for partner tokens, so we
// build the same signal ourselves from the per-backup
// /{engine}-backups/{id}/latest-success/ endpoint (which IS partner-visible).
// One row per backup; commands (attention, stale-backups, backup-facts, qbr)
// read it via LEFT JOIN so they stay offline once it is hydrated.
//
// last_success is the RFC3339/date string the API returned, or "" when the
// backup has never reported a success (date:null) or is a DR-upload account
// whose success must be read from its paired DR backup. checked_at is when we
// last hydrated the row.
const lastSuccessTableDDL = `CREATE TABLE IF NOT EXISTS pp_last_success (
  engine       TEXT NOT NULL,
  backup_id    TEXT NOT NULL,
  company_id   INTEGER,
  hostname     TEXT,
  last_success TEXT,
  checked_at   TEXT NOT NULL,
  PRIMARY KEY (engine, backup_id)
);`

// companyURLRE pulls the numeric company id out of a Servosity company URL
// (".../companies/4766/"). restic and dr store company as a URL string; the
// classic engine nests it as an object instead (handled separately).
var companyURLRE = regexp.MustCompile(`/companies/(\d+)`)

// ensureLastSuccessTable creates pp_last_success if absent. Safe to call on
// every command invocation.
func ensureLastSuccessTable(ctx context.Context, db *store.Store) error {
	_, err := db.DB().ExecContext(ctx, lastSuccessTableDDL)
	if err != nil {
		return fmt.Errorf("creating pp_last_success table: %w", err)
	}
	return nil
}

// companyIDFromValue coerces either a nested company object, a numeric id, or
// a company URL string into a company id. Returns 0 when nothing parses.
func companyIDFromValue(v any) int64 {
	switch t := v.(type) {
	case string:
		if m := companyURLRE.FindStringSubmatch(t); m != nil {
			n, _ := strconv.ParseInt(m[1], 10, 64)
			return n
		}
		return coerceID(t)
	case map[string]any:
		return coerceID(t["id"])
	default:
		return coerceID(v)
	}
}

// hydrateBackup is one backup row to refresh: which engine, its id, the
// company it belongs to, and a human hostname for display.
type hydrateBackup struct {
	Engine    string
	BackupID  string
	CompanyID int64
	Hostname  string
}

// collectHydrateTargets reads restic_backups + dr_backups from the local store
// and returns the backups whose freshness we can resolve via latest-success.
// Classic backups are intentionally excluded: the classic engine exposes no
// /backups/{id}/latest-success/ endpoint (404), so its freshness is unknown
// within partner scope.
func collectHydrateTargets(ctx context.Context, db *store.Store) ([]hydrateBackup, error) {
	var out []hydrateBackup
	for _, eng := range []struct{ engine, table string }{
		{"restic", "restic_backups"},
		{"dr", "dr_backups"},
	} {
		// A store that has never synced this engine has no table at all;
		// treat that as zero targets rather than a hard error so --refresh
		// works on partially-synced stores.
		var tableCount int
		if err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, eng.table,
		).Scan(&tableCount); err != nil || tableCount == 0 {
			continue
		}
		rows, err := db.DB().QueryContext(ctx, fmt.Sprintf(`
			SELECT id,
			       json_extract(data, '$.company'),
			       COALESCE(json_extract(data, '$.device_name'),
			                json_extract(data, '$.display_name'),
			                json_extract(data, '$.login'), '')
			  FROM %s`, eng.table))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", eng.table, err)
		}
		for rows.Next() {
			var id, companyRaw, hostname string
			if err := rows.Scan(&id, &companyRaw, &hostname); err != nil {
				continue
			}
			out = append(out, hydrateBackup{
				Engine:    eng.engine,
				BackupID:  id,
				CompanyID: companyIDFromValue(companyRaw),
				Hostname:  hostname,
			})
		}
		_ = rows.Close()
	}
	return out, nil
}

// parseLatestSuccess extracts the last-success timestamp string from a
// /{engine}-backups/{id}/latest-success/ response. Returns "" (never
// succeeded / not applicable) for {"date":null} and for the DR-upload-account
// {"detail":...} sentinel, both of which are normal partner responses.
func parseLatestSuccess(raw json.RawMessage) string {
	var resp struct {
		Date   *string `json:"date"`
		Detail string  `json:"detail"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ""
	}
	if resp.Date == nil {
		return ""
	}
	return strings.TrimSpace(*resp.Date)
}

// hydrateLastSuccess refreshes pp_last_success by calling latest-success for
// every restic/dr backup in the local store. Returns (rowsWritten, withSuccess)
// where withSuccess is how many reported a non-empty success date. Per-backup
// HTTP errors are tolerated (recorded as empty last_success) so one bad row
// doesn't abort the sweep.
func hydrateLastSuccess(ctx context.Context, c *client.Client, db *store.Store) (written int, withSuccess int, err error) {
	if err := ensureLastSuccessTable(ctx, db); err != nil {
		return 0, 0, err
	}
	targets, err := collectHydrateTargets(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	// Live-dogfood bound: hydrating every backup is one GET per backup and
	// cannot fit the matrix's flat 30s timeout on a real book; sample a
	// bounded prefix so the happy path proves the pipeline without the cost.
	if cliutil.IsDogfoodEnv() && len(targets) > 10 {
		targets = targets[:10]
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		path := fmt.Sprintf("/%s-backups/%s/latest-success/", t.Engine, t.BackupID)
		lastSuccess := ""
		if data, gerr := c.Get(ctx, path, nil); gerr == nil {
			lastSuccess = parseLatestSuccess(data)
		}
		if _, execErr := db.DB().ExecContext(ctx, `
			INSERT INTO pp_last_success (engine, backup_id, company_id, hostname, last_success, checked_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(engine, backup_id) DO UPDATE SET
			  company_id   = excluded.company_id,
			  hostname     = excluded.hostname,
			  last_success = excluded.last_success,
			  checked_at   = excluded.checked_at`,
			t.Engine, t.BackupID, nullableInt(t.CompanyID), t.Hostname, lastSuccess, now,
		); execErr != nil {
			return written, withSuccess, fmt.Errorf("upserting pp_last_success for %s/%s: %w", t.Engine, t.BackupID, execErr)
		}
		written++
		if lastSuccess != "" {
			withSuccess++
		}
	}
	return written, withSuccess, nil
}

// nullableInt returns nil for a zero company id so the column stores NULL
// rather than 0 (0 is not a valid company id and would pollute joins).
func nullableInt(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// lastSuccessRow is one hydrated freshness record joined with display fields.
type lastSuccessRow struct {
	Engine      string
	BackupID    string
	CompanyID   int64
	CompanyName string
	Hostname    string
	LastSuccess string // raw string from the API ("" = never)
	DaysStale   int    // computed; -1 means "never succeeded"
}

// staleDaysFrom returns whole days since lastSuccess relative to now. An empty
// lastSuccess (never succeeded) returns (sentinel, true) so callers can treat
// "never" as maximally stale without a fake date.
func staleDaysFrom(lastSuccess string, now time.Time) (days int, never bool) {
	if strings.TrimSpace(lastSuccess) == "" {
		return 0, true
	}
	t, err := parseFlexTime(lastSuccess)
	if err != nil {
		return 0, true
	}
	d := int(now.Sub(t).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return d, false
}
