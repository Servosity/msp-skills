// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-authored Veeam transcendence commands
// (fleet-health, stale-backups, at-risk, alarms-triage, license-usage,
// company-overview, since). These read the local SQLite mirror that `sync`
// populates and assemble cross-tenant views the per-instance VSPC API never
// returns in one call.
package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/cliutil"
	"veeam-pp-cli/internal/store"
)

// veeamRow is one decoded record from the local `resources` table.
type veeamRow map[string]any

// veeamOpenStore opens the local SQLite mirror read-write (creating and
// migrating it if needed), defaulting to the standard CLI database path when
// dbPath is empty. Used by commands that record local state (license-usage).
func veeamOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("veeam-cli")
	}
	return store.OpenWithContext(ctx, dbPath)
}

// veeamOpenStoreRead opens the local mirror read-only for read commands.
// Read-only handles do not take a write lock, so several read commands can run
// concurrently against the same mirror without SQLITE_BUSY. A missing mirror is
// an error, not an empty report: "no rows" must never read as "all healthy".
func veeamOpenStoreRead(dbPath string) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("veeam-cli")
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, fmt.Errorf("no local mirror at %s: run `veeam-cli sync` first", dbPath)
		}
		return nil, false, err
	}
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, false, err
	}
	return db, true, nil
}

// veeamSyncNames maps documented VSPC list paths to the sync partitions this
// build stores them under. Partition names are generator-derived and move
// between press versions (4.35.1 gave "alarms" to /alarms/templates and
// "infrastructure" to /infrastructure/backupAgents/jobs), so the novels name
// the API path and resolve it through the generated syncResourcePath.
func veeamSyncNames(path string) []string {
	var out []string
	for _, name := range knownSyncResourceNames() {
		if p, err := syncResourcePath(name); err == nil && p == path {
			out = append(out, name)
		}
	}
	return out
}

// veeamLoad returns the decoded records the last finished, unrestricted sync
// walk of each given documented list path returned (veeam_walk.go). It fails
// closed: a path no sync partition fetches, a partition that never synced or
// whose last sync did not finish, one no finished unrestricted walk covers, a
// store read error and an undecodable record are all errors, because a partial
// mirror reported as a whole one reads as a healthy fleet. Rows the walk did
// not return (a narrowed sync's, a live read's write-through, an import's, a
// record the API has since deleted) are not read. Records are de-duplicated on
// instanceUid (VSPC's primary key) so an entity reachable from two partitions
// counts once. NULL/blank data rows are skipped.
func veeamLoad(ctx context.Context, db *store.Store, paths ...string) ([]veeamRow, error) {
	if db == nil {
		return nil, fmt.Errorf("no local mirror: run `veeam-cli sync` first")
	}
	var types []string
	members := map[string]map[string]bool{}
	for _, p := range paths {
		names := veeamSyncNames(p)
		if len(names) == 0 {
			return nil, fmt.Errorf("no sync resource fetches %s in this build", p)
		}
		for _, name := range names {
			if err := veeamSyncComplete(ctx, db, p, name); err != nil {
				return nil, err
			}
			m, err := veeamWalkMembers(ctx, db, p, name)
			if err != nil {
				return nil, err
			}
			members[name] = m
		}
		types = append(types, names...)
	}
	if len(types) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(types))
	args := make([]any, len(types))
	for i, t := range types {
		placeholders[i] = "?"
		args[i] = t
	}
	q := "SELECT resource_type, id, data FROM resources WHERE resource_type IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reading local store: %w", err)
	}
	defer rows.Close()
	out := make([]veeamRow, 0)
	seen := map[string]bool{}
	for rows.Next() {
		var rtype, id string
		var data sql.NullString
		if err := rows.Scan(&rtype, &id, &data); err != nil {
			return nil, fmt.Errorf("reading local store: %w", err)
		}
		if !members[rtype][id] {
			continue
		}
		if !data.Valid || strings.TrimSpace(data.String) == "" {
			continue
		}
		var m veeamRow
		if err := json.Unmarshal([]byte(data.String), &m); err != nil {
			return nil, fmt.Errorf("reading local store: undecodable record: %w", err)
		}
		if uid := strings.ToLower(vstr(m, "instanceUid")); uid != "" {
			if seen[uid] {
				continue
			}
			seen[uid] = true
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading local store: %w", err)
	}
	return out, nil
}

