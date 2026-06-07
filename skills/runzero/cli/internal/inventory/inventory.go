// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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

func severityRank(m map[string]any) (string, int) {
	raw := firstStr(m, "severity", "risk", "risk_rank", "rank")
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
	now := time.Now().Unix()
	res, err := db.ExecContext(ctx, `INSERT INTO inv_sync_runs(started_at, note) VALUES(?, ?)`, now, search)
	if err != nil {
		return nil, fmt.Errorf("open sync run: %w", err)
	}
	runID, _ := res.LastInsertId()

	want := map[string]bool{}
	for _, e := range only {
		want[strings.ToLower(strings.TrimSpace(e))] = true
	}
	out := &SyncResult{RunID: runID, Counts: map[string]int{}, Errors: map[string]string{}}
	params := map[string]string{}
	if search != "" {
		params["search"] = search
	}

	for _, entity := range Entities {
		if len(want) > 0 && !want[entity] {
			continue
		}
		raw, err := g.Get(ctx, exportPath(entity), params)
		if err != nil {
			out.Errors[entity] = err.Error()
			continue
		}
		objs, err := decodeObjects(raw)
		if err != nil {
			out.Errors[entity] = "decode: " + err.Error()
			continue
		}
		n, err := upsertEntity(ctx, db, entity, objs, runID)
		if err != nil {
			out.Errors[entity] = err.Error()
			continue
		}
		out.Counts[entity] = n
	}
	_, _ = db.ExecContext(ctx, `UPDATE inv_sync_runs SET finished_at=? WHERE id=?`, time.Now().Unix(), runID)
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out, nil
}

