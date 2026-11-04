// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the local fleet store shared by fleet-sync and the
// cross-tenant commands (triage, gaps, stale-agents, cve-spread, overview,
// leaderboard, geo-audit).
//
// Tenant-scoped CompassOne routes need a per-tenant x-tenant-id header, which
// the generated sync cannot send. fleet-sync fetches them per tenant and
// replaces each (kind, scope) slice wholesale, so a detection resolved since
// the last run disappears instead of lingering as OPEN.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"blackpoint-pp-cli/internal/cliutil"
	"blackpoint-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// Fleet row kinds. scope is the tenant id for tenant-scoped kinds, the
// account id for contact groups, and "" for tenants and accounts.
const (
	fleetKindTenant       = "tenant"
	fleetKindAccount      = "account"
	fleetKindContactGroup = "contact_group"
	fleetKindDetection    = "detection"
	fleetKindDevice       = "device"
	fleetKindVuln         = "vulnerability"
	fleetKindScan         = "scan"
	fleetKindDarkweb      = "darkweb"
	fleetKindPosture      = "posture"
	fleetKindM365Conn     = "ms365_connection"
	fleetKindM365User     = "ms365_user"
	fleetKindOnboarding   = "cloud_onboarding"
	fleetKindConnCountry  = "cloud_connection_country"
	fleetKindConnUser     = "cloud_connection_user"
	// fleetKindCloudStatus holds no rows; its sync state records whether a
	// tenant's whole Cloud Response fetch succeeded.
	fleetKindCloudStatus = "cloud_status"
)

func ensureFleetSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS fleet_rows (
			kind TEXT NOT NULL,
			scope TEXT NOT NULL,
			id TEXT NOT NULL,
			data JSON NOT NULL,
			synced_at TEXT NOT NULL,
			PRIMARY KEY (kind, scope, id)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_sync_state (
			kind TEXT NOT NULL,
			scope TEXT NOT NULL,
			synced_at TEXT NOT NULL,
			row_count INTEGER NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (kind, scope)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating fleet tables: %w", err)
		}
	}
	return nil
}

type fleetRow struct {
	ID   string
	Data json.RawMessage
}

// replaceFleetScope swaps one (kind, scope) slice for rows in a single
// transaction and records a clean sync state for it.
func replaceFleetScope(db *sql.DB, kind, scope string, rows []fleetRow, at time.Time) error {
	return replaceFleetScopes(db, scope, map[string][]fleetRow{kind: rows}, at)
}

