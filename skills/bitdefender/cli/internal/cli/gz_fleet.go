// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Shared engine for the cross-company (fleet) commands: the partner tree walk,
// the per-company endpoint fan-out, and the local snapshot tables that give
// GravityZone the history its API does not keep. Method contracts come from the
// GravityZone Partners Public API docs (77211-*); see spec.yaml x-source.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"bitdefender-pp-cli/internal/client"
	"bitdefender-pp-cli/internal/cliutil"
	"bitdefender-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

const (
	gzPathCompanyDetails = "/v1.0/jsonrpc/companies#getCompanyDetails"
	gzPathCompaniesList  = "/v1.0/jsonrpc/network#getCompaniesList"
	gzPathFoldersList    = "/v1.0/jsonrpc/network#getCompanyFoldersList"
	gzPathEndpointsList  = "/v1.1/jsonrpc/network#getEndpointsList"
	gzPathEndpointDetail = "/v1.0/jsonrpc/network#getManagedEndpointDetails"
	gzPathUsageByProduct = "/v1.0/jsonrpc/licensing#getMonthlyUsagePerProductType"
	gzPathIncidentsList  = "/v1.2/jsonrpc/incidents#getIncidentsList"
	gzPathQuarantineList = "/v1.0/jsonrpc/quarantine/computers#getQuarantineItemsList"
	gzPathBlocklistItems = "/v1.2/jsonrpc/incidents#getBlocklistItems"
)

// gzRead calls one read-only JSON-RPC method; the jsonrpc transport adds the
// envelope and returns the `result` member.
func gzRead(ctx context.Context, c *client.Client, path string, params map[string]any) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	data, _, err := c.PostQueryWithParams(ctx, path, nil, params)
	return data, err
}

type fetchFailure struct {
	Scope string `json:"scope"`
	Error string `json:"error"`
}

type gzCompany struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"` // 0 partner, 1 customer (getCompanyDetails / getCompaniesList companyType)
	ParentID string `json:"parent_id,omitempty"`
	Depth    int    `json:"depth"`
	Path     string `json:"path"`
}

type idName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// walkCompanyTree returns the API key's company and every company below it.
// getCompaniesList answers one level per call, so this is a breadth-first walk
// over sub-partners (companyType 0) and customers (companyType 1), including
// companies filed in company folders. Duplicates are dropped by id.
// ponytail: sequential calls; the client limiter paces them. Parallelize per
// level if trees of thousands of companies make this slow.
func walkCompanyTree(ctx context.Context, c *client.Client, maxCompanies int) ([]gzCompany, []fetchFailure, error) {
	raw, err := gzRead(ctx, c, gzPathCompanyDetails, nil)
	if err != nil {
		return nil, nil, err
	}
	var root struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type int    `json:"type"`
	}
	if err := json.Unmarshal(raw, &root); err != nil || root.ID == "" {
		return nil, nil, fmt.Errorf("getCompanyDetails returned no company id")
	}
	out := []gzCompany{{ID: root.ID, Name: root.Name, Type: root.Type, Path: root.Name}}
	seen := map[string]bool{root.ID: true}
	var failures []fetchFailure
	// owner is the company a node's children belong to: the node itself for a
	// company, the enclosing company for a folder.
	type node struct {
		id, owner, path string
		depth           int
	}
	queue := []node{}
	if root.Type == 0 {
		queue = append(queue, node{id: root.ID, owner: root.ID, path: root.Name})
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, ct := range []int{0, 1} {
			kids, err := listIDNames(ctx, c, gzPathCompaniesList, map[string]any{"parentId": n.id, "filters": map[string]any{"companyType": ct}})
			if err != nil {
				failures = append(failures, fetchFailure{Scope: "getCompaniesList " + n.id, Error: err.Error()})
				continue
			}
			for _, k := range kids {
				if k.ID == "" || seen[k.ID] {
					continue
				}
				if maxCompanies > 0 && len(out) >= maxCompanies {
					return out, failures, fmt.Errorf("company tree exceeds --max-companies %d; raise it to walk the rest", maxCompanies)
				}
				seen[k.ID] = true
				out = append(out, gzCompany{ID: k.ID, Name: k.Name, Type: ct, ParentID: n.owner, Depth: n.depth + 1, Path: n.path + " / " + k.Name})
				if ct == 0 {
					queue = append(queue, node{id: k.ID, owner: k.ID, path: n.path + " / " + k.Name, depth: n.depth + 1})
				}
			}
		}
		folders, err := listIDNames(ctx, c, gzPathFoldersList, map[string]any{"parentId": n.id})
		if err != nil {
			failures = append(failures, fetchFailure{Scope: "getCompanyFoldersList " + n.id, Error: err.Error()})
			continue
		}
		for _, f := range folders {
			if f.ID == "" || seen["folder:"+f.ID] {
				continue
			}
			seen["folder:"+f.ID] = true
			// A folder is not a company: its children keep the enclosing company as
			// parent and the folder's depth.
			queue = append(queue, node{id: f.ID, owner: n.owner, path: n.path + " / " + f.Name, depth: n.depth})
		}
	}
	return out, failures, nil
}

