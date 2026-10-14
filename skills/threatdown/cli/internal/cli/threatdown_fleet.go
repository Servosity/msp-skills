// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared plumbing for the cross-site OneView commands (fleet, threats,
// respond, seats, customer): resolve every site's Nebula account_id, page
// the POST search endpoints by body next_cursor, and read the fleet-synced
// local store.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/client"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

const (
	tdBinary           = "threatdown-cli"
	tdFleetSyncCommand = "threatdown-cli fleet sync"
	tdSearchPageSize   = 2000 // documented maximum for endpoint/detection/job search
	tdDefaultMaxPages  = 50
)

// Local store resource types written by `fleet sync`. endpoint_jobs matches the
// generated endpoint-jobs resource table.
const (
	tdResSites      = "sites"
	tdResEndpoints  = "endpoints"
	tdResDetections = "detections"
	tdResJobs       = "endpoint_jobs"
)

// fetchSites lists every managed site (GET /v1/sites). OneView answers a
// request without a bearer token with HTTP 400 "headers must have required
// property 'authorization'"; that is reported as an auth error (exit 4).
func fetchSites(ctx context.Context, c *client.Client) ([]fleet.Site, error) {
	data, err := c.Get(ctx, "/v1/sites", nil)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || strings.Contains(apiErr.Body, "property 'authorization'")) {
			return nil, authErr(fmt.Errorf("not authenticated with OneView: set THREATDOWN_CLIENT_ID and THREATDOWN_CLIENT_SECRET, then run '%s doctor' (%w)", tdBinary, err))
		}
		return nil, err
	}
	return fleet.ParseSites(data)
}

