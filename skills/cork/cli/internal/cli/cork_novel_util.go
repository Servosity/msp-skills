// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored shared helpers for Cork novel (transcendence) commands.
// Kept in its own file so `generate --force` preserves it.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cork-pp-cli/internal/client"
	"cork-pp-cli/internal/cliutil"
	"cork-pp-cli/internal/store"
)

// corkEnvelope is the uniform paginated envelope every Cork collection returns.
type corkEnvelope struct {
	Items      []json.RawMessage `json:"items"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalItems int `json:"total_items"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

// corkDefaultScanPages bounds how many pages a scan-and-filter command will
// read before returning partial results. Cork publishes no rate limits, so the
// cap is deliberately conservative.
const corkDefaultScanPages = 5

// corkPageSize is the per-request page size used by novel commands.
const corkPageSize = 100

// corkFetchPages walks a Cork collection endpoint page by page.
//
// It returns the accumulated raw items, how many pages were actually read, and
// whether the scan stopped because it hit maxPages rather than because the
// collection ran out. Callers surface both so an empty result can be
// distinguished from a truncated one.
func corkFetchPages(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) (items []json.RawMessage, pagesRead int, capHit bool, err error) {
	if maxPages <= 0 {
		maxPages = corkDefaultScanPages
	}
	if cliutil.IsDogfoodEnv() && maxPages > 1 {
		// Live-dogfood runs under a flat per-command timeout; one page is enough
		// to prove the path works without risking the budget.
		maxPages = 1
	}
	items = make([]json.RawMessage, 0)
	for page := 1; page <= maxPages; page++ {
		p := make(map[string]string, len(params)+2)
		for k, v := range params {
			if v != "" {
				p[k] = v
			}
		}
		p["page"] = strconv.Itoa(page)
		p["page_size"] = strconv.Itoa(corkPageSize)

		raw, getErr := c.Get(ctx, path, p)
		if getErr != nil {
			return items, page - 1, false, getErr
		}
		// Most Cork collections return {items, pagination}, but the unpaginated
		// ones (notably /compliance/event-types, which declares no page params)
		// return a bare array. Accept both rather than assuming the envelope.
		var bare []json.RawMessage
		if json.Unmarshal(raw, &bare) == nil {
			items = append(items, bare...)
			return items, page, false, nil
		}
		var env corkEnvelope
		if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
			return items, page - 1, false, fmt.Errorf("parsing %s page %d: %w", path, page, jsonErr)
		}
		items = append(items, env.Items...)
		pagesRead = page
		if len(env.Items) == 0 {
			return items, pagesRead, false, nil
		}
		if env.Pagination.TotalPages > 0 && page >= env.Pagination.TotalPages {
			return items, pagesRead, false, nil
		}
		if env.Pagination.TotalPages == 0 && len(env.Items) < corkPageSize {
			return items, pagesRead, false, nil
		}
	}
	return items, pagesRead, true, nil
}

// corkClient is the local-store projection of a Cork client. score_history here
// is the array embedded in the /clients payload (the 10 most recent points),
// which is the only durable score history available locally: the generator
// cannot key the dependent score-history resource (ClientScoreHistory has no id
// field), so those rows collapse to one per client in the resources table.
type corkClient struct {
	UUID           string                 `json:"uuid"`
	Name           string                 `json:"name"`
	Hidden         bool                   `json:"hidden"`
	WarrantyStatus string                 `json:"warranty_status"`
	ScoreHistory   []corkScorePoint       `json:"score_history"`
	Tenants        []corkAssociatedTenant `json:"associated_tenants"`
}