func listIDNames(ctx context.Context, c *client.Client, path string, params map[string]any) ([]idName, error) {
	raw, err := gzRead(ctx, c, path, params)
	if err != nil {
		return nil, err
	}
	var items []idName
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("unexpected %s response: %w", path[strings.LastIndex(path, "#")+1:], err)
	}
	return items, nil
}

type gzEndpoint struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Label           string   `json:"label,omitempty"`
	FQDN            string   `json:"fqdn,omitempty"`
	IsManaged       bool     `json:"isManaged"`
	MachineType     int      `json:"machineType"`
	OS              string   `json:"operatingSystemVersion,omitempty"`
	IsIsolated      bool     `json:"isIsolated"`
	IP              string   `json:"ip,omitempty"`
	MACs            []string `json:"macs,omitempty"`
	ProductOutdated bool     `json:"productOutdated"`
	Policy          *struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Applied bool   `json:"applied"`
	} `json:"policy,omitempty"`
	LastScan *struct {
		Name string `json:"name"`
		Date string `json:"date"`
	} `json:"lastSuccessfulScan,omitempty"`
}

// gzInventoryScope is stated on every fleet view that reads endpoint
// inventory: getEndpointsList does not return endpoints that exist only in
// the Active Directory integration (Bitdefender points those callers to
// getNetworkInventoryItems), so a "clean" or "missing" answer covers this
// inventory only.
const gzInventoryScope = "getEndpointsList inventory (Computers and Groups, recursive). Endpoints that exist only under the Active Directory integration are not included; list them with 'bitdefender-cli inventory list' (getNetworkInventoryItems; Credential tier, because its company rows carry license keys)."

