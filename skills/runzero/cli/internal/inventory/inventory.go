// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package inventory is the hand-built local data layer that powers runZero's
// transcendence commands (triage, diff, affected, software rollup, stale,
// exposure-map).
//
// runZero's HTTP API is organized by SCOPE (/org, /account, /export) rather
// than by ENTITY, so the generator's first-path-segment resource grouping
// produces scope tables (org, account, ...) instead of the per-entity tables
// these cross-entity joins need. This package pulls runZero's per-entity
// Export endpoints (/export/org/<entity>.json) into proper local tables —
// assets, services, software, vulnerabilities, certificates, wireless, sites —
// keyed so the live one-query-at-a-time API can be replaced by offline joins.
//
// Every query degrades gracefully on an empty store (returns an empty slice,
// never an error) so the commands are safe to run before any sync and are
// verifiable on a seeded store without a live tenant.
package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Getter is the subset of the generated *client.Client that SyncAll needs.
// Using the generated client (rather than a hand-rolled HTTP client) keeps the
// AdaptiveLimiter / RateLimitError plumbing and avoids a second transport.
type Getter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// Entities is the canonical sync order. Assets are synced first because the
// sub-entities (services, software, vulnerabilities, certificates, wireless)
// join back to them by asset_id.
var Entities = []string{
	"assets", "services", "software", "vulnerabilities",
	"certificates", "wireless", "sites",
}

// clock is the package's one source of "now" (sync run stamps, stale and
// certificate windows). Tests pin it so a result never depends on today's
// date or on how long the test took (runzero handfix inventory-fixed-clock).
var clock = time.Now

// exportPath maps an entity to its runZero Export JSON endpoint.
func exportPath(entity string) string {
	return "/export/org/" + entity + ".json"
}