func upsertEntity(ctx context.Context, db *sql.DB, entity string, objs []map[string]any, runID int64) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	for i, m := range objs {
		if err := upsertRow(ctx, tx, entity, m, runID, i); err != nil {
			return count, err
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

// upsertRow inserts or updates one entity row, preserving first_run and
// advancing last_run to the current run id.
func upsertRow(ctx context.Context, tx *sql.Tx, entity string, m map[string]any, runID int64, idx int) error {
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
			firstInt(m, "os_eol", "eol_date"), firstStr(m, "type", "device_type"), firstStr(m, "hardware", "hw"),
			rawJSON(strSlice(m, "mac_vendors", "macs")), crit, critRank, firstNum(m, "risk", "risk_score"),
			boolToInt(firstBool(m, "alive")), boolToInt(firstBool(m, "external", "outernet", "public")),
			rawJSON(m["tags"]), rawJSON(m["owners"]), firstStr(m, "site_id"), firstStr(m, "site_name", "site"),
			firstStr(m, "org_id", "organization_id"), firstInt(m, "first_seen"), firstInt(m, "last_seen", "updated_at"),
			runID, runID, rawJSON(m))
		return err
	case "services":
		id := firstStr(m, "id", "service_id")
		if id == "" {
			id = fmt.Sprintf("%s-%s-%d", firstStr(m, "asset_id"), firstStr(m, "service_port", "port"), idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_services(id,asset_id,address,transport,port,protocol,product,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,address=excluded.address,transport=excluded.transport,
				port=excluded.port,protocol=excluded.protocol,product=excluded.product,last_run=excluded.last_run,data=excluded.data`,
			id, firstStr(m, "asset_id"), firstStr(m, "address", "service_address"),
			strings.ToLower(firstStr(m, "transport", "service_transport")), firstInt(m, "port", "service_port"),
			firstStr(m, "protocol", "service_protocol", "protocols"), firstStr(m, "product", "service_product"),
			runID, runID, rawJSON(m))
		return err
	case "software":
		id := firstStr(m, "id", "software_id")
		if id == "" {
			id = fmt.Sprintf("%s-%s-%s-%d", firstStr(m, "asset_id"), firstStr(m, "product", "software_product"), firstStr(m, "version", "software_version"), idx)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inv_software(id,asset_id,vendor,product,version,first_run,last_run,data)
			VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET asset_id=excluded.asset_id,vendor=excluded.vendor,product=excluded.product,
				version=excluded.version,last_run=excluded.last_run,data=excluded.data`,
			id, firstStr(m, "asset_id"), firstStr(m, "vendor", "software_vendor"),
			firstStr(m, "product", "software_product", "name"), firstStr(m, "version", "software_version"),
			runID, runID, rawJSON(m))
		return err
	case "vulnerabilities":
		id := firstStr(m, "id", "vulnerability_id", "finding_id")
		if id == "" {
			id = fmt.Sprintf("%s-%s-%d", firstStr(m, "asset_id"), firstStr(m, "cve"), idx)
		}
		sev, sevRank := severityRank(m)
		cve := firstStr(m, "cve")
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
			id, firstStr(m, "asset_id"), firstStr(m, "name", "title"), cve, sev, sevRank,
			firstNum(m, "cvss", "cvss_score", "cvss3"), runID, runID, rawJSON(m))
		return err
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
			firstInt(m, "not_after", "expiration", "valid_to"), boolToInt(firstBool(m, "self_signed", "selfsigned")),
			runID, runID, rawJSON(m))
		return err
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
		return err
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
		return err
	}
	return nil
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
	cutoff := time.Now().Add(-since).Unix()
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
func Triage(ctx context.Context, db *sql.DB, internetFacing bool) ([]TriageRow, error) {
	where := "WHERE a.last_run = (SELECT MAX(last_run) FROM inv_assets)"
	if internetFacing {
		where += " AND a.external = 1"
	}
	q := `
		SELECT a.id, COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,''), COALESCE(a.os,''),
			(SELECT COUNT(*) FROM inv_vulnerabilities v WHERE v.asset_id = a.id) AS vuln_count,
			COALESCE((SELECT MAX(v.severity_rank) FROM inv_vulnerabilities v WHERE v.asset_id = a.id), 0) AS max_sev,
			(SELECT COUNT(*) FROM inv_services s WHERE s.asset_id = a.id) AS svc_count,
			COALESCE((SELECT v.cve FROM inv_vulnerabilities v WHERE v.asset_id = a.id ORDER BY v.severity_rank DESC, v.cvss DESC LIMIT 1), '') AS top_cve
		FROM inv_assets a ` + where + `
		ORDER BY a.criticality_rank DESC, max_sev DESC, vuln_count DESC, a.risk DESC`
	rows, err := db.QueryContext(ctx, q)
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
}

// DiffSection lists what was added and removed for one entity.
type DiffSection struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// Diff compares the latest sync run against a baseline (the prior run, or the
// most recent run at/before now-since). Uses contiguous-presence semantics:
// added = first appeared after baseline and present at latest; removed =
// present at baseline but not at latest.
func Diff(ctx context.Context, db *sql.DB, since time.Duration) (*DiffResult, error) {
	latest, _, ok := latestTwoRuns(ctx, db)
	if !ok {
		return &DiffResult{Assets: emptySection(), Services: emptySection(), Software: emptySection()}, nil
	}
	baseline, _ := baselineForSince(ctx, db, since)
	res := &DiffResult{BaselineRun: baseline, LatestRun: latest}
	res.Assets = diffSection(ctx, db, "inv_assets", "COALESCE(NULLIF(name,''), address, id)", latest, baseline)
	res.Services = diffSection(ctx, db, "inv_services", "COALESCE(asset_id,'')||' '||COALESCE(protocol,'')||'/'||COALESCE(CAST(port AS TEXT),'')", latest, baseline)
	res.Software = diffSection(ctx, db, "inv_software", "COALESCE(product,'')||' '||COALESCE(version,'')", latest, baseline)
	return res, nil
}

func emptySection() DiffSection { return DiffSection{Added: []string{}, Removed: []string{}} }

// diffSection interpolates table and label directly into the query because
// SQLite cannot bind table or column identifiers as parameters. Both values are
// caller-supplied compile-time constants (see Diff above: only "inv_assets",
// "inv_services", "inv_software" and fixed COALESCE label expressions are ever
// passed), never derived from user input, so there is no injection surface. The
// row filter values (baseline, latest) are bound with ? as normal.
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

// Affected lists every asset with a vulnerability matching the CVE (case-insensitive
// substring; LIKE not FTS5, to tolerate the hyphens in CVE ids).
func Affected(ctx context.Context, db *sql.DB, cve string) ([]AffectedRow, error) {
	cve = strings.TrimSpace(cve)
	if cve == "" {
		return []AffectedRow{}, nil
	}
	q := `
		SELECT a.id, COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,''), COALESCE(v.cve,''), COALESCE(v.severity,'')
		FROM inv_vulnerabilities v
		JOIN inv_assets a ON a.id = v.asset_id
		WHERE LOWER(COALESCE(v.cve,'')) LIKE '%' || LOWER(?) || '%'
		ORDER BY a.criticality_rank DESC, v.severity_rank DESC`
	rows, err := db.QueryContext(ctx, q, cve)
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
		r.Services = assetServices(ctx, db, r.AssetID)
		out = append(out, r)
	}
	return out, nil
}