// listCompanyEndpoints pages getEndpointsList v1.1 for one company, recursing
// into its groups, with the productOutdated and lastSuccessfulScan options on.
func listCompanyEndpoints(ctx context.Context, c *client.Client, companyID string, maxPages int) ([]gzEndpoint, int, error) {
	var all []gzEndpoint
	pages, pagesCount, total := 0, 0, -1 // pagesCount and total arrive on page 1 only
	seen := map[string]bool{}
	for page := 1; maxPages <= 0 || page <= maxPages; page++ {
		filters := map[string]any{"depth": map[string]any{"allItemsRecursively": true}}
		raw, err := gzRead(ctx, c, gzPathEndpointsList, map[string]any{
			"parentId": companyID,
			"page":     page,
			"perPage":  1000,
			"filters":  filters,
			"options":  map[string]any{"returnProductOutdated": true, "includeScanLogs": true},
		})
		if err != nil {
			return all, pages, err
		}
		pages++
		var resp struct {
			Items          []gzEndpoint `json:"items"`
			PagesCount     int          `json:"pagesCount"`
			Total          *int         `json:"total"`
			HasMoreRecords *bool        `json:"hasMoreRecords"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return all, pages, fmt.Errorf("unexpected getEndpointsList response: %w", err)
		}
		for _, e := range resp.Items {
			if e.ID == "" || !seen[e.ID] {
				seen[e.ID] = true
				all = append(all, e)
			}
		}
		if page == 1 {
			pagesCount = resp.PagesCount
			if resp.Total != nil {
				total = *resp.Total
			}
		}
		more := len(resp.Items) > 0
		if resp.HasMoreRecords != nil {
			more = more && *resp.HasMoreRecords
		} else if pagesCount > 0 {
			more = more && page < pagesCount
		} else {
			more = more && len(resp.Items) >= 1000
		}
		if !more {
			// An endpoint removed while we paged shifts later pages and hides
			// one that still exists: fewer unique endpoints than page 1 counted.
			if total >= 0 && len(all) < total {
				return all, pages, fmt.Errorf("incomplete: the endpoint list changed while paging (%d of %d received); rerun", len(all), total)
			}
			return all, pages, nil
		}
	}
	// Returning a partial list as complete would undercount and make the next
	// `fleet changes` report the unseen endpoints as removed.
	return all, pages, fmt.Errorf("incomplete: more than %d pages of endpoints (raise --max-pages)", maxPages)
}

// --- local snapshot store -------------------------------------------------

const gzSchema = `
CREATE TABLE IF NOT EXISTS gz_company_tree (id INTEGER PRIMARY KEY CHECK (id = 1), taken_at TEXT NOT NULL, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS gz_fleet_runs (run_id INTEGER PRIMARY KEY AUTOINCREMENT, taken_at TEXT NOT NULL, companies INTEGER NOT NULL, endpoints INTEGER NOT NULL, failures INTEGER NOT NULL, tree_complete INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS gz_fleet_companies (run_id INTEGER NOT NULL, company_id TEXT NOT NULL, company_name TEXT, ok INTEGER NOT NULL, PRIMARY KEY (run_id, company_id));
CREATE TABLE IF NOT EXISTS gz_fleet_endpoints (
  run_id INTEGER NOT NULL, company_id TEXT NOT NULL, company_name TEXT, endpoint_id TEXT NOT NULL,
  name TEXT, fqdn TEXT, ip TEXT, macs TEXT, label TEXT, is_managed INTEGER, is_isolated INTEGER,
  policy_id TEXT, policy_name TEXT, policy_applied INTEGER, product_outdated INTEGER, last_scan TEXT,
  machine_type INTEGER, os TEXT, PRIMARY KEY (run_id, company_id, endpoint_id));
CREATE TABLE IF NOT EXISTS gz_incident_cursors (filter_key TEXT PRIMARY KEY, last_change TEXT NOT NULL);
`

func openGZStore(ctx context.Context) (*store.Store, error) {
	s, err := store.OpenWithContext(ctx, defaultDBPath("bitdefender-cli"))
	if err != nil {
		return nil, err
	}
	if _, err := s.DB().ExecContext(ctx, gzSchema); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("preparing fleet snapshot tables: %w", err)
	}
	// Databases from before tree_complete existed: add it, marking old runs
	// incomplete so they never feed company add/remove inference.
	if _, err := s.DB().ExecContext(ctx, `ALTER TABLE gz_fleet_runs ADD COLUMN tree_complete INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		_ = s.Close()
		return nil, fmt.Errorf("migrating fleet snapshot tables: %w", err)
	}
	return s, nil
}

func saveCompanyTree(ctx context.Context, db *sql.DB, tree []gzCompany) error {
	b, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO gz_company_tree (id, taken_at, data) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET taken_at = excluded.taken_at, data = excluded.data`, time.Now().UTC().Format(time.RFC3339), string(b))
	return err
}

func loadCompanyTree(ctx context.Context, db *sql.DB) ([]gzCompany, time.Time, error) {
	var takenAt, data string
	err := db.QueryRowContext(ctx, `SELECT taken_at, data FROM gz_company_tree WHERE id = 1`).Scan(&takenAt, &data)
	if err != nil {
		return nil, time.Time{}, err
	}
	var tree []gzCompany
	if err := json.Unmarshal([]byte(data), &tree); err != nil {
		return nil, time.Time{}, err
	}
	t, _ := time.Parse(time.RFC3339, takenAt)
	return tree, t, nil
}

// companyTreeFor returns the cached tree when it is younger than maxAge,
// otherwise walks the API. Only a complete walk (no failed calls, not cut off
// by --max-companies) is cached, so a cache hit is always a complete tree.
func companyTreeFor(ctx context.Context, c *client.Client, db *sql.DB, maxAge time.Duration, maxCompanies int) ([]gzCompany, []fetchFailure, error) {
	if maxAge > 0 {
		if tree, at, err := loadCompanyTree(ctx, db); err == nil && len(tree) > 0 && time.Since(at) <= maxAge {
			if maxCompanies > 0 && len(tree) > maxCompanies {
				return tree[:maxCompanies], nil, fmt.Errorf("company tree exceeds --max-companies %d; raise it to walk the rest", maxCompanies)
			}
			return tree, nil, nil
		}
	}
	tree, failures, err := walkCompanyTree(ctx, c, maxCompanies)
	if err != nil || len(failures) > 0 {
		return tree, failures, err
	}
	if err := saveCompanyTree(ctx, db, tree); err != nil {
		return tree, failures, err
	}
	return tree, failures, nil
}

type fleetCompanyResult struct {
	Company   gzCompany
	Endpoints []gzEndpoint
	Err       error
}

// collectFleet fans getEndpointsList out over every company in the tree.
// Under live dogfood it stops after three companies and one page each.
func collectFleet(ctx context.Context, c *client.Client, tree []gzCompany, maxPages int) ([]fleetCompanyResult, int) {
	// ponytail: sequential; the client limiter paces calls.
	limit := len(tree)
	if cliutil.IsDogfoodEnv() {
		maxPages = 1
		if limit > 3 {
			limit = 3
		}
	}
	results := make([]fleetCompanyResult, 0, limit)
	pages := 0
	for _, co := range tree[:limit] {
		eps, n, err := listCompanyEndpoints(ctx, c, co.ID, maxPages)
		pages += n
		results = append(results, fleetCompanyResult{Company: co, Endpoints: eps, Err: err})
	}
	return dedupeEndpoints(results), pages
}

// dedupeEndpoints keeps each endpoint id only under the deepest company that
// returned it: a partner's recursive endpoint list may include its children's
// machines. The tree is breadth-first, so later companies are deeper.
func dedupeEndpoints(results []fleetCompanyResult) []fleetCompanyResult {
	seen := map[string]bool{}
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].Err != nil {
			continue // every consumer skips failed results; their partial lists must not win ids
		}
		kept := results[i].Endpoints[:0:0]
		for _, e := range results[i].Endpoints {
			if e.ID == "" || !seen[e.ID] {
				seen[e.ID] = true
				kept = append(kept, e)
			}
		}
		results[i].Endpoints = kept
	}
	return results
}

// saveFleetSnapshot records one run. Companies whose fetch failed are stored
// with ok=0 so `fleet changes` never reads their missing endpoints as removals;
// treeComplete=false (failed tree calls, --max-companies or a dogfood cap) stops
// it from reading absent companies as removed.
func saveFleetSnapshot(ctx context.Context, db *sql.DB, results []fleetCompanyResult, treeComplete bool) (int64, time.Time, error) {
	takenAt := time.Now().UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, takenAt, err
	}
	defer func() { _ = tx.Rollback() }()
	endpoints, failures := 0, 0
	for _, r := range results {
		if r.Err != nil {
			failures++
		} else {
			endpoints += len(r.Endpoints)
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO gz_fleet_runs (taken_at, companies, endpoints, failures, tree_complete) VALUES (?, ?, ?, ?, ?)`,
		takenAt.Format(time.RFC3339), len(results), endpoints, failures, boolInt(treeComplete))
	if err != nil {
		return 0, takenAt, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, takenAt, err
	}
	for _, r := range results {
		ok := 1
		if r.Err != nil {
			ok = 0
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO gz_fleet_companies (run_id, company_id, company_name, ok) VALUES (?, ?, ?, ?)`,
			runID, r.Company.ID, r.Company.Name, ok); err != nil {
			return 0, takenAt, err
		}
		if r.Err != nil {
			continue
		}
		for _, e := range r.Endpoints {
			macs, _ := json.Marshal(e.MACs)
			var pid, pname, scan string
			pApplied := -1
			if e.Policy != nil {
				pid, pname = e.Policy.ID, e.Policy.Name
				pApplied = boolInt(e.Policy.Applied)
			}
			if e.LastScan != nil {
				scan = e.LastScan.Date
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO gz_fleet_endpoints
				(run_id, company_id, company_name, endpoint_id, name, fqdn, ip, macs, label, is_managed, is_isolated,
				 policy_id, policy_name, policy_applied, product_outdated, last_scan, machine_type, os)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				runID, r.Company.ID, r.Company.Name, e.ID, e.Name, e.FQDN, e.IP, string(macs), e.Label,
				boolInt(e.IsManaged), boolInt(e.IsIsolated), pid, pname, pApplied, boolInt(e.ProductOutdated), scan, e.MachineType, e.OS); err != nil {
				return 0, takenAt, err
			}
		}
	}
	return runID, takenAt, tx.Commit()
}

type snapEndpoint struct {
	CompanyID     string   `json:"company_id"`
	CompanyName   string   `json:"company_name"`
	EndpointID    string   `json:"endpoint_id"`
	Name          string   `json:"name"`
	FQDN          string   `json:"fqdn,omitempty"`
	IP            string   `json:"ip,omitempty"`
	MACs          []string `json:"macs,omitempty"`
	Label         string   `json:"label,omitempty"`
	IsManaged     bool     `json:"is_managed"`
	IsIsolated    bool     `json:"is_isolated"`
	PolicyName    string   `json:"policy_name,omitempty"`
	PolicyApplied *bool    `json:"policy_applied,omitempty"`
	Outdated      bool     `json:"product_outdated"`
	LastScan      string   `json:"last_scan,omitempty"`
}

type snapRun struct {
	RunID        int64     `json:"run_id"`
	TakenAt      time.Time `json:"taken_at"`
	TreeComplete bool      `json:"tree_complete"`
}

// listRuns returns snapshot runs newest first.
func listRuns(ctx context.Context, db *sql.DB) ([]snapRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT run_id, taken_at, tree_complete FROM gz_fleet_runs ORDER BY run_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []snapRun
	for rows.Next() {
		var r snapRun
		var at string
		var complete int
		if err := rows.Scan(&r.RunID, &at, &complete); err != nil {
			return nil, err
		}
		r.TreeComplete = complete == 1
		r.TakenAt, _ = time.Parse(time.RFC3339, at)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func loadRunEndpoints(ctx context.Context, db *sql.DB, runID int64) ([]snapEndpoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT company_id, company_name, endpoint_id, name, fqdn, ip, macs, label,
		is_managed, is_isolated, policy_name, policy_applied, product_outdated, last_scan
		FROM gz_fleet_endpoints WHERE run_id = ? ORDER BY company_name, name`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]snapEndpoint, 0)
	for rows.Next() {
		var e snapEndpoint
		var macs string
		var managed, isolated, applied, outdated int
		if err := rows.Scan(&e.CompanyID, &e.CompanyName, &e.EndpointID, &e.Name, &e.FQDN, &e.IP, &macs, &e.Label,
			&managed, &isolated, &e.PolicyName, &applied, &outdated, &e.LastScan); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(macs), &e.MACs)
		e.IsManaged, e.IsIsolated, e.Outdated = managed == 1, isolated == 1, outdated == 1
		if applied >= 0 {
			v := applied == 1
			e.PolicyApplied = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func loadRunCompanies(ctx context.Context, db *sql.DB, runID int64) (map[string]idName, map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT company_id, company_name, ok FROM gz_fleet_companies WHERE run_id = ?`, runID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	all := map[string]idName{}
	ok := map[string]bool{}
	for rows.Next() {
		var id, name string
		var good int
		if err := rows.Scan(&id, &name, &good); err != nil {
			return nil, nil, err
		}
		all[id] = idName{ID: id, Name: name}
		ok[id] = good == 1
	}
	return all, ok, rows.Err()
}

func snapFromLive(co gzCompany, e gzEndpoint) snapEndpoint {
	s := snapEndpoint{CompanyID: co.ID, CompanyName: co.Name, EndpointID: e.ID, Name: e.Name, FQDN: e.FQDN, IP: e.IP,
		MACs: e.MACs, Label: e.Label, IsManaged: e.IsManaged, IsIsolated: e.IsIsolated, Outdated: e.ProductOutdated}
	if e.Policy != nil {
		s.PolicyName = e.Policy.Name
		v := e.Policy.Applied
		s.PolicyApplied = &v
	}
	if e.LastScan != nil {
		s.LastScan = e.LastScan.Date
	}
	return s
}

// --- small helpers --------------------------------------------------------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// parseGZTime accepts the timestamp shapes GravityZone returns
// ("2026-08-22T13:46:59", RFC 3339, or a space separator).
func parseGZTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseSpan accepts Go durations plus day and week suffixes (7d, 2w).
func parseSpan(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if strings.HasSuffix(s, suffix) {
			n, err := strconv.Atoi(strings.TrimSuffix(s, suffix))
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid duration %q", s)
			}
			return time.Duration(n) * unit, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use 24h, 7d or 2w)", s)
	}
	return d, nil
}

var macShapeRE = regexp.MustCompile(`^(?:(?:[0-9a-f]{2}:){5}[0-9a-f]{2}|(?:[0-9a-f]{2}-){5}[0-9a-f]{2}|[0-9a-f]{4}\.[0-9a-f]{4}\.[0-9a-f]{4}|[0-9a-f]{12})$`)

// normalizeMAC returns a MAC address as 12 lowercase hex digits, or "" when s is
// not MAC-shaped (aa:bb:..., aa-bb-..., aabb.ccdd.eeff or 12 bare hex digits).
func normalizeMAC(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if !macShapeRE.MatchString(s) {
		return ""
	}
	return strings.NewReplacer(":", "", "-", "", ".", "").Replace(s)
}

// sharedMACs are adapter addresses that are identical on every host carrying the
// adapter (Cisco AnyConnect, Npcap loopback, VMware VMnet1/VMnet8).
var sharedMACs = map[string]bool{"00059a3c7a00": true, "02004c4f4f50": true, "005056c00001": true, "005056c00008": true}

// identifyingMAC reports whether a normalized MAC can identify one machine:
// not all-zero, broadcast/multicast, locally administered or a known shared one.
func identifyingMAC(m string) bool {
	if m == "" || m == "000000000000" || sharedMACs[m] {
		return false
	}
	first, err := strconv.ParseUint(m[:2], 16, 8)
	return err == nil && first&0x03 == 0
}

// shortHost lowercases a hostname and drops any domain suffix.
func shortHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '.'); i > 0 && net4(s) == "" {
		s = s[:i]
	}
	return s
}

// net4 returns s when it is a dotted IPv4 literal, else "".
func net4(s string) string {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return ""
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return ""
		}
	}
	return s
}

func failureList(results []fleetCompanyResult, extra []fetchFailure) []fetchFailure {
	out := make([]fetchFailure, 0, len(extra))
	out = append(out, extra...)
	for _, r := range results {
		if r.Err != nil {
			out = append(out, fetchFailure{Scope: "company " + r.Company.Name + " (" + r.Company.ID + ")", Error: r.Err.Error()})
		}
	}
	return out
}

type inventoryResult struct {
	Endpoints []snapEndpoint
	Source    string // "snapshot" or "live"
	TakenAt   string
	Stale     bool
	Failures  []fetchFailure
	Companies int
	Complete  bool   // every company in a complete tree was fetched
	Note      string // e.g. a snapshot that could not be saved
}

// fleetInventory answers from the latest local snapshot or a live fan-out:
// --data-source local reads the snapshot only, live always fans out (and saves
// a snapshot), auto uses the snapshot when one exists and fans out otherwise.
func fleetInventory(ctx context.Context, c func() (*client.Client, error), db *sql.DB, mode string, maxAge time.Duration, maxPages int) (inventoryResult, error) {
	if mode != "live" {
		runs, err := listRuns(ctx, db)
		if err != nil {
			return inventoryResult{}, err
		}
		if len(runs) > 0 {
			eps, err := loadRunEndpoints(ctx, db, runs[0].RunID)
			if err != nil {
				return inventoryResult{}, err
			}
			cos, ok, err := loadRunCompanies(ctx, db, runs[0].RunID)
			if err != nil {
				return inventoryResult{}, err
			}
			out := inventoryResult{Endpoints: eps, Source: "snapshot", TakenAt: runs[0].TakenAt.Format(time.RFC3339),
				Stale: maxAge > 0 && time.Since(runs[0].TakenAt) > maxAge, Companies: len(cos), Complete: runs[0].TreeComplete,
				Failures: make([]fetchFailure, 0)}
			for id, co := range cos {
				if !ok[id] {
					out.Complete = false
					out.Failures = append(out.Failures, fetchFailure{Scope: "company " + co.Name + " (" + id + ")", Error: "endpoint fetch failed when this snapshot was taken"})
				}
			}
			return out, nil
		}
		if mode == "local" {
			return inventoryResult{Source: "snapshot"}, nil
		}
	}
	cl, err := c()
	if err != nil {
		return inventoryResult{}, err
	}
	tree, treeFailures, err := companyTreeFor(ctx, cl, db, maxAge, 5000)
	if err != nil && len(tree) == 0 {
		return inventoryResult{}, err
	}
	treeComplete := err == nil && len(treeFailures) == 0
	limit := len(tree)
	if dogfoodLimit(&limit) {
		maxPages, treeComplete = 1, false
	}
	var results []fleetCompanyResult
	for _, co := range tree[:limit] {
		eps, _, ferr := listCompanyEndpoints(ctx, cl, co.ID, maxPages)
		results = append(results, fleetCompanyResult{Company: co, Endpoints: eps, Err: ferr})
	}
	results = dedupeEndpoints(results)
	out := inventoryResult{Source: "live", TakenAt: time.Now().UTC().Format(time.RFC3339), Failures: failureList(results, treeFailures),
		Companies: len(results), Complete: treeComplete && countFailed(results) == 0}
	if err != nil {
		out.Failures = append(out.Failures, fetchFailure{Scope: "company tree", Error: err.Error()})
	}
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		for _, e := range r.Endpoints {
			out.Endpoints = append(out.Endpoints, snapFromLive(r.Company, e))
		}
	}
	// A full fan-out is a fleet snapshot; keep it for fleet changes.
	if _, _, serr := saveFleetSnapshot(ctx, db, results, treeComplete); serr != nil {
		out.Note = "snapshot not saved: " + serr.Error()
	}
	return out, nil
}

// dogfoodLimit caps a company fan-out under live dogfood and reports whether it did.
func dogfoodLimit(limit *int) bool {
	if cliutil.IsDogfoodEnv() && *limit > 3 {
		*limit = 3
		return true
	}
	return false
}

// maxInputFileBytes bounds --devices / --agreements exports.
const maxInputFileBytes = 50 << 20

// readBoundedFile reads a regular file of at most maxInputFileBytes, refusing
// devices, FIFOs and other special files that could block or never end.
func readBoundedFile(flag, path string) ([]byte, error) {
	// Lstat first so a FIFO or device is rejected without the blocking open.
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: file not found: %s", flag, path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read %s", flag, path)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %s is not a regular file", flag, path)
	}
	f, err := openNoBlock(path)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read %s", flag, path)
	}
	defer f.Close()
	// The opened descriptor must still be that regular file, and the read is bounded.
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return nil, fmt.Errorf("%s: %s changed while being opened", flag, path)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxInputFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read %s", flag, path)
	}
	if len(b) > maxInputFileBytes {
		return nil, fmt.Errorf("%s: %s is larger than %d MiB", flag, path, maxInputFileBytes>>20)
	}
	return decodeUTF16BOM(b), nil
}

// decodeUTF16BOM converts a UTF-16 file (Windows PowerShell 5.1 writes `>`
// redirects and Out-File as UTF-16LE with a BOM) to UTF-8; anything else is
// returned unchanged.
func decodeUTF16BOM(b []byte) []byte {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		order = binary.LittleEndian
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		order = binary.BigEndian
	default:
		return b
	}
	b = b[2:]
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[2*i:])
	}
	return []byte(string(utf16.Decode(units)))
}

// fleetUsageErr reports bad input as exit 2 and, for --json/--agent callers,
// also writes the JSON error object the generated commands emit.
func fleetUsageErr(cmd *cobra.Command, flags *rootFlags, err error) error {
	writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, err, 2)
	return usageErr(err)
}

// humanNote prints a command's note to stderr in table mode so warnings stay
// visible next to result rows; machine output carries it in the JSON note field.
func humanNote(cmd *cobra.Command, note string) {
	// Notes can carry tenant-controlled company names: scrub terminal escapes.
	if note = strings.TrimSpace(cliutil.ScrubTerminal(note)); note != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "note: "+note)
	}
}

// inventoryAgentSource is the agent envelope's meta.source for an answer
// built from fleetInventory: a saved snapshot is local data, not a live read
// (handfixes.json: fleet-snapshot-answers-report-local).
func inventoryAgentSource(inv inventoryResult) string {
	if inv.Source == "snapshot" {
		return "local"
	}
	return "live"
}