// EnsureSchema creates every inventory table and index if absent. Idempotent;
// safe to call on every command invocation and from tests against an
// in-memory or temp-file SQLite database.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS inv_sync_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at INTEGER NOT NULL,
			finished_at INTEGER,
			note TEXT
		)`,
		// Which entities each run refreshed: a run can skip one (--only) or
		// fail one (an export refused), and a comparison must not read the
		// skipped entity's rows as removed.
		`CREATE TABLE IF NOT EXISTS inv_sync_entities (
			run_id INTEGER NOT NULL,
			entity TEXT NOT NULL,
			PRIMARY KEY (run_id, entity)
		)`,
		// Which entities each run tried and failed to refresh, and why: a
		// result that reads one must not answer as if it were complete
		// (runzero handfix r1-incomplete-inventory-partial-failure).
		`CREATE TABLE IF NOT EXISTS inv_sync_failures (
			run_id INTEGER NOT NULL,
			entity TEXT NOT NULL,
			error TEXT,
			PRIMARY KEY (run_id, entity)
		)`,
		// Each row's presence, as unbroken streaks of the refreshes of its
		// entity that returned it: a row absent from one refresh and back in
		// the next is two streaks, so a comparison reads presence at exactly
		// the two runs it compares (runzero handfix r1-presence-per-refresh).
		`CREATE TABLE IF NOT EXISTS inv_presence (
			entity TEXT NOT NULL,
			id TEXT NOT NULL,
			first_run INTEGER NOT NULL,
			last_run INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_presence_row ON inv_presence(entity, id, last_run)`,
		// Which refreshes recorded presence: a run an older build wrote has
		// inv_sync_entities rows and no presence history, and a refresh that
		// returned no rows records presence with no inv_presence rows, so the
		// history is declared per run rather than read from the rows (runzero
		// handfix r2-presence-history-per-run).
		`CREATE TABLE IF NOT EXISTS inv_presence_runs (
			run_id INTEGER NOT NULL,
			entity TEXT NOT NULL,
			PRIMARY KEY (run_id, entity)
		)`,
		`CREATE TABLE IF NOT EXISTS inv_assets (
			id TEXT PRIMARY KEY,
			name TEXT, address TEXT, addresses TEXT,
			os TEXT, os_vendor TEXT, os_eol INTEGER,
			type TEXT, hardware TEXT, mac_vendors TEXT,
			criticality TEXT, criticality_rank INTEGER DEFAULT 0,
			risk REAL DEFAULT 0,
			alive INTEGER DEFAULT 0, external INTEGER DEFAULT 0,
			tags TEXT, owners TEXT,
			site_id TEXT, site_name TEXT, org_id TEXT,
			first_seen INTEGER DEFAULT 0, last_seen INTEGER DEFAULT 0,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_last_run ON inv_assets(last_run)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_crit ON inv_assets(criticality_rank)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_last_seen ON inv_assets(last_seen)`,
		`CREATE TABLE IF NOT EXISTS inv_services (
			id TEXT PRIMARY KEY,
			asset_id TEXT, address TEXT,
			transport TEXT, port INTEGER DEFAULT 0,
			protocol TEXT, product TEXT,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_services_asset ON inv_services(asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_services_last_run ON inv_services(last_run)`,
		`CREATE TABLE IF NOT EXISTS inv_software (
			id TEXT PRIMARY KEY,
			asset_id TEXT, vendor TEXT, product TEXT, version TEXT,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_software_asset ON inv_software(asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_product ON inv_software(product)`,
		`CREATE INDEX IF NOT EXISTS idx_software_last_run ON inv_software(last_run)`,
		`CREATE TABLE IF NOT EXISTS inv_vulnerabilities (
			id TEXT PRIMARY KEY,
			asset_id TEXT, name TEXT, cve TEXT,
			severity TEXT, severity_rank INTEGER DEFAULT 0, cvss REAL DEFAULT 0,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_asset ON inv_vulnerabilities(asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_cve ON inv_vulnerabilities(cve)`,
		`CREATE TABLE IF NOT EXISTS inv_certificates (
			id TEXT PRIMARY KEY,
			asset_id TEXT, subject TEXT, issuer TEXT,
			not_after INTEGER DEFAULT 0, self_signed INTEGER DEFAULT 0,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_certs_asset ON inv_certificates(asset_id)`,
		`CREATE TABLE IF NOT EXISTS inv_wireless (
			id TEXT PRIMARY KEY,
			asset_id TEXT, bssid TEXT, essid TEXT, encryption TEXT,
			last_seen INTEGER DEFAULT 0,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS inv_sites (
			id TEXT PRIMARY KEY,
			name TEXT, description TEXT, asset_count INTEGER DEFAULT 0,
			first_run INTEGER NOT NULL, last_run INTEGER NOT NULL,
			data TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("inventory schema: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tolerant JSON field extraction (handles json.Number from UseNumber decoders).
// ---------------------------------------------------------------------------

func decodeObjects(raw json.RawMessage) ([]map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	// Top-level array of objects (the runZero export shape).
	if arr, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out, nil
	}
	// Object wrapper: find the first array-of-objects field.
	if m, ok := v.(map[string]any); ok {
		for _, key := range []string{"data", "assets", "services", "results", "items"} {
			if arr, ok := m[key].([]any); ok {
				out := make([]map[string]any, 0, len(arr))
				for _, e := range arr {
					if mm, ok := e.(map[string]any); ok {
						out = append(out, mm)
					}
				}
				return out, nil
			}
		}
		// Single object.
		return []map[string]any{m}, nil
	}
	return nil, nil
}

// firstStr returns the first non-empty string among the candidate keys.
func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch t := m[k].(type) {
		case string:
			if t != "" {
				return t
			}
		case json.Number:
			return t.String()
		case bool:
			return strconv.FormatBool(t)
		}
	}
	return ""
}

// firstNum returns the first numeric value (handles json.Number and numeric strings).
func firstNum(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch t := m[k].(type) {
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return f
			}
		case float64:
			return t
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func firstInt(m map[string]any, keys ...string) int64 {
	return int64(firstNum(m, keys...))
}

func firstBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		switch t := m[k].(type) {
		case bool:
			return t
		case string:
			s := strings.ToLower(strings.TrimSpace(t))
			if s == "t" || s == "true" || s == "yes" || s == "1" {
				return true
			}
		case json.Number:
			if t.String() != "0" && t.String() != "" {
				return true
			}
		}
	}
	return false
}

// strSlice extracts a []string from an array-or-scalar field, joined for storage.
func strSlice(m map[string]any, keys ...string) []string {
	for _, k := range keys {
		switch t := m[k].(type) {
		case []any:
			out := make([]string, 0, len(t))
			for _, e := range t {
				switch ev := e.(type) {
				case string:
					if ev != "" {
						out = append(out, ev)
					}
				case json.Number:
					out = append(out, ev.String())
				case map[string]any:
					// e.g. addresses as [{addr:"1.2.3.4"}]
					if s := firstStr(ev, "addr", "address", "ip", "value", "name"); s != "" {
						out = append(out, s)
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if t != "" {
				return []string{t}
			}
		}
	}
	return nil
}

func rawJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// criticalityRank maps runZero criticality (string or integer) to a 0-4 rank.
func criticalityRank(m map[string]any) (string, int) {
	raw := firstStr(m, "criticality", "asset_criticality", "criticality_rank")
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "critical", "mission_critical", "mission-critical", "5", "4":
		return raw, 4
	case "high", "3":
		return raw, 3
	case "medium", "moderate", "2":
		return raw, 2
	case "low", "1":
		return raw, 1
	case "", "none", "unknown", "0":
		return raw, 0
	}
	// numeric-but-large fallthrough
	if n := firstInt(m, "criticality"); n > 0 {
		if n > 4 {
			n = 4
		}
		return raw, int(n)
	}
	return raw, 0
}

func severityRank(m map[string]any, keys ...string) (string, int) {
	raw := firstStr(m, keys...)
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "critical", "4":
		return raw, 4
	case "high", "3":
		return raw, 3
	case "medium", "moderate", "2":
		return raw, 2
	case "low", "1":
		return raw, 1
	case "info", "informational", "none", "", "0":
		return raw, 0
	}
	return raw, 0
}

// ---------------------------------------------------------------------------
// Sync
// ---------------------------------------------------------------------------

// SyncResult reports per-entity row counts for a sync run.
type SyncResult struct {
	RunID  int64             `json:"run_id"`
	Counts map[string]int    `json:"counts"`
	Errors map[string]string `json:"errors,omitempty"`
}

// SyncAll pulls each requested entity from the Export API and upserts it into
// the local tables under a new sync run. `only` restricts the entity set (nil =
// all). `search` is passed through as the runZero `search` query param.
func SyncAll(ctx context.Context, g Getter, db *sql.DB, only []string, search string) (*SyncResult, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, e := range only {
		want[strings.ToLower(strings.TrimSpace(e))] = true
	}
	var requested []string
	for _, entity := range Entities {
		if len(want) == 0 || want[entity] {
			requested = append(requested, entity)
		}
	}
	runID, err := openRun(ctx, db, search, requested)
	if err != nil {
		return nil, fmt.Errorf("open sync run: %w", err)
	}
	out := &SyncResult{RunID: runID, Counts: map[string]int{}, Errors: map[string]string{}}
	params := map[string]string{}
	if search != "" {
		params["search"] = search
	}

	// A refresh that fails is recorded with its error, so every later
	// result that reads the entity names it (Coverage).
	fail := func(entity, msg string) {
		out.Errors[entity] = msg
		_, _ = db.ExecContext(ctx, `INSERT OR REPLACE INTO inv_sync_failures(run_id, entity, error) VALUES(?, ?, ?)`, runID, entity, msg)
	}
	for _, entity := range requested {
		raw, err := g.Get(ctx, exportPath(entity), params)
		if err != nil {
			fail(entity, err.Error())
			continue
		}
		objs, err := decodeObjects(raw)
		if err != nil {
			fail(entity, "decode: "+err.Error())
			continue
		}
		n, err := upsertEntity(ctx, db, entity, objs, runID)
		if err != nil {
			fail(entity, err.Error())
			continue
		}
		out.Counts[entity] = n
	}
	_, _ = db.ExecContext(ctx, `UPDATE inv_sync_runs SET finished_at=? WHERE id=?`, clock().Unix(), runID)
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out, nil
}

// unfinishedAttempt is the failure a sync records for each entity it will
// refresh before fetching any export; the entity's refresh clears it, a
// failed export replaces it with the error (runzero handfix
// r2-unfinished-attempt-is-incomplete).
const unfinishedAttempt = "did not finish: the sync stopped before this export was stored, or is still running"

// openRun records a sync run and, in the same committed write and before any
// export is fetched, a provisional failure (unfinishedAttempt) for each entity
// it will refresh: a sync killed or crashed before an export returned leaves
// that entity named by Coverage instead of its previous snapshot reading as
// complete (runzero handfix r2-unfinished-attempt-is-incomplete).
func openRun(ctx context.Context, db *sql.DB, search string, entities []string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO inv_sync_runs(started_at, note) VALUES(?, ?)`, clock().Unix(), search)
	if err != nil {
		return 0, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, entity := range entities {
		if _, err := tx.ExecContext(ctx, `INSERT INTO inv_sync_failures(run_id, entity, error) VALUES(?, ?, ?)`, runID, entity, unfinishedAttempt); err != nil {
			return 0, err
		}
	}
	return runID, tx.Commit()
}

func upsertEntity(ctx context.Context, db *sql.DB, entity string, objs []map[string]any, runID int64) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// The entity's previous refresh: a row it returned continues its
	// presence streak; any other row starts a new one.
	var prev int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_id), 0) FROM inv_sync_entities WHERE entity = ? AND run_id < ?`, entity, runID).Scan(&prev); err != nil {
		return 0, err
	}
	count := 0
	for i, m := range objs {
		id, err := upsertRow(ctx, tx, entity, m, runID, i)
		if err != nil {
			return count, err
		}
		if err := recordPresence(ctx, tx, entity, id, prev, runID); err != nil {
			return count, err
		}
		count++
	}
	// The run refreshed this entity only if every row landed (same transaction).
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inv_sync_entities(run_id, entity) VALUES(?, ?)`, runID, entity); err != nil {
		return count, err
	}
	// It wrote presence for every row it returned (same transaction).
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inv_presence_runs(run_id, entity) VALUES(?, ?)`, runID, entity); err != nil {
		return count, err
	}
	// Its provisional failure (openRun) goes with the refresh it stood for.
	if _, err := tx.ExecContext(ctx, `DELETE FROM inv_sync_failures WHERE run_id = ? AND entity = ?`, runID, entity); err != nil {
		return count, err
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

// recordPresence extends the row's streak that reached the entity's previous
// refresh (prev) to this run, or starts a streak at this run.
func recordPresence(ctx context.Context, tx *sql.Tx, entity, id string, prev, runID int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE inv_presence SET last_run = ? WHERE entity = ? AND id = ? AND (last_run = ? OR last_run = ?)`,
		runID, entity, id, prev, runID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inv_presence(entity, id, first_run, last_run) VALUES(?, ?, ?, ?)`, entity, id, runID, runID)
	return err
}

// rowKeys resolves a sub-entity row's own key and the asset it belongs to.
// runZero's export rows name both with the entity's prefix (vendor OpenAPI:
// Service.service_id/service_asset_id, Software.software_id/software_asset_id,
// Vulnerability.vulnerability_id/vulnerability_asset_id). Service and
// vulnerability rows also embed their asset, so their `id` is the ASSET's id:
// keying on it kept one row per asset, and reading `asset_id` (absent) linked
// none of them (runzero handfix inventory-vendor-row-fields). The unprefixed
// names stay as fallbacks for other shapes.
func rowKeys(m map[string]any, prefix string, embedsAsset bool) (id, assetID string) {
	own := firstStr(m, prefix+"_id")
	assetID = firstStr(m, prefix+"_asset_id", "asset_id")
	if own == "" {
		return firstStr(m, "id"), assetID
	}
	if assetID == "" && embedsAsset {
		assetID = firstStr(m, "id")
	}
	return own, assetID
}

// upsertRow inserts or updates one entity row, preserving first_run and
// advancing last_run to the current run id; it returns the row's key.
func upsertRow(ctx context.Context, tx *sql.Tx, entity string, m map[string]any, runID int64, idx int) (string, error) {
	switch entity {
	case "assets":
		id := firstStr(m, "id", "asset_id", "uuid")
		if id == "" {
			id = fmt.Sprintf("asset-%d-%d", runID, idx)
		}
		crit, critRank := criticalityRank(m)
		addrs := strSlice(m, "addresses", "addresses_extra", "ip_addresses")
		names := strSlice(m, "names", "hostnames", "name")
		addr := ""
		if len(addrs) > 0 {
			addr = addrs[0]
		}
		name := ""
		if len(names) > 0 {
			name = names[0]
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_assets(id,name,address,addresses,os,os_vendor,os_eol,type,hardware,mac_vendors,
				criticality,criticality_rank,risk,alive,external,tags,owners,site_id,site_name,org_id,
				first_seen,last_seen,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				name=excluded.name,address=excluded.address,addresses=excluded.addresses,os=excluded.os,
				os_vendor=excluded.os_vendor,os_eol=excluded.os_eol,type=excluded.type,hardware=excluded.hardware,
				mac_vendors=excluded.mac_vendors,criticality=excluded.criticality,criticality_rank=excluded.criticality_rank,
				risk=excluded.risk,alive=excluded.alive,external=excluded.external,tags=excluded.tags,owners=excluded.owners,
				site_id=excluded.site_id,site_name=excluded.site_name,org_id=excluded.org_id,
				first_seen=excluded.first_seen,last_seen=excluded.last_seen,last_run=excluded.last_run,data=excluded.data`,
			id, name, addr, rawJSON(addrs), firstStr(m, "os", "operating_system"), firstStr(m, "os_vendor"),
			firstInt(m, "eol_os", "os_eol", "eol_date"), firstStr(m, "type", "device_type"), firstStr(m, "hardware", "hw"),
			rawJSON(strSlice(m, "mac_vendors", "macs")), crit, critRank, firstNum(m, "risk", "risk_score"),
			boolToInt(firstBool(m, "alive")), boolToInt(firstBool(m, "external", "outernet", "public")),
			rawJSON(m["tags"]), rawJSON(m["owners"]), firstStr(m, "site_id"), firstStr(m, "site_name", "site"),
			firstStr(m, "org_id", "organization_id"), firstInt(m, "first_seen"), firstInt(m, "last_seen", "updated_at"),
			runID, runID, rawJSON(m))
		return id, err
	case "services":
		id, assetID := rowKeys(m, "service", true)
		if id == "" {
			id = fmt.Sprintf("%s-%s-%d", assetID, firstStr(m, "service_port", "port"), idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_services(id,asset_id,address,transport,port,protocol,product,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,address=excluded.address,transport=excluded.transport,
				port=excluded.port,protocol=excluded.protocol,product=excluded.product,last_run=excluded.last_run,data=excluded.data`,
			id, assetID, firstStr(m, "service_address", "address"),
			strings.ToLower(firstStr(m, "service_transport", "transport")), firstInt(m, "service_port", "port"),
			firstStr(m, "service_protocol", "protocol", "protocols"), firstStr(m, "service_product", "product"),
			runID, runID, rawJSON(m))
		return id, err
	case "software":
		id, assetID := rowKeys(m, "software", false)
		if id == "" {
			id = fmt.Sprintf("%s-%s-%s-%d", assetID, firstStr(m, "software_product", "product"), firstStr(m, "software_version", "version"), idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_software(id,asset_id,vendor,product,version,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,vendor=excluded.vendor,product=excluded.product,
				version=excluded.version,last_run=excluded.last_run,data=excluded.data`,
			id, assetID, firstStr(m, "software_vendor", "vendor"),
			firstStr(m, "software_product", "product", "name"), firstStr(m, "software_version", "version"),
			runID, runID, rawJSON(m))
		return id, err
	case "vulnerabilities":
		id, assetID := rowKeys(m, "vulnerability", true)
		if id == "" {
			id = firstStr(m, "finding_id")
		}
		cve := firstStr(m, "vulnerability_cve", "cve")
		if id == "" {
			id = fmt.Sprintf("%s-%s-%d", assetID, cve, idx)
		}
		// A vendor row embeds its asset, whose `risk` is the asset's, not the
		// finding's: the finding's own severity fields are read first.
		sev, sevRank := severityRank(m, "vulnerability_severity", "vulnerability_risk", "severity", "risk", "risk_rank", "rank")
		if cve == "" {
			if cves := strSlice(m, "cves"); len(cves) > 0 {
				cve = strings.Join(cves, ",")
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_vulnerabilities(id,asset_id,name,cve,severity,severity_rank,cvss,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,name=excluded.name,cve=excluded.cve,
				severity=excluded.severity,severity_rank=excluded.severity_rank,cvss=excluded.cvss,
				last_run=excluded.last_run,data=excluded.data`,
			id, assetID, firstStr(m, "vulnerability_name", "name", "title"), cve, sev, sevRank,
			firstNum(m, "vulnerability_cvss3_base_score", "vulnerability_cvss2_base_score", "cvss", "cvss_score", "cvss3"), runID, runID, rawJSON(m))
		return id, err
	case "certificates":
		id := firstStr(m, "id", "certificate_id", "fingerprint")
		if id == "" {
			id = fmt.Sprintf("cert-%d-%d", runID, idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_certificates(id,asset_id,subject,issuer,not_after,self_signed,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,subject=excluded.subject,issuer=excluded.issuer,
				not_after=excluded.not_after,self_signed=excluded.self_signed,last_run=excluded.last_run,data=excluded.data`,
			id, firstStr(m, "asset_id"), firstStr(m, "subject", "subject_cn", "cn"), firstStr(m, "issuer", "issuer_cn"),
			firstInt(m, "validity_end", "not_after", "expiration", "valid_to"), boolToInt(firstBool(m, "self_signed", "selfsigned")),
			runID, runID, rawJSON(m))
		return id, err
	case "wireless":
		id := firstStr(m, "id", "wireless_id", "bssid")
		if id == "" {
			id = fmt.Sprintf("wifi-%d-%d", runID, idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_wireless(id,asset_id,bssid,essid,encryption,last_seen,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,bssid=excluded.bssid,essid=excluded.essid,
				encryption=excluded.encryption,last_seen=excluded.last_seen,last_run=excluded.last_run,data=excluded.data`,
			id, firstStr(m, "asset_id"), firstStr(m, "bssid"), firstStr(m, "essid", "ssid"),
			firstStr(m, "encryption", "auth"), firstInt(m, "last_seen"), runID, runID, rawJSON(m))
		return id, err
	case "sites":
		id := firstStr(m, "id", "site_id")
		if id == "" {
			id = fmt.Sprintf("site-%d-%d", runID, idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_sites(id,name,description,asset_count,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,
				asset_count=excluded.asset_count,last_run=excluded.last_run,data=excluded.data`,
			id, firstStr(m, "name"), firstStr(m, "description"), firstInt(m, "asset_count", "assets"),
			runID, runID, rawJSON(m))
		return id, err
	}
	return "", fmt.Errorf("unknown inventory entity %q", entity)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Queries (all degrade to empty results on an empty store)
// ---------------------------------------------------------------------------

func latestTwoRuns(ctx context.Context, db *sql.DB) (latest, baseline int64, ok bool) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM inv_sync_runs ORDER BY id DESC LIMIT 2`)
	if err != nil {
		return 0, 0, false
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, 0, false
	}
	latest = ids[0]
	if len(ids) > 1 {
		baseline = ids[1]
	} else {
		baseline = ids[0] - 1 // no prior run; baseline before the first
	}
	return latest, baseline, true
}

// baselineForSince resolves the baseline run id as the most recent run started
// at or before (now - since). Falls back to the second-most-recent run.
func baselineForSince(ctx context.Context, db *sql.DB, since time.Duration) (int64, bool) {
	if since <= 0 {
		_, b, ok := latestTwoRuns(ctx, db)
		return b, ok
	}
	cutoff := clock().Add(-since).Unix()
	var id sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM inv_sync_runs WHERE started_at <= ? ORDER BY id DESC LIMIT 1`, cutoff).Scan(&id)
	if err != nil || !id.Valid {
		_, b, ok := latestTwoRuns(ctx, db)
		return b, ok
	}
	// A window older than the most recent sync resolves to the latest run
	// itself; comparing a run against itself yields a silently-empty diff.
	// Fall back to the prior run so the comparison stays meaningful.
	if latest, b, ok := latestTwoRuns(ctx, db); ok && id.Int64 >= latest {
		return b, ok
	}
	return id.Int64, true
}

// currentRun is the run whose rows are entity's current snapshot (the rows
// whose last_run equals it): the latest run that refreshed the entity
// (inv_sync_entities). A row runZero stopped returning keeps its older
// last_run, and a refresh that returned no rows advances none, so neither
// the table's newest last_run nor "every stored row" is the current snapshot
// (runzero handfix r1-current-snapshot-per-entity). An entity no run recorded
// a refresh of (a store an older build wrote) keeps its newest stored rows; 0
// when it has none.
func currentRun(ctx context.Context, db *sql.DB, entity string) int64 {
	var run sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(run_id) FROM inv_sync_entities WHERE entity = ?`, entity).Scan(&run); err == nil && run.Valid {
		return run.Int64
	}
	q, ok := newestStoredRunQuery[entity]
	if !ok {
		return 0
	}
	if err := db.QueryRowContext(ctx, q).Scan(&run); err == nil && run.Valid {
		return run.Int64
	}
	return 0
}

// The per-entity queries, written out (SQLite cannot bind a table name).
var (
	newestStoredRunQuery = map[string]string{
		"assets": `SELECT MAX(last_run) FROM inv_assets`, "services": `SELECT MAX(last_run) FROM inv_services`,
		"software": `SELECT MAX(last_run) FROM inv_software`, "vulnerabilities": `SELECT MAX(last_run) FROM inv_vulnerabilities`,
		"certificates": `SELECT MAX(last_run) FROM inv_certificates`, "wireless": `SELECT MAX(last_run) FROM inv_wireless`,
		"sites": `SELECT MAX(last_run) FROM inv_sites`,
	}
	currentRowsQuery = map[string]string{
		"assets": `SELECT COUNT(*) FROM inv_assets WHERE last_run = ?`, "services": `SELECT COUNT(*) FROM inv_services WHERE last_run = ?`,
		"software": `SELECT COUNT(*) FROM inv_software WHERE last_run = ?`, "vulnerabilities": `SELECT COUNT(*) FROM inv_vulnerabilities WHERE last_run = ?`,
		"certificates": `SELECT COUNT(*) FROM inv_certificates WHERE last_run = ?`, "wireless": `SELECT COUNT(*) FROM inv_wireless WHERE last_run = ?`,
		"sites": `SELECT COUNT(*) FROM inv_sites WHERE last_run = ?`,
	}
	storedRowsQuery = map[string]string{
		"assets": `SELECT COUNT(*) FROM inv_assets`, "services": `SELECT COUNT(*) FROM inv_services`,
		"software": `SELECT COUNT(*) FROM inv_software`, "vulnerabilities": `SELECT COUNT(*) FROM inv_vulnerabilities`,
		"certificates": `SELECT COUNT(*) FROM inv_certificates`, "wireless": `SELECT COUNT(*) FROM inv_wireless`,
		"sites": `SELECT COUNT(*) FROM inv_sites`,
	}
	// assetlessRowsQuery lists, for each entity whose rows link to an asset,
	// its current rows (bind: the entity's current run) whose asset the
	// current asset snapshot (bind: the asset run) does not hold.
	assetlessRowsQuery = map[string]string{
		"services": `SELECT c.id, c.asset_id FROM inv_services c WHERE c.last_run = ? AND COALESCE(c.asset_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM inv_assets a WHERE a.id = c.asset_id AND a.last_run = ?) ORDER BY c.id`,
		"software": `SELECT c.id, c.asset_id FROM inv_software c WHERE c.last_run = ? AND COALESCE(c.asset_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM inv_assets a WHERE a.id = c.asset_id AND a.last_run = ?) ORDER BY c.id`,
		"vulnerabilities": `SELECT c.id, c.asset_id FROM inv_vulnerabilities c WHERE c.last_run = ? AND COALESCE(c.asset_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM inv_assets a WHERE a.id = c.asset_id AND a.last_run = ?) ORDER BY c.id`,
		"certificates": `SELECT c.id, c.asset_id FROM inv_certificates c WHERE c.last_run = ? AND COALESCE(c.asset_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM inv_assets a WHERE a.id = c.asset_id AND a.last_run = ?) ORDER BY c.id`,
		"wireless": `SELECT c.id, c.asset_id FROM inv_wireless c WHERE c.last_run = ? AND COALESCE(c.asset_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM inv_assets a WHERE a.id = c.asset_id AND a.last_run = ?) ORDER BY c.id`,
	}
)

// assetlessRows names entity's current rows whose asset the current asset
// snapshot (run assetRun) does not hold: the asset was deleted, or the two
// entities were refreshed at different times, so a result joining them shows
// the row without (or, before runzero handfix r2-current-asset-snapshot-joins,
// with a stale copy of) its asset. "" when there are none or entity has no
// asset link. A query that fails is reported, not read as none.
func assetlessRows(ctx context.Context, db *sql.DB, entity string, assetRun int64) string {
	q, ok := assetlessRowsQuery[entity]
	if !ok {
		return ""
	}
	rows, err := db.QueryContext(ctx, q, currentRun(ctx, db, entity), assetRun)
	if err != nil {
		return fmt.Sprintf("could not check which assets the current %s belong to: %v", entity, err)
	}
	defer rows.Close()
	const shown = 5
	var named []string
	n := 0
	for rows.Next() {
		var id, assetID string
		if err := rows.Scan(&id, &assetID); err != nil {
			return fmt.Sprintf("could not check which assets the current %s belong to: %v", entity, err)
		}
		n++
		if len(named) < shown {
			named = append(named, id+" (asset "+assetID+")")
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Sprintf("could not check which assets the current %s belong to: %v", entity, err)
	}
	if n == 0 {
		return ""
	}
	list := strings.Join(named, ", ")
	if n > len(named) {
		list += fmt.Sprintf(" and %d more", n-len(named))
	}
	return fmt.Sprintf("%s whose asset is not in the current asset snapshot (run %d): %s", entity, assetRun, list)
}

// runScope is the search filter a run synced under ("" when none).
func runScope(ctx context.Context, db *sql.DB, run int64) string {
	var note sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT note FROM inv_sync_runs WHERE id = ?`, run).Scan(&note)
	return strings.TrimSpace(note.String)
}

// Coverage names, for each entity a result reads, why the local store does not
// hold it completely (runzero handfix r1-incomplete-inventory-partial-failure
// and r1-compare-same-scope): no inventory sync has run; the entity was never
// synced; its latest sync failed (runZero refused the export: the spec's
// documented 403, a timeout), so the store holds an older snapshot; or the
// entities were synced under different --search filters, so a join reads two
// different slices of the surface. A sync that did not finish counts as a
// failed sync of every entity it was asked to refresh and did not (openRun);
// an unfinished run with nothing recorded for it (a store an older build
// wrote, which never recorded what a run was asked to refresh) counts for every
// entity (runzero handfix r2-unfinished-attempt-is-incomplete). A result that
// reads assets with an entity
// whose rows link to an asset also names the entity's current rows whose asset
// the current asset snapshot does not hold (runzero handfix
// r2-current-asset-snapshot-joins): the two snapshots disagree, so the result
// shows those rows without their asset. Skipping an entity with --only is not
// a failure: the store keeps that entity's last snapshot. Nil when complete.
func Coverage(ctx context.Context, db *sql.DB, entities ...string) []string {
	var runs int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_sync_runs`).Scan(&runs); err != nil || runs == 0 {
		return []string{"no inventory sync has run"}
	}
	var problems []string
	scopes := map[string][]string{}
	var order []string
	for _, e := range entities {
		cur := currentRun(ctx, db, e)
		var failRun sql.NullInt64
		var failErr sql.NullString
		_ = db.QueryRowContext(ctx, `SELECT run_id, error FROM (
				SELECT run_id, error FROM inv_sync_failures WHERE entity = ?
				UNION ALL
				SELECT r.id, ? FROM inv_sync_runs r WHERE r.finished_at IS NULL
					AND NOT EXISTS (SELECT 1 FROM inv_sync_failures f WHERE f.run_id = r.id)
					AND NOT EXISTS (SELECT 1 FROM inv_sync_entities x WHERE x.run_id = r.id)
			) ORDER BY run_id DESC LIMIT 1`, e, unfinishedAttempt).Scan(&failRun, &failErr)
		switch {
		case failRun.Valid && failRun.Int64 > cur && cur == 0:
			problems = append(problems, fmt.Sprintf("%s were never synced (run %d failed: %s)", e, failRun.Int64, failErr.String))
		case failRun.Valid && failRun.Int64 > cur:
			problems = append(problems, fmt.Sprintf("the latest sync of %s failed (run %d: %s); the store holds run %d's", e, failRun.Int64, failErr.String, cur))
		case cur == 0:
			problems = append(problems, e+" were never synced")
		}
		if cur > 0 {
			sc := runScope(ctx, db, cur)
			if _, seen := scopes[sc]; !seen {
				order = append(order, sc)
			}
			scopes[sc] = append(scopes[sc], e)
		}
	}
	readsAssets := false
	for _, e := range entities {
		readsAssets = readsAssets || e == "assets"
	}
	if readsAssets {
		// Assets never synced are named above; with none there is no
		// snapshot to check the links against.
		if ra := currentRun(ctx, db, "assets"); ra > 0 {
			for _, e := range entities {
				if p := assetlessRows(ctx, db, e, ra); p != "" {
					problems = append(problems, p)
				}
			}
		}
	}
	if len(scopes) > 1 {
		parts := make([]string, 0, len(order))
		for _, sc := range order {
			label := "no --search filter"
			if sc != "" {
				label = "--search " + strconv.Quote(sc)
			}
			parts = append(parts, strings.Join(scopes[sc], ", ")+" under "+label)
		}
		problems = append(problems, "synced under different filters: "+strings.Join(parts, "; "))
	}
	return problems
}

// FilteredScope is the --search filter the entities' current snapshots were
// all synced under, "" when there is none (or they differ: Coverage names
// that). A result over a filtered snapshot covers only what the filter
// matched, and says so (runzero handfix r1-compare-same-scope).
func FilteredScope(ctx context.Context, db *sql.DB, entities ...string) string {
	scope := ""
	for i, e := range entities {
		cur := currentRun(ctx, db, e)
		if cur == 0 {
			return ""
		}
		sc := runScope(ctx, db, cur)
		if i > 0 && sc != scope {
			return ""
		}
		scope = sc
	}
	return scope
}

// TriageRow is one ranked exposed-and-vulnerable asset.
type TriageRow struct {
	AssetID      string `json:"asset_id"`
	Name         string `json:"name"`
	Address      string `json:"address"`
	Criticality  string `json:"criticality"`
	OS           string `json:"os"`
	VulnCount    int    `json:"vuln_count"`
	MaxSeverity  string `json:"max_severity"`
	ServiceCount int    `json:"service_count"`
	TopCVE       string `json:"top_cve"`
}

// Triage ranks assets by criticality, then by max vulnerability severity, then
// by vulnerability count. internetFacing restricts to externally-exposed assets.
// Each entity is read at its current snapshot (currentRun).
func Triage(ctx context.Context, db *sql.DB, internetFacing bool) ([]TriageRow, error) {
	ra, rs, rv := currentRun(ctx, db, "assets"), currentRun(ctx, db, "services"), currentRun(ctx, db, "vulnerabilities")
	where := "WHERE a.last_run = ?"
	if internetFacing {
		where += " AND a.external = 1"
	}
	q := `
		SELECT a.id, COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,''), COALESCE(a.os,''),
			(SELECT COUNT(*) FROM inv_vulnerabilities v WHERE v.asset_id = a.id AND v.last_run = ?) AS vuln_count,
			COALESCE((SELECT MAX(v.severity_rank) FROM inv_vulnerabilities v WHERE v.asset_id = a.id AND v.last_run = ?), 0) AS max_sev,
			(SELECT COUNT(*) FROM inv_services s WHERE s.asset_id = a.id AND s.last_run = ?) AS svc_count,
			COALESCE((SELECT v.cve FROM inv_vulnerabilities v WHERE v.asset_id = a.id AND v.last_run = ? ORDER BY v.severity_rank DESC, v.cvss DESC LIMIT 1), '') AS top_cve
		FROM inv_assets a ` + where + `
		ORDER BY a.criticality_rank DESC, max_sev DESC, vuln_count DESC, a.risk DESC`
	rows, err := db.QueryContext(ctx, q, rv, rv, rs, rv, ra)
	if err != nil {
		return []TriageRow{}, nil // empty/absent store: degrade gracefully
	}
	defer rows.Close()
	out := []TriageRow{}
	for rows.Next() {
		var r TriageRow
		var maxSev int
		if err := rows.Scan(&r.AssetID, &r.Name, &r.Address, &r.Criticality, &r.OS, &r.VulnCount, &maxSev, &r.ServiceCount, &r.TopCVE); err != nil {
			continue
		}
		r.MaxSeverity = rankLabel(maxSev)
		out = append(out, r)
	}
	return out, nil
}

func rankLabel(r int) string {
	switch r {
	case 4:
		return "critical"
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	}
	return "none"
}

// DiffResult is the attack-surface delta between two sync runs.
type DiffResult struct {
	BaselineRun int64       `json:"baseline_run"`
	LatestRun   int64       `json:"latest_run"`
	Assets      DiffSection `json:"assets"`
	Services    DiffSection `json:"services"`
	Software    DiffSection `json:"software"`
	// StaleEntities names the entities the latest run did not refresh
	// (skipped with --only, or their export failed); their sections compare
	// the runs that did refresh them.
	StaleEntities []string `json:"stale_entities,omitempty"`
	// NoBaselineEntities names the entities whose latest refresh has no
	// earlier refresh under the same --search filter to compare with; their
	// sections are empty (runzero handfix r1-compare-same-scope).
	NoBaselineEntities []string `json:"no_baseline_entities,omitempty"`
}

// DiffSection lists what was added and removed for one entity.
type DiffSection struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// Diff compares the latest sync run against a baseline (the prior run, or the
// most recent run at/before now-since). Each entity is compared across its own
// refreshes under the same --search filter, by presence at the two runs:
// added = present at its latest refresh and not at the baseline; removed =
// present at the baseline and not at its latest refresh.
func Diff(ctx context.Context, db *sql.DB, since time.Duration) (*DiffResult, error) {
	latest, _, ok := latestTwoRuns(ctx, db)
	if !ok {
		return &DiffResult{Assets: emptySection(), Services: emptySection(), Software: emptySection()}, nil
	}
	baseline, _ := baselineForSince(ctx, db, since)
	res := &DiffResult{BaselineRun: baseline, LatestRun: latest}
	// Each entity is compared across the runs that refreshed it: a run that
	// skipped or failed one must not read its rows as removed (runzero handfix
	// diff-compares-each-entitys-own-runs). The skipped ones are named.
	section := func(entity, table, label string) DiffSection {
		c := entityComparison(ctx, db, entity, since)
		if !c.recorded {
			return diffSection(ctx, db, table, label, latest, baseline)
		}
		if c.latest < latest {
			res.StaleEntities = append(res.StaleEntities, entity)
		}
		if c.noBaseline {
			res.NoBaselineEntities = append(res.NoBaselineEntities, entity)
		}
		if c.latest == 0 || c.noBaseline {
			return emptySection()
		}
		if c.baseline == 0 {
			if !rowsBefore(ctx, db, entity, c.latest) {
				// Its first refresh, and nothing older to compare with:
				// no baseline, not every row new.
				res.NoBaselineEntities = append(res.NoBaselineEntities, entity)
				return emptySection()
			}
			// Its first recorded refresh over rows an older build wrote:
			// the rows first seen in it are new.
			return diffSection(ctx, db, table, label, c.latest, c.latest-1)
		}
		if !presenceRecorded(ctx, db, entity, c.baseline) {
			// The baseline has no presence history to compare with.
			return diffSection(ctx, db, table, label, c.latest, c.baseline)
		}
		return presenceSection(ctx, db, entity, table, label, c.latest, c.baseline)
	}
	res.Assets = section("assets", "inv_assets", "COALESCE(NULLIF(name,''), address, id)")
	res.Services = section("services", "inv_services", "COALESCE(asset_id,'')||' '||COALESCE(protocol,'')||'/'||COALESCE(CAST(port AS TEXT),'')")
	res.Software = section("software", "inv_software", "COALESCE(product,'')||' '||COALESCE(version,'')")
	return res, nil
}

// comparison is one entity's two runs to compare.
type comparison struct {
	latest, baseline int64
	// noBaseline: the entity has earlier refreshes, none under the latest
	// refresh's --search filter, so there is nothing comparable.
	noBaseline bool
	// recorded is false for a store no run of this build wrote (no
	// inv_sync_entities rows at all): the caller keeps the global runs.
	recorded bool
}

// entityComparison resolves the latest run that refreshed entity and the
// baseline to compare it with: its previous refresh under the same --search
// filter, or with since the most recent such refresh started at or before
// now-since (the previous one when none is that old). A filtered refresh
// returns only what the filter matches, so comparing it with a refresh under
// another filter read every row the filter left out as removed (runzero
// handfix r1-compare-same-scope). latest 0 means no run refreshed the entity;
// baseline 0 with noBaseline false means this is its first refresh (every row
// it holds is new).
func entityComparison(ctx context.Context, db *sql.DB, entity string, since time.Duration) comparison {
	var recordedRuns int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_sync_entities`).Scan(&recordedRuns); err != nil || recordedRuns == 0 {
		return comparison{}
	}
	rows, err := db.QueryContext(ctx, `SELECT r.id, r.started_at, COALESCE(r.note, '') FROM inv_sync_entities e JOIN inv_sync_runs r ON r.id = e.run_id
		WHERE e.entity = ? ORDER BY r.id DESC`, entity)
	if err != nil {
		return comparison{recorded: true}
	}
	defer rows.Close()
	type run struct {
		id, started int64
		scope       string
	}
	var runs []run
	for rows.Next() {
		var r run
		if rows.Scan(&r.id, &r.started, &r.scope) == nil {
			r.scope = strings.TrimSpace(r.scope)
			runs = append(runs, r)
		}
	}
	if len(runs) == 0 {
		return comparison{recorded: true}
	}
	c := comparison{latest: runs[0].id, recorded: true}
	var same []run
	for _, r := range runs[1:] {
		if r.scope == runs[0].scope {
			same = append(same, r)
		}
	}
	if len(same) == 0 {
		c.noBaseline = len(runs) > 1
		return c
	}
	c.baseline = same[0].id
	if since > 0 {
		cutoff := clock().Add(-since).Unix()
		for _, r := range same {
			if r.started <= cutoff {
				c.baseline = r.id
				break
			}
		}
	}
	return c
}

// presenceRecorded reports whether run recorded the presence of entity's rows
// (inv_presence_runs). Presence is compared only at a baseline that recorded
// it: a run an older build wrote (inv_sync_entities, no presence history) has
// none, so comparing by presence read every row it returned as absent, which
// made each unchanged row added and dropped each removal (runzero handfix
// r2-presence-history-per-run); such a baseline is compared by
// first_run/last_run. A failed lookup reads as not recorded.
// rowsBefore reports whether entity holds rows first seen before run, which only
// a build that did not record entity refreshes can have written. An entity's
// first recorded refresh is compared with those rows; with none, it has no
// baseline at all (runzero handfix r2-first-refresh-has-no-baseline). Each
// query is a constant string.
func rowsBefore(ctx context.Context, db *sql.DB, entity string, run int64) bool {
	var q string
	switch entity {
	case "assets":
		q = `SELECT EXISTS (SELECT 1 FROM inv_assets WHERE first_run < ?)`
	case "services":
		q = `SELECT EXISTS (SELECT 1 FROM inv_services WHERE first_run < ?)`
	case "software":
		q = `SELECT EXISTS (SELECT 1 FROM inv_software WHERE first_run < ?)`
	case "vulnerabilities":
		q = `SELECT EXISTS (SELECT 1 FROM inv_vulnerabilities WHERE first_run < ?)`
	default:
		return false
	}
	var n int
	if err := db.QueryRowContext(ctx, q, run).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func presenceRecorded(ctx context.Context, db *sql.DB, entity string, run int64) bool {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_presence_runs WHERE run_id = ? AND entity = ?`, run, entity).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// presentAt is the SQL condition "row <alias>.id of entity was returned by
// run ?" from the recorded presence streaks (runzero handfix
// r1-presence-per-refresh); bind entity, run, run.
func presentAt(alias string) string {
	return `EXISTS (SELECT 1 FROM inv_presence p WHERE p.entity = ? AND p.id = ` + alias + `.id AND p.first_run <= ? AND p.last_run >= ?)`
}

// presenceSection lists the rows of table present at latest and not at
// baseline (added) and present at baseline and not at latest (removed).
// latest is the entity's latest refresh, so a row is present at it exactly
// when its last_run is latest. table and label are the constants Diff passes.
func presenceSection(ctx context.Context, db *sql.DB, entity, table, label string, latest, baseline int64) DiffSection {
	sec := emptySection()
	collect := func(q string, args ...any) []string {
		out := []string{}
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	sec.Added = collect(`SELECT `+label+` FROM `+table+` t WHERE t.last_run = ? AND NOT `+presentAt("t")+` ORDER BY 1`,
		latest, entity, baseline, baseline)
	sec.Removed = collect(`SELECT `+label+` FROM `+table+` t WHERE t.last_run <> ? AND `+presentAt("t")+` ORDER BY 1`,
		latest, entity, baseline, baseline)
	return sec
}

func emptySection() DiffSection { return DiffSection{Added: []string{}, Removed: []string{}} }

// diffSection interpolates table and label directly into the query because
// SQLite cannot bind table or column identifiers as parameters. Both values are
// caller-supplied compile-time constants (see Diff above: only "inv_assets",
// "inv_services", "inv_software" and fixed COALESCE label expressions are ever
// passed), never derived from user input, so there is no injection surface. The
// row filter values (baseline, latest) are bound with ? as normal. It is the
// comparison for a store no run of this build wrote (no presence records).
func diffSection(ctx context.Context, db *sql.DB, table, label string, latest, baseline int64) DiffSection {
	sec := emptySection()
	// added: first_run > baseline AND last_run = latest
	addRows, err := db.QueryContext(ctx,
		`SELECT `+label+` FROM `+table+` WHERE first_run > ? AND last_run = ? ORDER BY 1`, baseline, latest) // #nosec G202 -- table/label are constant identifiers, not user input; SQLite can't bind identifiers
	if err == nil {
		for addRows.Next() {
			var s string
			if addRows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
				sec.Added = append(sec.Added, s)
			}
		}
		_ = addRows.Close()
	}
	// removed: present at baseline (first_run<=baseline AND last_run>=baseline) AND not at latest (last_run<latest)
	remRows, err := db.QueryContext(ctx,
		`SELECT `+label+` FROM `+table+` WHERE first_run <= ? AND last_run >= ? AND last_run < ? ORDER BY 1`, baseline, baseline, latest) // #nosec G202 -- table/label are constant identifiers, not user input; SQLite can't bind identifiers
	if err == nil {
		for remRows.Next() {
			var s string
			if remRows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
				sec.Removed = append(sec.Removed, s)
			}
		}
		_ = remRows.Close()
	}
	return sec
}

// AffectedRow is one asset affected by a CVE.
type AffectedRow struct {
	AssetID     string   `json:"asset_id"`
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Criticality string   `json:"criticality"`
	CVE         string   `json:"cve"`
	Severity    string   `json:"severity"`
	Services    []string `json:"services"`
}

// Affected lists every asset with a current vulnerability finding matching the
// CVE (case-insensitive substring; LIKE not FTS5, to tolerate the hyphens in
// CVE ids). A finding is joined to its asset only in the current asset
// snapshot (runzero handfix r2-current-asset-snapshot-joins): a finding whose
// asset that snapshot does not hold is listed by its asset id alone, and
// Coverage names it.
func Affected(ctx context.Context, db *sql.DB, cve string) ([]AffectedRow, error) {
	cve = strings.TrimSpace(cve)
	if cve == "" {
		return []AffectedRow{}, nil
	}
	ra, rv, rs := currentRun(ctx, db, "assets"), currentRun(ctx, db, "vulnerabilities"), currentRun(ctx, db, "services")
	q := `
		SELECT COALESCE(v.asset_id,''), COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,''), COALESCE(v.cve,''), COALESCE(v.severity,'')
		FROM inv_vulnerabilities v
		LEFT JOIN inv_assets a ON a.id = v.asset_id AND a.last_run = ?
		WHERE v.last_run = ? AND LOWER(COALESCE(v.cve,'')) LIKE '%' || LOWER(?) || '%'
		ORDER BY COALESCE(a.criticality_rank,0) DESC, v.severity_rank DESC`
	rows, err := db.QueryContext(ctx, q, ra, rv, cve)
	if err != nil {
		return []AffectedRow{}, nil
	}
	defer rows.Close()
	out := []AffectedRow{}
	for rows.Next() {
		var r AffectedRow
		if err := rows.Scan(&r.AssetID, &r.Name, &r.Address, &r.Criticality, &r.CVE, &r.Severity); err != nil {
			continue
		}
		out = append(out, r)
	}
	_ = rows.Close()
	for i := range out {
		out[i].Services = assetServices(ctx, db, out[i].AssetID, rs)
	}
	return out, nil
}

func assetServices(ctx context.Context, db *sql.DB, assetID string, run int64) []string {
	out := []string{}
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT COALESCE(protocol,'')||'/'||COALESCE(CAST(port AS TEXT),'') FROM inv_services WHERE asset_id = ? AND last_run = ? ORDER BY 1`, assetID, run)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil && s != "/" {
			out = append(out, s)
		}
	}
	return out
}

// RollupRow is one product+version group with its asset count.
type RollupRow struct {
	Product    string `json:"product"`
	Version    string `json:"version"`
	AssetCount int    `json:"asset_count"`
}

// SoftwareRollup groups the current software inventory matching name by
// product+version with a distinct-asset count, laggards (most-deployed) first.
func SoftwareRollup(ctx context.Context, db *sql.DB, name string) ([]RollupRow, error) {
	name = strings.TrimSpace(name)
	args := []any{currentRun(ctx, db, "software")}
	where := `WHERE last_run = ?`
	if name != "" {
		where += ` AND (LOWER(product) LIKE '%'||LOWER(?)||'%' OR LOWER(vendor) LIKE '%'||LOWER(?)||'%')`
		args = append(args, name, name)
	}
	q := `SELECT COALESCE(product,'') AS product, COALESCE(version,'') AS version, COUNT(DISTINCT asset_id) AS n
		FROM inv_software ` + where + `
		GROUP BY product, version ORDER BY n DESC, product, version`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return []RollupRow{}, nil
	}
	defer rows.Close()
	out := []RollupRow{}
	for rows.Next() {
		var r RollupRow
		if err := rows.Scan(&r.Product, &r.Version, &r.AssetCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// StaleRow is one asset that is stale, EOL, or unowned.
type StaleRow struct {
	AssetID  string   `json:"asset_id"`
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	OS       string   `json:"os"`
	LastSeen int64    `json:"last_seen"`
	AgeDays  int      `json:"age_days"`
	Reasons  []string `json:"reasons"`
}

// Stale lists current assets not seen in `days` days, plus EOL-OS and
// tag/owner-less assets. days <= 0 defaults to 30.
func Stale(ctx context.Context, db *sql.DB, days int) ([]StaleRow, error) {
	if days <= 0 {
		days = 30
	}
	now := clock().Unix()
	cutoff := now - int64(days)*86400
	q := `SELECT id, COALESCE(name,''), COALESCE(address,''), COALESCE(os,''), COALESCE(last_seen,0), COALESCE(os_eol,0), COALESCE(tags,''), COALESCE(owners,'')
		FROM inv_assets WHERE last_run = ?`
	rows, err := db.QueryContext(ctx, q, currentRun(ctx, db, "assets"))
	if err != nil {
		return []StaleRow{}, nil
	}
	defer rows.Close()
	out := []StaleRow{}
	for rows.Next() {
		var r StaleRow
		var osEOL int64
		var tags, owners string
		if err := rows.Scan(&r.AssetID, &r.Name, &r.Address, &r.OS, &r.LastSeen, &osEOL, &tags, &owners); err != nil {
			continue
		}
		var reasons []string
		if r.LastSeen > 0 && r.LastSeen < cutoff {
			reasons = append(reasons, fmt.Sprintf("not seen in %dd", days))
		}
		if osEOL > 0 && osEOL < now {
			reasons = append(reasons, "os-eol")
		}
		if isEmptyJSON(tags) {
			reasons = append(reasons, "untagged")
		}
		if isEmptyJSON(owners) {
			reasons = append(reasons, "no-owner")
		}
		if len(reasons) == 0 {
			continue
		}
		if r.LastSeen > 0 {
			r.AgeDays = int((now - r.LastSeen) / 86400)
		}
		r.Reasons = reasons
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AgeDays > out[j].AgeDays })
	return out, nil
}

func isEmptyJSON(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || t == "null" || t == "{}" || t == "[]"
}

// ExposureRow is one exposed protocol/port with its asset count.
type ExposureRow struct {
	Transport  string `json:"transport"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	AssetCount int    `json:"asset_count"`
}

// ExposureMap rolls up the current exposed services across a CIDR, counted per
// protocol/port by distinct asset (service_asset_id: separate runZero assets
// can share an address; a service with no asset link counts by its address).
// CIDR matching is done in Go against each service's address (and the parent
// asset's address, from the current asset snapshot only: runzero handfix
// r2-current-asset-snapshot-joins) since SQLite has no native CIDR predicate.
func ExposureMap(ctx context.Context, db *sql.DB, cidr string) ([]ExposureRow, error) {
	cidr = strings.TrimSpace(cidr)
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	q := `SELECT COALESCE(s.transport,''), COALESCE(s.port,0), COALESCE(s.protocol,''), COALESCE(s.address,''), COALESCE(a.address,''), COALESCE(s.asset_id,'')
		FROM inv_services s LEFT JOIN inv_assets a ON a.id = s.asset_id AND a.last_run = ? WHERE s.last_run = ?`
	rows, err := db.QueryContext(ctx, q, currentRun(ctx, db, "assets"), currentRun(ctx, db, "services"))
	if err != nil {
		return []ExposureRow{}, nil
	}
	defer rows.Close()
	type key struct {
		transport, protocol string
		port                int
	}
	agg := map[key]map[string]bool{} // key -> set of assets
	for rows.Next() {
		var transport, protocol, svcAddr, assetAddr, assetID string
		var port int
		if err := rows.Scan(&transport, &port, &protocol, &svcAddr, &assetAddr, &assetID); err != nil {
			continue
		}
		addr := firstIPIn(ipnet, svcAddr, assetAddr)
		if addr == "" {
			continue
		}
		k := key{transport: transport, protocol: protocol, port: port}
		if agg[k] == nil {
			agg[k] = map[string]bool{}
		}
		member := "address:" + addr
		if strings.TrimSpace(assetID) != "" {
			member = "asset:" + assetID
		}
		agg[k][member] = true
	}
	out := []ExposureRow{}
	for k, set := range agg {
		out = append(out, ExposureRow{Transport: k.transport, Port: k.port, Protocol: k.protocol, AssetCount: len(set)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AssetCount != out[j].AssetCount {
			return out[i].AssetCount > out[j].AssetCount
		}
		return out[i].Port < out[j].Port
	})
	return out, nil
}

// firstIPIn returns the first candidate address that is a single IP inside ipnet.
func firstIPIn(ipnet *net.IPNet, candidates ...string) string {
	for _, c := range candidates {
		for _, part := range strings.FieldsFunc(c, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
			ip := net.ParseIP(strings.TrimSpace(part))
			if ip != nil && ipnet.Contains(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

// ListableEntities is the set of entities `inventory list` can browse.
var ListableEntities = map[string]string{
	"assets": "inv_assets", "services": "inv_services", "software": "inv_software",
	"vulnerabilities": "inv_vulnerabilities", "certificates": "inv_certificates",
	"wireless": "inv_wireless", "sites": "inv_sites",
}

// List returns the stored entity rows (the original runZero JSON for each row)
// of the entity's current snapshot, capped at limit. match is an optional
// case-insensitive substring filtered against the raw row JSON. Returns an
// empty slice (never an error) when the entity table is empty.
func List(ctx context.Context, db *sql.DB, entity string, limit int, match string) ([]json.RawMessage, error) {
	entity = strings.ToLower(strings.TrimSpace(entity))
	tbl, ok := ListableEntities[entity]
	if !ok {
		return nil, fmt.Errorf("unknown entity %q (one of: assets, services, software, vulnerabilities, certificates, wireless, sites)", entity)
	}
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT COALESCE(data,'{}') FROM ` + tbl + ` WHERE last_run = ?`
	args := []any{currentRun(ctx, db, entity)}
	if strings.TrimSpace(match) != "" {
		q += ` AND LOWER(data) LIKE '%'||LOWER(?)||'%'`
		args = append(args, match)
	}
	q += ` LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return []json.RawMessage{}, nil
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		out = append(out, json.RawMessage(data))
	}
	return out, nil
}

// StatusResult reports per-entity row counts and the last sync time.
type StatusResult struct {
	// Tables counts each entity's current snapshot (currentRun).
	Tables map[string]int `json:"tables"`
	// StoredRows counts every row the store keeps, earlier snapshots included.
	StoredRows  map[string]int `json:"stored_rows"`
	Runs        int            `json:"sync_runs"`
	LastSync    int64          `json:"last_sync_unix"`
	LastSyncRFC string         `json:"last_sync,omitempty"`
	// Incomplete is Coverage over every entity.
	Incomplete []string `json:"incomplete,omitempty"`
}

// Status returns row counts per inventory table and the last sync timestamp.
func Status(ctx context.Context, db *sql.DB) (*StatusResult, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	res := &StatusResult{Tables: map[string]int{}, StoredRows: map[string]int{}}
	for label := range ListableEntities {
		var n, all int
		_ = db.QueryRowContext(ctx, currentRowsQuery[label], currentRun(ctx, db, label)).Scan(&n)
		_ = db.QueryRowContext(ctx, storedRowsQuery[label]).Scan(&all)
		res.Tables[label] = n
		res.StoredRows[label] = all
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_sync_runs`).Scan(&res.Runs)
	var last sql.NullInt64
	_ = db.QueryRowContext(ctx, `SELECT MAX(finished_at) FROM inv_sync_runs`).Scan(&last)
	if last.Valid {
		res.LastSync = last.Int64
		res.LastSyncRFC = time.Unix(last.Int64, 0).UTC().Format(time.RFC3339)
	}
	if res.Runs > 0 {
		res.Incomplete = Coverage(ctx, db, Entities...)
	}
	return res, nil
}