// replaceFleetScopes swaps several kinds of one scope atomically, so related
// slices (a tenant's cloud connections, users and countries) never mix runs.
func replaceFleetScopes(db *sql.DB, scope string, slices map[string][]fleetRow, at time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := at.UTC().Format(time.RFC3339)
	for kind, rows := range slices {
		if _, err := tx.Exec(`DELETE FROM fleet_rows WHERE kind = ? AND scope = ?`, kind, scope); err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO fleet_rows (kind, scope, id, data, synced_at) VALUES (?, ?, ?, ?, ?)`,
				kind, scope, r.ID, string(cliutil.RedactSecretFields(r.Data)), stamp); err != nil { // Hand-fix (secret-field-persistence)
				return err
			}
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO fleet_sync_state (kind, scope, synced_at, row_count, error) VALUES (?, ?, ?, ?, '')`,
			kind, scope, stamp, len(rows)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteFleetScope removes one (kind, scope) slice and its sync state.
func deleteFleetScope(db *sql.DB, kind, scope string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM fleet_rows WHERE kind = ? AND scope = ?`, `DELETE FROM fleet_sync_state WHERE kind = ? AND scope = ?`} {
		if _, err := tx.Exec(q, kind, scope); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// recordFleetFailure keeps the previous rows for (kind, scope) and notes why
// this run could not refresh them.
func recordFleetFailure(db *sql.DB, kind, scope string, cause error, at time.Time) error {
	_, err := db.Exec(`INSERT INTO fleet_sync_state (kind, scope, synced_at, row_count, error)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(kind, scope) DO UPDATE SET error = excluded.error`,
		kind, scope, at.UTC().Format(time.RFC3339), cause.Error())
	return err
}

// mergeFleetRows upserts rows into (kind, scope) without deleting others and
// records note as the slice's sync error (an incomplete fetch prunes nothing).
func mergeFleetRows(db *sql.DB, kind, scope string, rows []fleetRow, at time.Time, note string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := at.UTC().Format(time.RFC3339)
	for _, r := range rows {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO fleet_rows (kind, scope, id, data, synced_at) VALUES (?, ?, ?, ?, ?)`,
			kind, scope, r.ID, string(cliutil.RedactSecretFields(r.Data)), stamp); err != nil { // Hand-fix (secret-field-persistence)
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO fleet_sync_state (kind, scope, synced_at, row_count, error) VALUES (?, ?, ?, ?, ?)`,
		kind, scope, stamp, len(rows), note); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteFleetTenantScope removes every fleet slice and sync state of a tenant
// that no longer exists.
func deleteFleetTenantScope(db *sql.DB, tenantID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM fleet_rows WHERE scope = ?`, `DELETE FROM fleet_sync_state WHERE scope = ?`} {
		if _, err := tx.Exec(q, tenantID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type fleetRecord struct {
	Scope    string
	ID       string
	Obj      map[string]any
	SyncedAt string
}

// loadFleetRows reads every row of kind, optionally limited to scopes. Rows
// are drained before returning so callers can run follow-up queries.
func loadFleetRows(db *sql.DB, kind string, scopes ...string) ([]fleetRecord, error) {
	q := `SELECT scope, id, data, synced_at FROM fleet_rows WHERE kind = ?`
	args := []any{kind}
	if len(scopes) > 0 {
		q += ` AND scope IN (` + strings.TrimSuffix(strings.Repeat("?,", len(scopes)), ",") + `)`
		for _, s := range scopes {
			args = append(args, s)
		}
	}
	rows, err := db.Query(q+` ORDER BY scope, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fleetRecord
	for rows.Next() {
		var rec fleetRecord
		var data string
		if err := rows.Scan(&rec.Scope, &rec.ID, &data, &rec.SyncedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &rec.Obj); err != nil || rec.Obj == nil {
			rec.Obj = map[string]any{}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

type fleetSyncState struct {
	Kind     string `json:"kind"`
	Scope    string `json:"scope,omitempty"`
	SyncedAt string `json:"synced_at"`
	Rows     int    `json:"rows"`
	Error    string `json:"error,omitempty"`
}

func loadFleetSyncState(db *sql.DB, scopes ...string) ([]fleetSyncState, error) {
	q := `SELECT kind, scope, synced_at, row_count, error FROM fleet_sync_state`
	var args []any
	if len(scopes) > 0 {
		q += ` WHERE scope IN (` + strings.TrimSuffix(strings.Repeat("?,", len(scopes)), ",") + `)`
		for _, s := range scopes {
			args = append(args, s)
		}
	}
	rows, err := db.Query(q+` ORDER BY kind, scope`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fleetSyncState
	for rows.Next() {
		var s fleetSyncState
		if err := rows.Scan(&s.Kind, &s.Scope, &s.SyncedAt, &s.Rows, &s.Error); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type fleetTenant struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	ContactGroupID string `json:"contact_group_id,omitempty"`
	Domain         string `json:"domain,omitempty"`
}

func loadFleetTenants(db *sql.DB) ([]fleetTenant, error) {
	recs, err := loadFleetRows(db, fleetKindTenant)
	if err != nil {
		return nil, err
	}
	out := make([]fleetTenant, 0, len(recs))
	for _, r := range recs {
		out = append(out, tenantFromObj(r.ID, r.Obj))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func tenantFromObj(id string, obj map[string]any) fleetTenant {
	if v := fieldStr(obj, "id"); v != "" {
		id = v
	}
	return fleetTenant{
		ID:             id,
		Name:           fieldStr(obj, "name"),
		Type:           fieldStr(obj, "type"),
		AccountID:      fieldStr(obj, "accountId"),
		ContactGroupID: fieldStr(obj, "contactGroupId"),
		Domain:         fieldStr(obj, "domain"),
	}
}

// resolveFleetTenant matches query against tenant id, then exact name, then a
// unique case-insensitive name substring.
func resolveFleetTenant(tenants []fleetTenant, query string) (fleetTenant, error) {
	q := strings.TrimSpace(query)
	lq := strings.ToLower(q)
	for _, t := range tenants {
		if t.ID == q {
			return t, nil
		}
	}
	var exact []fleetTenant
	for _, t := range tenants {
		if strings.ToLower(t.Name) == lq {
			exact = append(exact, t)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		ids := make([]string, 0, len(exact))
		for _, t := range exact {
			ids = append(ids, t.ID)
		}
		return fleetTenant{}, fmt.Errorf("%d tenants are named %q (%s); pass the tenant id", len(exact), query, strings.Join(ids, ", "))
	}
	var hits []fleetTenant
	for _, t := range tenants {
		if lq != "" && strings.Contains(strings.ToLower(t.Name), lq) {
			hits = append(hits, t)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return fleetTenant{}, fmt.Errorf("no synced tenant matches %q", query)
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.Name)
	}
	return fleetTenant{}, fmt.Errorf("%q matches %d tenants (%s); pass the tenant id or a more specific name", query, len(hits), strings.Join(names, ", "))
}

// openFleetForRead opens the local store read-only for a cross-tenant
// command. ok=false means there is no fleet data yet; the caller should emit
// an empty result (a missing mirror is an empty state, not an error).
// The generated sync hints point at 'sync', which cannot fetch tenant-scoped
// data, so fleet readers use checkFleetKind for freshness and failures instead.
func openFleetForRead(cmd *cobra.Command, flags *rootFlags, dbPath *string) (db *store.Store, ok bool, err error) {
	if *dbPath == "" {
		*dbPath = defaultDBPath("blackpoint-cli")
	}
	if _, statErr := os.Stat(*dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; run 'blackpoint-cli fleet-sync --db %s' first\n", *dbPath, *dbPath)
		return nil, false, nil
	}
	db, err = store.OpenReadOnlyContext(cmd.Context(), *dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening local store %s: %w", *dbPath, err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'fleet_rows'`).Scan(&n); err != nil || n == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: no fleet data yet. Run 'blackpoint-cli fleet-sync' to pull every tenant into the local store.")
		_ = db.Close() // read-only handle on an empty-state early return; nothing to flush
		return nil, false, nil
	}
	return db, true, nil
}

// fleetKindLabel names a kind in hints and errors, with the --kinds value that
// refreshes it.
var fleetKindLabel = map[string][2]string{
	fleetKindDetection:    {"open detections", "detections"},
	fleetKindDevice:       {"devices", "devices"},
	fleetKindVuln:         {"vulnerability findings", "vulnerabilities"},
	fleetKindScan:         {"scans", "scans"},
	fleetKindDarkweb:      {"dark-web exposures", "darkweb"},
	fleetKindPosture:      {"posture ratings", "posture"},
	fleetKindContactGroup: {"contact groups", "contacts"},
	fleetKindCloudStatus:  {"Cloud Response data", "cloud"},
}

// checkFleetKind reports the last fleet-sync of kind. It returns an error when
// every tenant's fetch failed, so a 403 never reads as an empty result; a
// partial failure is a stderr warning. A kind never synced is a hint, not an
// error (an empty local store is an ordinary state).
//
// scopes limits the check to the requested tenants (from --tenant); a
// requested tenant with no recorded state counts as never synced.
func checkFleetKind(cmd *cobra.Command, flags *rootFlags, db *sql.DB, kind string, scopes ...string) error {
	label := fleetKindLabel[kind]
	states, err := loadFleetSyncState(db)
	if err != nil {
		return err
	}
	explicit := len(scopes) > 0
	if !explicit {
		// Fleet-wide: every inventoried tenant (or account, for contact
		// groups) is expected, so a scope never synced is a coverage gap.
		inv := fleetKindTenant
		if kind == fleetKindContactGroup {
			inv = fleetKindAccount
		}
		recs, err := loadFleetRows(db, inv)
		if err != nil {
			return err
		}
		for _, r := range recs {
			scopes = append(scopes, r.ID)
		}
	}
	want := map[string]bool{}
	for _, s := range scopes {
		want[s] = true
	}
	seen := map[string]bool{}
	var synced, failed, stale int
	sample := ""
	for _, s := range states {
		if s.Kind != kind || (explicit && !want[s.Scope]) {
			continue
		}
		seen[s.Scope] = true
		if t, ok := parseFleetTime(s.SyncedAt); ok && flags.maxAge > 0 && time.Since(t) > flags.maxAge {
			stale++
		}
		if s.Error == "" {
			synced++
			continue
		}
		failed++
		if sample == "" {
			sample = s.Scope + ": " + s.Error
		}
	}
	if !explicit && kind != fleetKindContactGroup {
		if st := fleetSectionStatus(states, fleetKindTenant, ""); st != "ok" {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: the tenant inventory is %s; tenants it missed are absent from these results. Rerun 'blackpoint-cli fleet-sync'.\n", st)
		}
	}
	missing := 0
	for s := range want {
		if !seen[s] {
			missing++
		}
	}
	w := cmd.ErrOrStderr()
	switch {
	case synced == 0 && failed == 0:
		fmt.Fprintf(w, "hint: no %s synced yet. Run 'blackpoint-cli fleet-sync --kinds %s'.\n", label[0], label[1])
		return nil
	case synced == 0:
		return apiErr(fmt.Errorf("no %s: the last fleet-sync failed for all %d requested tenants (e.g. %s)", label[0], failed, sample))
	case failed > 0:
		fmt.Fprintf(w, "warning: %s could not be fully fetched for %d of %d tenants at the last fleet-sync (e.g. %s); results exclude them\n", label[0], failed, synced+failed, sample)
	}
	if missing > 0 {
		fmt.Fprintf(w, "warning: %s were never synced for %d of %d tenant(s); results exclude them. Run 'blackpoint-cli fleet-sync --kinds %s'.\n", label[0], missing, len(want), label[1])
	}
	if stale > 0 {
		fmt.Fprintf(w, "hint: %s for %d tenant(s) are older than --max-age=%s. Run 'blackpoint-cli fleet-sync --kinds %s' to refresh.\n", label[0], stale, flags.maxAge, label[1])
	}
	return nil
}

// cleanFleetScopes returns the scopes whose last sync of kind succeeded and was
// complete. Readers keep only these, so rows retained from a failed or
// incomplete refresh never appear as current results.
func cleanFleetScopes(db *sql.DB, kind string) (map[string]bool, error) {
	states, err := loadFleetSyncState(db)
	if err != nil {
		return nil, err
	}
	clean := map[string]bool{}
	for _, s := range states {
		if s.Kind == kind && s.Error == "" {
			clean[s.Scope] = true
		}
	}
	return clean, nil
}

// loadCleanFleetRows is loadFleetRows limited to scopes whose healthKind sync is
// clean. healthKind is usually kind itself; cloud rows use the cloud status.
func loadCleanFleetRows(db *sql.DB, kind, healthKind string, scopes ...string) ([]fleetRecord, error) {
	clean, err := cleanFleetScopes(db, healthKind)
	if err != nil {
		return nil, err
	}
	recs, err := loadFleetRows(db, kind, scopes...)
	if err != nil {
		return nil, err
	}
	out := recs[:0]
	for _, r := range recs {
		if clean[r.Scope] {
			out = append(out, r)
		}
	}
	return out, nil
}

// fleetSectionStatus reports one tenant's last sync of kind: "ok",
// "not synced", or "failed: <error>".
func fleetSectionStatus(states []fleetSyncState, kind, scope string) string {
	for _, s := range states {
		if s.Kind == kind && s.Scope == scope {
			if s.Error != "" {
				return "failed: " + s.Error
			}
			return "ok"
		}
	}
	return "not synced"
}

// emitRows prints list results: a table for humans, a JSON array otherwise.
// rows must marshal to a JSON array of objects; nil becomes [].
// cols, when given, fixes the human table's columns and their order.
func emitRows(cmd *cobra.Command, flags *rootFlags, rows any, cols ...string) error {
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	if items == nil {
		items = []map[string]any{}
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		if len(items) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no matching rows")
			return nil
		}
		if len(cols) == 0 {
			return printAutoTable(cmd.OutOrStdout(), items)
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.ToUpper(strings.Join(cols, "\t")))
		for _, it := range items {
			vals := make([]string, len(cols))
			for i, c := range cols {
				if v, ok := it[c]; ok && v != nil {
					vals[i] = fmt.Sprint(v)
				}
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		return tw.Flush()
	}
	return printJSONFiltered(cmd.OutOrStdout(), items, flags)
}

// fieldStr returns the first non-empty value among keys, stringified.
func fieldStr(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := fieldAny(obj, k)
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(t)
		}
	}
	return ""
}

// nestedStr reads obj[seg0][seg1]... as a string.
func nestedStr(obj map[string]any, segs ...string) string {
	cur := obj
	for i, seg := range segs {
		if i == len(segs)-1 {
			return fieldStr(cur, seg)
		}
		next, ok := cur[seg].(map[string]any)
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

func fieldAny(obj map[string]any, path string) (any, bool) {
	cur := any(obj)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func fieldNum(obj map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := fieldAny(obj, k)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t, true
		case string:
			if f, err := strconv.ParseFloat(t, 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// fieldBool reads a boolean that some payloads send as a string.
func fieldBool(obj map[string]any, key string) (val, ok bool) {
	v, present := fieldAny(obj, key)
	if !present {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(t))
		return b, err == nil
	}
	return false, false
}

func fieldList(obj map[string]any, key string) []map[string]any {
	v, _ := fieldAny(obj, key)
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func parseFleetTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseFleetDuration accepts Go durations plus d (days) and w (weeks).
func parseFleetDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if strings.HasSuffix(s, suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, suffix), 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid duration %q (use e.g. 24h, 7d, 2w)", s)
			}
			return time.Duration(n * float64(unit)), nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q (use e.g. 24h, 7d, 2w)", s)
	}
	return d, nil
}

// humanAge renders a duration as "3d4h" / "5h" / "12m".
func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// fleetContacts resolves a tenant's escalation contact group. Groups live under
// accounts, so the join is by account and group id; a tenant without a known
// account falls back to a group id that is unique across accounts.
type fleetContacts struct {
	byKey   map[string]map[string]any
	byID    map[string][]map[string]any
	healthy map[string]bool
	synced  bool
	anyFail bool
}

func loadFleetContacts(db *sql.DB, states []fleetSyncState) (*fleetContacts, error) {
	groups, err := loadFleetRows(db, fleetKindContactGroup)
	if err != nil {
		return nil, err
	}
	x := &fleetContacts{byKey: map[string]map[string]any{}, byID: map[string][]map[string]any{}, healthy: map[string]bool{}}
	for _, g := range groups {
		id := fieldStr(g.Obj, "id")
		x.byKey[g.Scope+"/"+id] = g.Obj
		x.byID[id] = append(x.byID[id], g.Obj)
	}
	inventoryOK := false
	for _, s := range states {
		if s.Kind == fleetKindAccount && s.Scope == "" {
			inventoryOK = s.Error == ""
		}
	}
	for _, s := range states {
		if s.Kind != fleetKindContactGroup {
			continue
		}
		x.synced = true
		// Contacts are only current when the account inventory that enumerates
		// them also succeeded.
		x.healthy[s.Scope] = s.Error == "" && inventoryOK
		if s.Error != "" {
			x.anyFail = true
		}
	}
	if !inventoryOK {
		x.anyFail = true
	}
	// The accountless fallback may only trust contacts when every inventoried
	// account has a clean contacts sync.
	accounts, err := loadFleetRows(db, fleetKindAccount)
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		if !x.healthy[a.ID] {
			x.anyFail = true
		}
	}
	return x, nil
}

// lookup returns the tenant's group, whether it was found, and whether the
// contacts fetch that would hold it is current and complete.
func (x *fleetContacts) lookup(t fleetTenant) (map[string]any, bool, bool) {
	if t.AccountID != "" {
		g, found := x.byKey[t.AccountID+"/"+t.ContactGroupID]
		return g, found, x.healthy[t.AccountID]
	}
	healthy := x.synced && !x.anyFail
	if matches := x.byID[t.ContactGroupID]; len(matches) == 1 {
		return matches[0], true, healthy
	}
	return nil, false, healthy
}

// contactMembers returns the group's member count and whether the payload
// actually reported one (members list or totalMembers).
func contactMembers(g map[string]any) (int, bool) {
	list, hasList := g["members"].([]any)
	total, hasTotal := g["totalMembers"].(float64)
	members := len(list)
	if hasTotal && int(total) > members {
		members = int(total)
	}
	return members, hasList || hasTotal
}

// fleetTenantIndex returns tenants by id plus the sorted list.
func fleetTenantIndex(db *sql.DB) (map[string]fleetTenant, []fleetTenant, error) {
	tenants, err := loadFleetTenants(db)
	if err != nil {
		return nil, nil, err
	}
	idx := make(map[string]fleetTenant, len(tenants))
	for _, t := range tenants {
		idx[t.ID] = t
	}
	return idx, tenants, nil
}

// filterTenantScope resolves an optional --tenant flag to a scope list.
func filterTenantScope(tenants []fleetTenant, query string) ([]string, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	t, err := resolveFleetTenant(tenants, query)
	if err != nil {
		return nil, usageErr(err)
	}
	return []string{t.ID}, nil
}

func tenantName(idx map[string]fleetTenant, id string) string {
	if t, ok := idx[id]; ok && t.Name != "" {
		return t.Name
	}
	return id
}

// emitFleetGapHint warns that local results are incomplete when fleet-sync
// recorded a failed or truncated fetch, or when that record cannot be read.
func emitFleetGapHint(w io.Writer, db *store.Store) {
	if w == nil || db == nil {
		return
	}
	gaps, err := db.FleetSyncGaps()
	switch {
	case err != nil:
		fmt.Fprintf(w, "warning: cannot read fleet-sync completeness (%v); treat local results as possibly incomplete.\n", err)
	case gaps > 0:
		fmt.Fprintf(w, "warning: %d fleet-sync slice(s) failed or were truncated; local results are incomplete. Rerun fleet-sync (see its fetch_failures).\n", gaps)
	}
}