// scopeSites applies --site. A filter that matches no site is an error that
// lists real names, so a typo never reads as an all-clear.
func scopeSites(sites []fleet.Site, filter string) ([]fleet.Site, error) {
	out := filterSitesByName(sites, filter)
	if strings.TrimSpace(filter) == "" || len(out) > 0 {
		return out, nil
	}
	if len(sites) == 0 {
		return nil, notFoundErr(fmt.Errorf("--site %q matches no site: there are no sites to match (locally, run %s first)", filter, tdFleetSyncCommand))
	}
	names := make([]string, 0, len(sites))
	for _, s := range sites {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	if len(names) > 10 {
		names = append(names[:10], "...")
	}
	return nil, notFoundErr(fmt.Errorf("--site %q matches no site; sites include: %s", filter, strings.Join(names, ", ")))
}

// accountlessNote names the sites a report leaves out because they have no
// account_id yet (no subscription): OneView holds no endpoints, detections or
// jobs for them. Without it such a site vanishes from a per-site report, and a
// --site filter that matches only those sites reads as an all-clear.
func accountlessNote(sites []fleet.Site) string {
	names := make([]string, 0)
	for _, s := range sites {
		if s.AccountID == "" {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	n := len(names)
	sort.Strings(names)
	if len(names) > 10 {
		names = append(names[:10], "...")
	}
	return fmt.Sprintf("%d site(s) have no account_id yet (no subscription), so OneView holds no endpoints for them and they are not listed: %s", n, strings.Join(names, ", "))
}

// joinNotes joins the non-empty notes with "; ".
func joinNotes(notes ...string) string {
	kept := make([]string, 0, len(notes))
	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

// stopFallback reports errors that a local fallback must not hide: usage,
// not-found, auth and similar typed CLI errors are about the request or the
// credentials, not a passing API outage. API (5) and rate-limit (7) errors
// fall back.
func stopFallback(err error) bool {
	var typed *cliError
	return errors.As(classifyAPIErrorOnly(err), &typed) && typed.code != 5 && typed.code != 7
}

// printColumns renders rows as an aligned table with the given columns, in
// order. Lists join with ", ", booleans print yes/no.
func printColumns(w io.Writer, rows any, cols ...string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = strings.ToUpper(strings.ReplaceAll(c, "_", " "))
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range toRows(rows) {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = cellString(r[c])
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
}

func cellString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, cellString(p))
		}
		return strings.Join(parts, ", ")
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// searchInfo describes how a paged search ended.
type searchInfo struct {
	Pages     int
	Truncated bool // stopped at maxPages with a cursor still pending
	// Complete means the server ended the cursor chain explicitly (an empty
	// next_cursor). Only a Complete pull may delete cached rows it did not see.
	Complete bool
}

// searchPages POSTs body to a OneView search endpoint and follows the body
// next_cursor until the server returns an empty one or maxPages is reached.
// itemsKey names the array in the response envelope (endpoints, detections,
// jobs). Every page must carry that array; a cursor must be a string and must
// advance. An empty page that still has a cursor is followed, not treated as
// the end.
func searchPages(ctx context.Context, c *client.Client, path string, body map[string]any, itemsKey string, maxPages int) ([]json.RawMessage, searchInfo, error) {
	if maxPages <= 0 {
		maxPages = tdDefaultMaxPages
	}
	req := make(map[string]any, len(body)+2)
	for k, v := range body {
		req[k] = v
	}
	if _, ok := req["page_size"]; !ok {
		req["page_size"] = tdSearchPageSize
	}
	items := make([]json.RawMessage, 0)
	var info searchInfo
	prev := ""
	for page := 1; page <= maxPages; page++ {
		data, _, err := c.Post(ctx, path, req)
		if err != nil {
			return items, info, err
		}
		info.Pages = page
		var env map[string]json.RawMessage
		if err := json.Unmarshal(data, &env); err != nil {
			return items, info, fmt.Errorf("parsing %s response: %w", path, err)
		}
		// A response without the collection is not an empty result (dry-run
		// and verify modes return a synthetic envelope); treating it as one
		// would let fleet sync delete cached rows.
		raw, ok := env[itemsKey]
		if !ok || string(raw) == "null" {
			return items, info, fmt.Errorf("%s response has no %q array", path, itemsKey)
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(raw, &batch); err != nil {
			return items, info, fmt.Errorf("parsing %s.%s: %w", path, itemsKey, err)
		}
		items = append(items, batch...)
		rawNext, hasNext := env["next_cursor"]
		if !hasNext || string(rawNext) == "null" {
			// End of data, but not confirmed by the server: readers may use the
			// rows, reconciliation may not.
			return items, info, nil
		}
		var next string
		if err := json.Unmarshal(rawNext, &next); err != nil {
			return items, info, fmt.Errorf("%s returned a non-string next_cursor", path)
		}
		if next == "" {
			info.Complete = true
			return items, info, nil
		}
		if next == prev {
			return items, info, fmt.Errorf("%s returned the same next_cursor twice; stopping", path)
		}
		prev = next
		req["next_cursor"] = next
	}
	info.Truncated = true
	return items, info, nil
}

// parseEndpoints flattens raw endpoint records. bad counts records that could
// not be read; callers that delete or certify state must not proceed when it
// is non-zero.
func parseEndpoints(raws []json.RawMessage) (eps []fleet.Endpoint, bad int) {
	eps = make([]fleet.Endpoint, 0, len(raws))
	for _, r := range raws {
		if e, err := fleet.ParseEndpoint(r); err == nil {
			eps = append(eps, e)
		} else {
			bad++
		}
	}
	return eps, bad
}

func parseDetections(raws []json.RawMessage) []fleet.Detection {
	out := make([]fleet.Detection, 0, len(raws))
	for _, r := range raws {
		var d fleet.Detection
		if err := json.Unmarshal(r, &d); err == nil && d.ID != "" {
			out = append(out, d)
		}
	}
	return out
}

// tdDBPath resolves the local store path (the same file `sync` writes).
func tdDBPath(dbPath string) string {
	if dbPath != "" {
		return dbPath
	}
	return defaultDBPath(tdBinary)
}

// openFleetStore opens the local store read-only. ok=false means no mirror
// exists yet; a hint naming the sync command has been printed to stderr.
func openFleetStore(cmd *cobra.Command, dbPath string) (*store.Store, bool, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: %s --db %s\n", dbPath, tdFleetSyncCommand, dbPath)
		return nil, false, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	return db, true, nil
}

// fleetHint mirrors hintIfUnsynced/hintIfStale for resources only `fleet sync`
// fills (the generic sync command cannot page POST searches across sites).
// tdMaxAge is the staleness threshold for the fleet reports' local-copy
// hints: an explicit global --max-age (0 disables), else 24h, because
// fleet sync is a heavier pull than the 30m generic default assumes.
func tdMaxAge(cmd *cobra.Command) time.Duration {
	if f := cmd.Flag("max-age"); f != nil && f.Changed {
		if d, err := time.ParseDuration(f.Value.String()); err == nil {
			return d
		}
	}
	return 24 * time.Hour
}

// markSyncIncomplete flags a resource whose latest full fleet sync did not
// finish cleanly; the next complete sync (SaveSyncState) clears it.
func markSyncIncomplete(db *store.Store, resource string) {
	_, _ = db.DB().Exec(`UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = ?`, resource)
}

// incompleteSyncNote is non-empty when the latest full fleet sync of resource
// could not read or confirm every record, so local results may miss rows.
func incompleteSyncNote(db *store.Store, resource string) string {
	var complete sql.NullInt64
	if err := db.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ? AND last_synced_at IS NOT NULL`, resource).Scan(&complete); err != nil || !complete.Valid || complete.Int64 != 0 {
		return ""
	}
	return fmt.Sprintf("the latest %s did not get a complete copy of %s (see its note), so local results may be missing rows; run it again", tdFleetSyncCommand, resource)
}

func fleetHint(cmd *cobra.Command, db *store.Store, resource string, maxAge time.Duration) {
	state, err := readSyncHintState(db, resource)
	if err != nil {
		return
	}
	if n := incompleteSyncNote(db, resource); n != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s.\n", n)
	}
	if !state.hasState {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local %s yet. Run '%s' before trusting local results.\n", resource, tdFleetSyncCommand)
		return
	}
	if maxAge > 0 && time.Since(state.lastSynced) > maxAge {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: local %s are %s old, older than --max-age=%s. Run '%s' to refresh.\n", resource, syncHintRoundAge(time.Since(state.lastSynced)), maxAge, tdFleetSyncCommand)
	}
}

// fleetCoverage returns the start of the window the last complete fleet sync
// covered for detections or jobs (stored as the sync cursor).
func fleetCoverage(ctx context.Context, db *store.Store, resource string) (time.Time, bool) {
	var cursor sql.NullString
	if err := db.DB().QueryRowContext(ctx, `SELECT last_cursor FROM sync_state WHERE resource_type = ?`, resource).Scan(&cursor); err != nil || !cursor.Valid {
		return time.Time{}, false
	}
	return fleet.ParseTime(cursor.String)
}

// coverageNote explains when a local report asks for history older than the
// synced window.
func coverageNote(ctx context.Context, db *store.Store, resource string, since time.Time) string {
	start, ok := fleetCoverage(ctx, db, resource)
	if !ok || !since.Before(start.Add(-time.Minute)) {
		return ""
	}
	return fmt.Sprintf("local %s only cover %s onward; run %s --since <a longer window> for older history", resource, start.Format("2006-01-02"), tdFleetSyncCommand)
}

// loadLocal reads every stored record of one resource type. Drain-first: the
// rows are fully read and closed before the caller issues more queries.
func loadLocal(ctx context.Context, db *store.Store, resource string) ([]json.RawMessage, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resource)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", resource, err)
	}
	out := make([]json.RawMessage, 0)
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s: %w", resource, err)
		}
		if data.Valid {
			out = append(out, json.RawMessage(data.String))
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

func loadLocalSites(ctx context.Context, db *store.Store) ([]fleet.Site, error) {
	raws, err := loadLocal(ctx, db, tdResSites)
	if err != nil {
		return nil, err
	}
	sites := make([]fleet.Site, 0, len(raws))
	for _, r := range raws {
		var s fleet.Site
		if json.Unmarshal(r, &s) == nil && (s.ID != "" || s.AccountID != "") {
			sites = append(sites, s)
		}
	}
	return sites, nil
}

// loadLocalEndpoints reads the flattened endpoint records fleet sync stores.
func loadLocalEndpoints(ctx context.Context, db *store.Store) ([]fleet.Endpoint, error) {
	raws, err := loadLocal(ctx, db, tdResEndpoints)
	if err != nil {
		return nil, err
	}
	out := make([]fleet.Endpoint, 0, len(raws))
	for _, r := range raws {
		var e fleet.Endpoint
		if json.Unmarshal(r, &e) == nil && e.ID != "" {
			out = append(out, e)
		}
	}
	return out, nil
}

// resolveDataSource picks live or local for an auto-strategy command.
func resolveDataSource(flags *rootFlags) string {
	switch flags.dataSource {
	case "live", "local":
		return flags.dataSource
	}
	return "auto"
}

// filterSitesByName keeps sites whose name contains nameFilter (case-insensitive).
func filterSitesByName(sites []fleet.Site, nameFilter string) []fleet.Site {
	nameFilter = strings.ToLower(strings.TrimSpace(nameFilter))
	if nameFilter == "" {
		return sites
	}
	out := make([]fleet.Site, 0)
	for _, s := range sites {
		if strings.Contains(strings.ToLower(s.Name), nameFilter) {
			out = append(out, s)
		}
	}
	return out
}

// toRows converts typed rows into generic maps for the human table renderer.
func toRows(v any) []map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var rows []map[string]any
	_ = json.Unmarshal(data, &rows)
	return rows
}

// isoAgo formats now-d as the ISO timestamp OneView date filters expect.
func isoAgo(now time.Time, d time.Duration) string {
	return now.Add(-d).UTC().Format("2006-01-02T15:04:05.000Z")
}