// veeamSyncComplete reports whether the partition's last sync finished
// (sync_state.last_attempt_complete). A partition with no sync record, or one
// whose last attempt stopped early (--max-pages, an error), is incomplete.
// Finished is necessary, not sufficient: veeamWalkMembers then requires a
// finished unrestricted walk. Each error names the sync that fixes it.
func veeamSyncComplete(ctx context.Context, db *store.Store, path, name string) error {
	var complete sql.NullInt64
	err := db.DB().QueryRowContext(ctx, `SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, name).Scan(&complete)
	switch {
	case err == sql.ErrNoRows:
		return fmt.Errorf("%s (sync resource %s) has never been synced: run `%s` first", path, name, veeamWalkFix(ctx, db, name))
	case err != nil:
		return fmt.Errorf("reading sync state for %s: %w", name, err)
	case !complete.Valid || complete.Int64 != 1:
		return fmt.Errorf("the last sync of %s (sync resource %s) did not finish, so this report would be partial: run `%s` to finish it", path, name, veeamWalkFix(ctx, db, name))
	}
	return nil
}

// veeamOrgOf returns the organization a record belongs to. For a backup
// server job that is mappedOrganizationUid, the organization the job is
// assigned to (BackupServerJob.organizationUid names the server's owner);
// other records carry only organizationUid.
func veeamOrgOf(m veeamRow) string {
	if org := vstr(m, "mappedOrganizationUid"); org != "" {
		return org
	}
	return vstr(m, "organizationUid")
}

// veeamJobEnded returns when a job's latest session ended (lastEndTime),
// falling back to when it started (lastRun) while no end is recorded.
func veeamJobEnded(m veeamRow) (time.Time, bool) {
	if t, ok := vtime(m, "lastEndTime"); ok {
		return t, true
	}
	return vtime(m, "lastRun")
}

// veeamAlarmActive reports whether a triggered alarm is still active: its
// latest activation (AlarmActivation.status) is anything but Resolved.
func veeamAlarmActive(m veeamRow) bool {
	return vstr(m, "lastActivation.status") != "Resolved"
}

// VSPC list paths the novels read (documented in spec.json).
const (
	veeamPathCompanies         = "/organizations/companies"
	veeamPathProviderCompanies = "/organizations/provider/companies"
	veeamPathServerJobs        = "/infrastructure/backupServers/jobs"
	veeamPathBackupServers     = "/infrastructure/backupServers"
	// All agent jobs and all agents. The per-OS lists (/backupAgents/windows,
	// /backupAgents/windows/jobs, ...) repeat the same entities, and the per-OS
	// agent items carry neither status nor organizationUid.
	veeamPathAgentJobs        = "/infrastructure/backupAgents/jobs"
	veeamPathAgents           = "/infrastructure/backupAgents"
	veeamPathActiveAlarms     = "/alarms/active"
	veeamPathAlarmTemplates   = "/alarms/templates"
	veeamPathWorkloadsServer  = "/protectedWorkloads/computersManagedByBackupServer"
	veeamPathWorkloadsConsole = "/protectedWorkloads/computersManagedByConsole"
	veeamPathLicenseUsage     = "/licensing/usage/organizations"
)

// veeamUsedPoints sums the license points an organization consumes: the
// usedUnits of every workload whose unitType is Points, across every server
// (OrganizationLicenseUsage.servers[].workloads[] in spec.json). Workloads
// counted in Instances are not points and are left out.
func veeamUsedPoints(m veeamRow) float64 {
	var total float64
	servers, _ := vget(m, "servers").([]any)
	for _, s := range servers {
		sm, _ := s.(map[string]any)
		workloads, _ := sm["workloads"].([]any)
		for _, w := range workloads {
			wm, _ := w.(map[string]any)
			if unit, _ := wm["unitType"].(string); unit != "Points" {
				continue
			}
			if v, ok := wm["usedUnits"].(float64); ok {
				total += v
			}
		}
	}
	return total
}

// vget walks a dotted path through nested maps and returns the raw value.
func vget(m veeamRow, path string) any {
	var cur any = map[string]any(m)
	for _, seg := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[seg]
	}
	return cur
}

// vstr returns a string for a dotted path, coercing numbers/bools sensibly.
func vstr(m veeamRow, path string) string {
	switch v := vget(m, path).(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// vnum returns a float64 for a dotted path; missing/non-numeric yields 0.
func vnum(m veeamRow, path string) float64 {
	switch v := vget(m, path).(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
	}
}

// vbool returns (value, present) for a boolean dotted path.
func vbool(m veeamRow, path string) (bool, bool) {
	if v, ok := vget(m, path).(bool); ok {
		return v, true
	}
	return false, false
}

// vtime parses a timestamp at a dotted path. ok is false when the value is
// absent or unparseable.
func vtime(m veeamRow, path string) (time.Time, bool) {
	return parseVeeamTime(vstr(m, path))
}

// parseVeeamTime parses the ISO8601/RFC3339 timestamps VSPC emits.
func parseVeeamTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// veeamCompanyNames builds an organizationUid -> company name map from synced
// companies so cross-tenant views can show names instead of GUIDs.
func veeamCompanyNames(ctx context.Context, db *store.Store) (map[string]string, error) {
	rows, err := veeamLoad(ctx, db, veeamPathCompanies, veeamPathProviderCompanies)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		uid := strings.ToLower(vstr(r, "instanceUid"))
		name := vstr(r, "name")
		if uid != "" && name != "" {
			names[uid] = name
		}
	}
	return names, nil
}

// veeamAlarmNames maps alarm template UIDs to template names. An active alarm
// (ActiveAlarm in spec.json) names its template only by alarmTemplateUid.
func veeamAlarmNames(ctx context.Context, db *store.Store) (map[string]string, error) {
	rows, err := veeamLoad(ctx, db, veeamPathAlarmTemplates)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		if uid, name := strings.ToLower(vstr(r, "instanceUid")), vstr(r, "name"); uid != "" && name != "" {
			names[uid] = name
		}
	}
	return names, nil
}

// veeamCompanyLabel resolves an org uid to a display name, falling back to the
// uid (or a placeholder when empty).
func veeamCompanyLabel(names map[string]string, uid string) string {
	if strings.TrimSpace(uid) == "" {
		return "(unassigned)"
	}
	if n, ok := names[strings.ToLower(uid)]; ok {
		return n
	}
	return uid
}

// veeamJobHealth classifies a job status by its documented enum value
// (BackupServerJob.status, BackupAgentJob.status in spec.json). Exact match,
// never substring: an unlisted value is "other", not a guess.
func veeamJobHealth(status string) string {
	switch strings.TrimSpace(status) {
	case "":
		return "unknown"
	case "Success":
		return "success"
	case "Warning":
		return "warning"
	case "Failed":
		return "failed"
	case "Running", "Starting", "Stopping", "Enabling", "Disabling", "WaitingTape", "WaitingRepository":
		return "running"
	default:
		return "other"
	}
}

// veeamAgentOnline reports whether a backup agent is running: BackupAgent.status
// is one of Unknown, Active, NotRunning, and only Active is online.
func veeamAgentOnline(status string) bool {
	return strings.TrimSpace(status) == "Active"
}

// veeamSeverityRank orders alarm severities most-to-least urgent for sorting.
func veeamSeverityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "error":
		return 0
	case "warning":
		return 1
	case "info", "information":
		return 2
	case "resolved", "acknowledged":
		return 3
	default:
		return 4
	}
}

// veeamParseWindow parses a duration window like "24h", "3d", "1w" (via the
// loose parser), returning def for an empty input.
func veeamParseWindow(s string, def time.Duration) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	return cliutil.ParseDurationLoose(s)
}

// veeamParseDays parses a day count flag (a plain number of days, e.g. "3", or
// a duration like "36h") into a duration, returning def days when empty.
func veeamParseDays(s string, defDays float64) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Duration(defDays * float64(24*time.Hour)), nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f < 0 {
			return 0, fmt.Errorf("--days must be non-negative")
		}
		return time.Duration(f * float64(24*time.Hour)), nil
	}
	return cliutil.ParseDurationLoose(s)
}

// veeamEmit renders a structured view as JSON (honoring --select/--compact/etc.)
// or, for an interactive terminal, a human table; emptyMsg is shown when the
// table form has no rows.
func veeamEmit(cmd *cobra.Command, flags *rootFlags, view any, table []map[string]any, emptyMsg string) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		if len(table) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), emptyMsg)
			return nil
		}
		return printAutoTable(cmd.OutOrStdout(), table)
	}
	return printJSONFiltered(cmd.OutOrStdout(), view, flags)
}

// veeamHoursSince returns whole hours between t and now (>= 0).
func veeamHoursSince(now, t time.Time) int {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return int(d.Hours())
}