func assetServices(ctx context.Context, db *sql.DB, assetID string) []string {
	out := []string{}
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT COALESCE(protocol,'')||'/'||COALESCE(CAST(port AS TEXT),'') FROM inv_services WHERE asset_id = ? ORDER BY 1`, assetID)
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

// SoftwareRollup groups installed software matching name by product+version
// with a distinct-asset count, laggards (most-deployed) first.
func SoftwareRollup(ctx context.Context, db *sql.DB, name string) ([]RollupRow, error) {
	name = strings.TrimSpace(name)
	args := []any{}
	where := ""
	if name != "" {
		where = `WHERE LOWER(product) LIKE '%'||LOWER(?)||'%' OR LOWER(vendor) LIKE '%'||LOWER(?)||'%'`
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

// Stale lists assets not seen in `days` days, plus EOL-OS and tag/owner-less
// assets. days <= 0 defaults to 30.
func Stale(ctx context.Context, db *sql.DB, days int) ([]StaleRow, error) {
	if days <= 0 {
		days = 30
	}
	now := time.Now().Unix()
	cutoff := now - int64(days)*86400
	q := `SELECT id, COALESCE(name,''), COALESCE(address,''), COALESCE(os,''), COALESCE(last_seen,0), COALESCE(os_eol,0), COALESCE(tags,''), COALESCE(owners,'')
		FROM inv_assets WHERE last_run = (SELECT MAX(last_run) FROM inv_assets)`
	rows, err := db.QueryContext(ctx, q)
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

// ExposureMap rolls up exposed services across a CIDR, asset-counted per
// protocol/port. CIDR matching is done in Go against each service's address
// (and the parent asset's address) since SQLite has no native CIDR predicate.
func ExposureMap(ctx context.Context, db *sql.DB, cidr string) ([]ExposureRow, error) {
	cidr = strings.TrimSpace(cidr)
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	q := `SELECT COALESCE(s.transport,''), COALESCE(s.port,0), COALESCE(s.protocol,''), COALESCE(s.address,''), COALESCE(a.address,'')
		FROM inv_services s LEFT JOIN inv_assets a ON a.id = s.asset_id`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return []ExposureRow{}, nil
	}
	defer rows.Close()
	type key struct {
		transport, protocol string
		port                int
	}
	agg := map[key]map[string]bool{} // key -> set of asset addresses
	for rows.Next() {
		var transport, protocol, svcAddr, assetAddr string
		var port int
		if err := rows.Scan(&transport, &port, &protocol, &svcAddr, &assetAddr); err != nil {
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
		agg[k][addr] = true
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
// from the latest sync, newest first, capped at limit. match is an optional
// case-insensitive substring filtered against the raw row JSON. Returns an
// empty slice (never an error) when the entity table is empty.
func List(ctx context.Context, db *sql.DB, entity string, limit int, match string) ([]json.RawMessage, error) {
	tbl, ok := ListableEntities[strings.ToLower(strings.TrimSpace(entity))]
	if !ok {
		return nil, fmt.Errorf("unknown entity %q (one of: assets, services, software, vulnerabilities, certificates, wireless, sites)", entity)
	}
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT COALESCE(data,'{}') FROM ` + tbl + ` WHERE last_run = (SELECT MAX(last_run) FROM ` + tbl + `)`
	args := []any{}
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

// StatusResult reports per-table row counts and the last sync time.
type StatusResult struct {
	Tables      map[string]int `json:"tables"`
	Runs        int            `json:"sync_runs"`
	LastSync    int64          `json:"last_sync_unix"`
	LastSyncRFC string         `json:"last_sync,omitempty"`
}

// Status returns row counts per inventory table and the last sync timestamp.
func Status(ctx context.Context, db *sql.DB) (*StatusResult, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	res := &StatusResult{Tables: map[string]int{}}
	tables := map[string]string{
		"assets": "inv_assets", "services": "inv_services", "software": "inv_software",
		"vulnerabilities": "inv_vulnerabilities", "certificates": "inv_certificates",
		"wireless": "inv_wireless", "sites": "inv_sites",
	}
	for label, tbl := range tables {
		var n int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+tbl).Scan(&n)
		res.Tables[label] = n
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_sync_runs`).Scan(&res.Runs)
	var last sql.NullInt64
	_ = db.QueryRowContext(ctx, `SELECT MAX(finished_at) FROM inv_sync_runs`).Scan(&last)
	if last.Valid {
		res.LastSync = last.Int64
		res.LastSyncRFC = time.Unix(last.Int64, 0).UTC().Format(time.RFC3339)
	}
	return res, nil
}