// corkAssociatedTenant is a tenant on a client. `integration` is a full nested
// integration object, not an identifier string — decoding it as a string makes
// json.Unmarshal fail for the whole client, which silently drops every row.
type corkAssociatedTenant struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Integration struct {
		UUID             string `json:"uuid"`
		DisplayName      string `json:"display_name"`
		ConnectionStatus string `json:"connection_status"`
		LastSyncedAt     string `json:"last_synced_at"`
		Vendor           struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"vendor"`
	} `json:"integration"`
}

// corkClientMinimal is the decode fallback. If the full client shape ever fails
// to unmarshal because a nested field changed type upstream, the core identity
// and score fields still load, so a schema surprise degrades a command's extra
// columns instead of silently returning zero clients.
type corkClientMinimal struct {
	UUID           string           `json:"uuid"`
	Name           string           `json:"name"`
	Hidden         bool             `json:"hidden"`
	WarrantyStatus string           `json:"warranty_status"`
	ScoreHistory   []corkScorePoint `json:"score_history"`
}

type corkScorePoint struct {
	Score     int    `json:"score"`
	CreatedAt string `json:"created_at"`
}

// corkOpenStore opens the local mirror read-only, returning ok=false (after
// writing a human hint to stderr) when the mirror does not exist yet.
//
// OpenReadOnlyContext is deliberate: these commands are annotated
// mcp:read-only, and the read-write open would MkdirAll, switch the journal to
// WAL, and run migrations as a side effect of a "read-only" tool call.
func corkOpenStore(ctx context.Context, dbPath string, errOut, out interface{ Write([]byte) (int, error) }, syncResources string) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("cork-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(errOut, "no local mirror at %s\nrun: cork-cli sync --resources %s --db %s\n", dbPath, syncResources, dbPath)
		return nil, false, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	return db, true, nil
}

// corkPathSeg escapes a caller-supplied value for use as a single URL path
// segment. Raw concatenation lets a value containing "?" or "/" retarget the
// request at a different endpoint or inject query parameters.
func corkPathSeg(s string) string { return url.PathEscape(s) }

// corkDecodeClients decodes a live /clients page set with the same
// degrade-don't-drop contract corkLoadClients uses for the mirror: a record
// that fails the full shape falls back to the minimal shape, and the count of
// wholly undecodable records is returned so callers can refuse to assert an
// empty result they did not actually establish.
func corkDecodeClients(raw []json.RawMessage) (clients []corkClient, undecodable int) {
	clients = make([]corkClient, 0, len(raw))
	for _, r := range raw {
		var cc corkClient
		if err := json.Unmarshal(r, &cc); err == nil && cc.UUID != "" {
			clients = append(clients, cc)
			continue
		}
		var min corkClientMinimal
		if json.Unmarshal(r, &min) == nil && min.UUID != "" {
			clients = append(clients, corkClient{
				UUID:           min.UUID,
				Name:           min.Name,
				Hidden:         min.Hidden,
				WarrantyStatus: min.WarrantyStatus,
				ScoreHistory:   min.ScoreHistory,
			})
			continue
		}
		undecodable++
	}
	return clients, undecodable
}

// corkDecodeFailure builds the error returned when every record in a non-empty
// response failed to decode. Reporting "nothing found" in that situation turns
// a read failure into a false clean result, which for a risk tool is worse than
// an outright error.
func corkDecodeFailure(what string, undecodable int) error {
	return apiErr(fmt.Errorf("could not decode any of the %d %s returned by Cork; refusing to report an empty result that was not actually established", undecodable, what))
}

// corkNewestOldest returns the newest and oldest parseable score points in a
// client's embedded history, plus how many parsed.
func corkNewestOldest(points []corkScorePoint) (newest, oldest *corkScorePoint, newestT, oldestT time.Time, parsed int) {
	for i := range points {
		t, ok := corkParseTime(points[i].CreatedAt)
		if !ok {
			continue
		}
		parsed++
		if newest == nil || t.After(newestT) {
			newest = &points[i]
			newestT = t
		}
		if oldest == nil || t.Before(oldestT) {
			oldest = &points[i]
			oldestT = t
		}
	}
	return newest, oldest, newestT, oldestT, parsed
}

// corkLoadClients reads every locally mirrored client, drain-first: the parent
// rows are fully scanned and the *sql.Rows closed before any caller runs
// follow-up queries. SQLite uses a single connection, so nested queries against
// an open result set are not safe.
func corkLoadClients(ctx context.Context, db *store.Store) ([]corkClient, error) {
	rows, err := db.DB().QueryContext(ctx, `
		SELECT data FROM resources
		WHERE resource_type IN ('clients', 'client')`)
	if err != nil {
		return nil, fmt.Errorf("query clients: %w", err)
	}
	raws := make([]string, 0)
	for rows.Next() {
		var data sql.NullString
		if scanErr := rows.Scan(&data); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan client: %w", scanErr)
		}
		if data.Valid {
			raws = append(raws, data.String)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate clients: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close clients rows: %w", err)
	}

	out := make([]corkClient, 0, len(raws))
	degraded := 0
	for _, r := range raws {
		var cc corkClient
		if err := json.Unmarshal([]byte(r), &cc); err != nil {
			// Fall back to the minimal shape rather than dropping the client.
			// A silently-skipped row here would turn a wrong answer into a
			// confident empty one, which is worse than losing tenant detail.
			var min corkClientMinimal
			if json.Unmarshal([]byte(r), &min) != nil || min.UUID == "" {
				continue
			}
			degraded++
			out = append(out, corkClient{
				UUID:           min.UUID,
				Name:           min.Name,
				Hidden:         min.Hidden,
				WarrantyStatus: min.WarrantyStatus,
				ScoreHistory:   min.ScoreHistory,
			})
			continue
		}
		if cc.UUID == "" {
			continue
		}
		out = append(out, cc)
	}
	if degraded > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d client record(s) decoded in reduced form; tenant-derived columns may be incomplete\n", degraded)
	}
	return out, nil
}

// corkClientNames maps client uuid -> display name for resolving the bare UUIDs
// that vulnerability and compliance payloads carry.
func corkClientNames(clients []corkClient) map[string]string {
	m := make(map[string]string, len(clients))
	for _, c := range clients {
		m[c.UUID] = c.Name
	}
	return m
}

// corkDeviceNames maps device uuid -> device name from the locally mirrored
// client devices.
func corkDeviceNames(ctx context.Context, db *store.Store) (map[string]string, error) {
	rows, err := db.DB().QueryContext(ctx, `
		SELECT id, data FROM resources
		WHERE resource_type IN ('clients_devices', 'devices', 'integrations_devices')`)
	if err != nil {
		return nil, fmt.Errorf("query devices: %w", err)
	}
	type pair struct{ id, data string }
	pairs := make([]pair, 0)
	for rows.Next() {
		var id, data sql.NullString
		if scanErr := rows.Scan(&id, &data); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan device: %w", scanErr)
		}
		pairs = append(pairs, pair{id.String, data.String})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close device rows: %w", err)
	}

	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		var d struct {
			UUID string `json:"uuid"`
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(p.data), &d) != nil {
			continue
		}
		if d.UUID != "" && d.Name != "" {
			m[d.UUID] = d.Name
		}
	}
	return m, nil
}

// corkResolve returns the display name for a uuid, falling back to a shortened
// uuid so output is never an empty cell.
func corkResolve(m map[string]string, uuid string) string {
	if uuid == "" {
		return ""
	}
	if n, ok := m[uuid]; ok && n != "" {
		return n
	}
	if len(uuid) > 8 {
		return uuid[:8]
	}
	return uuid
}

// corkParseTime accepts the RFC3339 timestamps Cork returns, tolerating the
// fractional-second precision variations seen across endpoints.
func corkParseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// corkSince parses a --since value using the loose duration parser so 7d/1w
// shorthand works, matching the framework's own `sync --since`.
// corkWindowLabel is what a human note should call the window: the literal the
// user typed, so "--since 90d" does not read back as "2160h0m0s". An empty
// --since is a supported input (corkSince falls back to the default), so fall
// back with it rather than printing nothing.
func corkWindowLabel(raw string, resolved time.Duration) string {
	if strings.TrimSpace(raw) == "" {
		return resolved.String()
	}
	return raw
}

func corkSince(raw string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	d, err := cliutil.ParseDurationLoose(raw)
	if err != nil {
		return 0, usageErr(fmt.Errorf("invalid --since %q: use a duration like 7d, 24h, or 1w", raw))
	}
	if d <= 0 {
		return 0, usageErr(fmt.Errorf("--since must be positive, got %q", raw))
	}
	return d, nil
}

// corkPriorityRank orders Cork's SSVC-style priority tiers for local sorting.
// Lower is more urgent. Unknown tiers sort last.
func corkPriorityRank(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "critical":
		return 0
	case "accelerated":
		return 1
	case "routine":
		return 2
	default:
		return 3
	}
}

// corkSortStable sorts by the supplied less function, keeping ties in input
// order so repeated runs against unchanged data produce identical output.
func corkSortStable[T any](s []T, less func(a, b T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}

// pickTopCVEForTest mirrors the top-CVE selection rule used by
// `vulnerabilities triage`: KEV outranks non-KEV outright, and only within the
// same KEV class do EPSS then CVSS decide. Exported to the package's tests so
// the ordering contract is pinned independently of the aggregation loop.
func pickTopCVEForTest(cves []corkCVE) (topCVE string, topIsKEV bool) {
	var topEPSS, topCVSS float64
	for _, cve := range cves {
		replace := false
		switch {
		case topCVE == "":
			replace = true
		case cve.IsKEV && !topIsKEV:
			replace = true
		case cve.IsKEV == topIsKEV:
			if cve.EPSS > topEPSS || (cve.EPSS == topEPSS && cve.CVSS > topCVSS) {
				replace = true
			}
		}
		if replace {
			topCVE, topIsKEV, topEPSS, topCVSS = cve.CVEID, cve.IsKEV, cve.EPSS, cve.CVSS
		}
	}
	return topCVE, topIsKEV
}
