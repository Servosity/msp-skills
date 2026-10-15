// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared plumbing for the hand-written QuoteWerks commands (docs, items,
// products, requote, mirror and the local analytics). Generated endpoint
// commands do not use this file.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"quotewerks-pp-cli/internal/client"
	"quotewerks-pp-cli/internal/cliutil"
	"quotewerks-pp-cli/internal/qw"
	"quotewerks-pp-cli/internal/store"
)

const (
	qwCLIName       = "quotewerks-cli"
	qwTablesPath    = "/api/v1/qw/tables"
	qwMaxPageSize   = 1000
	qwResDocuments  = "documents"
	qwResLineItems  = "line_items"
	qwResProducts   = "products"
	qwResDocChanges = "doc_changes"
	// qwResItemCoverage has one row per document whose lines were read in full.
	qwResItemCoverage = "item_coverage"
	// qwResMirrorScope records what the last documents mirror covered.
	qwResMirrorScope = "mirror_scope"
)

// High-gravity columns returned by default; --all-fields returns every column.
var (
	qwHeaderFields = []string{"id", "DocNo", "DocType", "DocStatus", "DocDate", "DocName", "SoldToCompany", "SoldToContact", "SalesRep", "SubTotal", "TotalTax", "GrandTotal", "ProfitAmount", "QuoteValetStatusCode"}
	qwItemFields   = []string{"id", "DocRecGUID", "LineType", "Manufacturer", "ManufacturerPartNumber", "VendorPartNumber", "Description", "QtyBase", "UnitCost", "UnitPrice", "UnitList", "ExtendedCost", "ExtendedPrice"}
)

func qwTablePath(table string, rest ...string) string {
	parts := []string{qwTablesPath, url.PathEscape(table)}
	for _, r := range rest {
		parts = append(parts, url.PathEscape(r))
	}
	return strings.Join(parts, "/")
}

// errIgnoredFilters is returned when QuoteWerks reports meta.ignored_filters:
// the server dropped a filter (unknown or restricted column) and the rows are
// not filtered the way the caller asked.
type errIgnoredFilters struct {
	table   string
	filters []string
}

func (e *errIgnoredFilters) Error() string {
	return fmt.Sprintf("QuoteWerks ignored filter(s) %s on %s, so the rows would not be filtered as requested; check column names with '%s tables schema %s'",
		cliutil.ScrubTerminal(strings.Join(e.filters, ", ")), e.table, qwCLIName, e.table)
}

// qwAllPages makes qwSearch read every page; qwSafetyPages bounds that walk
// and turns a runaway into an error instead of a silently truncated result.
const (
	qwAllPages    = -1
	qwSafetyPages = 10000
)

type qwSearchOpts struct {
	filters  []map[string]any
	sort     string
	pageSize int
	maxPages int
	limit    int
}

// qwSearch pages POST /tables/{table}/search. complete is true only when every
// matching row was read: meta.total (when present) is the authority, and a
// server that caps the page size below the request (meta.per_page) keeps the
// walk going instead of looking like a short last page.
func qwSearch(ctx context.Context, c *client.Client, table string, o qwSearchOpts) (rows []qw.Resource, complete bool, err error) {
	size := o.pageSize
	if size <= 0 || size > qwMaxPageSize {
		size = qwMaxPageSize
	}
	if o.limit > 0 && o.limit < size {
		size = o.limit
	}
	maxPages := o.maxPages
	if maxPages == qwAllPages {
		maxPages = qwSafetyPages
	} else if maxPages <= 0 {
		maxPages = 1
	}
	rows = make([]qw.Resource, 0)
	seen := map[string]bool{}
	unstable := false // a repeated id or a moving total means the walk is not trustworthy
	lastTotal := -1
	for page := 1; page <= maxPages; page++ {
		body := map[string]any{"page": map[string]any{"number": page, "size": size}}
		if len(o.filters) > 0 {
			body["filter"] = o.filters
		}
		if o.sort != "" {
			body["sort"] = o.sort
		}
		raw, _, err := c.PostQueryWithParams(ctx, qwTablePath(table, "search"), nil, body)
		if err != nil {
			return nil, false, classifyAPIErrorOnly(err)
		}
		p, err := qw.ParsePage(raw)
		if err != nil {
			return nil, false, apiErr(err)
		}
		if ig := qw.IgnoredFilters(p.Meta); len(ig) > 0 {
			return nil, false, &errIgnoredFilters{table: table, filters: ig}
		}
		for _, r := range p.Data {
			if r.ID != "" && seen[r.ID] {
				unstable = true
				continue
			}
			seen[r.ID] = true
			rows = append(rows, r)
		}
		if o.limit > 0 && len(rows) >= o.limit {
			return rows[:o.limit], false, nil
		}
		pageSize := size
		if per, ok := qw.Num(p.Meta["per_page"]); ok && int(per) > 0 && int(per) < size {
			pageSize = int(per)
		}
		total, hasTotal := qw.Num(p.Meta["total"])
		if hasTotal {
			if lastTotal >= 0 && int(total) != lastTotal {
				unstable = true
			}
			lastTotal = int(total)
		}
		switch {
		case hasTotal && len(rows) >= int(total):
			return rows, !unstable, nil
		case len(p.Data) == 0:
			// An empty page short of meta.total means rows moved mid-walk.
			return rows, !hasTotal && !unstable, nil
		case !hasTotal && len(p.Data) < pageSize:
			return rows, !unstable, nil
		}
	}
	if o.maxPages == qwAllPages {
		return nil, false, apiErr(fmt.Errorf("%s: still more rows after %d pages; refusing to return a truncated result", table, qwSafetyPages))
	}
	return rows, false, nil
}

// asUsage turns an ignored-filter error into a usage error (exit 2).
func asUsage(err error) error {
	var ig *errIgnoredFilters
	if errors.As(err, &ig) {
		return usageErr(err)
	}
	return err
}

func qwGetRow(ctx context.Context, c *client.Client, table, id string) (qw.Resource, error) {
	// Uncached: a header read is often paired with fresh line searches.
	raw, err := c.GetNoCache(ctx, qwTablePath(table, id), nil)
	if err != nil {
		return qw.Resource{}, classifyAPIErrorOnly(err)
	}
	p, err := qw.ParsePage(raw)
	if err != nil {
		return qw.Resource{}, apiErr(err)
	}
	if len(p.Data) == 0 {
		return qw.Resource{}, notFoundErr(fmt.Errorf("%s %s not found", table, id))
	}
	if p.Data[0].ID == "" {
		return qw.Resource{}, apiErr(fmt.Errorf("%s %s: the response had a resource without an id", table, id))
	}
	return p.Data[0], nil
}

// RecGUIDs appear hyphenated and in the compact 32-hex form.
var guidRe = regexp.MustCompile(`^(\{?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}?|[0-9a-fA-F]{32})$`)

func looksLikeGUID(s string) bool { return guidRe.MatchString(strings.TrimSpace(s)) }

// resolveDoc accepts a RecGUID or a DocNo and returns the document header.
func resolveDoc(ctx context.Context, c *client.Client, ref string) (qw.Resource, error) {
	ref = strings.TrimSpace(ref)
	if looksLikeGUID(ref) {
		doc, err := qwGetRow(ctx, c, qw.TableHeaders, strings.Trim(ref, "{}"))
		var ce *cliError
		if err == nil || !errors.As(err, &ce) || ce.code != 3 {
			return doc, err
		}
		// Not a RecGUID after all: fall through to a DocNo lookup.
	}
	rows, complete, err := qwSearch(ctx, c, qw.TableHeaders, qwSearchOpts{filters: []map[string]any{qw.Cond("DocNo", "eq", ref)}, maxPages: qwAllPages})
	if err != nil {
		return qw.Resource{}, asUsage(err)
	}
	if !complete {
		return qw.Resource{}, apiErr(fmt.Errorf("documents numbered %q changed while they were being read; retry or pass the RecGUID", ref))
	}
	switch len(rows) {
	case 0:
		return qw.Resource{}, notFoundErr(fmt.Errorf("no document with DocNo %q; find it with '%s docs search --number %s'", ref, qwCLIName, ref))
	case 1:
		if rows[0].ID == "" {
			return qw.Resource{}, apiErr(fmt.Errorf("document %q came back without a RecGUID; the response is malformed", ref))
		}
		return rows[0], nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, cliutil.ScrubTerminal(fmt.Sprintf("%s (%s)", r.ID, qw.Str(r.Attributes["DocType"]))))
	}
	return qw.Resource{}, usageErr(fmt.Errorf("DocNo %q matches %d documents: %s; pass the RecGUID instead", ref, len(rows), strings.Join(ids, ", ")))
}

// fetchItems returns a document's lines in display order. It filters
// DocumentItems by DocRecGUID instead of relying on include=, because the
// vendor KB (include=Items) and spec (include=DocumentItems) disagree.
func fetchItems(ctx context.Context, c *client.Client, docGUID string) ([]map[string]any, error) {
	rows, complete, err := qwSearch(ctx, c, qw.TableItems, qwSearchOpts{filters: []map[string]any{qw.Cond("DocRecGUID", "eq", docGUID)}, maxPages: qwAllPages})
	if err != nil {
		return nil, asUsage(err)
	}
	if !complete {
		return nil, apiErr(fmt.Errorf("lines of document %s changed while they were being read; retry", docGUID))
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Flat())
	}
	sortLines(out)
	return out, nil
}

func sortLines(lines []map[string]any) {
	key := func(m map[string]any) (float64, float64) {
		return qw.NumOr(m["LineNumberActual"], 1e12), qw.NumOr(m["ID"], 1e12)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		a1, a2 := key(lines[i])
		b1, b2 := key(lines[j])
		if a1 != b1 {
			return a1 < b1
		}
		return a2 < b2
	})
}

// productTables lists every Products_<Db>_Products table in the tenant.
func productTables(ctx context.Context, c *client.Client, onlyDB string) ([]string, error) {
	raw, err := c.Get(ctx, qwTablesPath, nil)
	if err != nil {
		return nil, classifyAPIErrorOnly(err)
	}
	p, err := qw.ParsePage(raw)
	if err != nil {
		return nil, apiErr(err)
	}
	out := make([]string, 0)
	for _, name := range qw.TableNames(p) {
		db, ok := qw.ProductDB(name)
		if !ok {
			continue
		}
		if onlyDB != "" && !strings.EqualFold(db, onlyDB) && !strings.EqualFold(name, onlyDB) {
			continue
		}
		out = append(out, name)
	}
	if onlyDB != "" && len(out) == 0 {
		return nil, notFoundErr(fmt.Errorf("no product database %q; list them with '%s products search --dbs'", onlyDB, qwCLIName))
	}
	return out, nil
}

// qwInBatch is how many values go into one "in" filter.
const qwInBatch = 50

// catalogLookup returns every product row whose ManufacturerPartNumber or
// VendorPartNumber equals one of parts, across the given product tables,
// reading every page. A table without one of the two columns is skipped for
// that column (QuoteWerks reports it as an ignored filter).
func catalogLookup(ctx context.Context, c *client.Client, tables []string, parts []string) ([]qw.Product, error) {
	uniq := make([]string, 0, len(parts))
	seenPart := map[string]bool{}
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && !seenPart[p] {
			seenPart[p] = true
			uniq = append(uniq, p)
		}
	}
	out := make([]qw.Product, 0)
	seen := map[string]bool{}
	for _, t := range tables {
	columns:
		for _, col := range []string{"ManufacturerPartNumber", "VendorPartNumber"} {
			for start := 0; start < len(uniq); start += qwInBatch {
				end := min(start+qwInBatch, len(uniq))
				vals := make([]any, 0, end-start)
				for _, v := range uniq[start:end] {
					vals = append(vals, v)
				}
				rows, complete, err := qwSearch(ctx, c, t, qwSearchOpts{filters: []map[string]any{qw.Cond(col, "in", vals)}, maxPages: qwAllPages})
				if err != nil {
					var ig *errIgnoredFilters
					if errors.As(err, &ig) {
						continue columns
					}
					return nil, err
				}
				if !complete {
					return nil, apiErr(fmt.Errorf("product database %s changed while it was being read; retry", t))
				}
				for _, r := range rows {
					if key := t + ":" + r.ID; !seen[key] {
						seen[key] = true
						out = append(out, qw.ProductFromAttrs(t, r.ID, r.Attributes))
					}
				}
			}
		}
	}
	return out, nil
}

// lineParts collects the part numbers on a document's product lines.
func lineParts(lines []map[string]any) []string {
	out := make([]string, 0, len(lines)*2)
	for _, l := range lines {
		if qw.IsProductLine(l) {
			out = append(out, qw.Str(l["ManufacturerPartNumber"]), qw.Str(l["VendorPartNumber"]))
		}
	}
	return out
}

func qwWrite(ctx context.Context, c *client.Client, method, table, id string, attrs map[string]any) (qw.Resource, error) {
	data := map[string]any{"type": table, "attributes": attrs}
	var (
		raw json.RawMessage
		err error
	)
	if method == "PATCH" {
		data["id"] = id
		raw, _, err = c.Patch(ctx, qwTablePath(table, id), map[string]any{"data": data})
	} else {
		raw, _, err = c.Post(ctx, qwTablePath(table), map[string]any{"data": data})
	}
	if err != nil {
		// A create that failed without a QuoteWerks rejection (a dropped
		// connection, a timeout, a 5xx) may still have been committed, so the
		// caller must check before retrying rather than create it again.
		var rejected *client.APIError
		if method == "POST" && !(errors.As(err, &rejected) && rejected.StatusCode >= 400 && rejected.StatusCode < 500) {
			err = fmt.Errorf("%w (%w: check QuoteWerks before retrying, a retried create can duplicate the row)", err, errWriteMayHaveCommitted)
		}
		return qw.Resource{}, classifyAPIErrorOnly(err)
	}
	p, err := qw.ParsePage(raw)
	if err != nil || len(p.Data) == 0 || p.Data[0].ID == "" {
		e := fmt.Errorf("%s %s returned success but no usable resource in the response", method, table)
		if method == "POST" { // the create succeeded, so its row may exist
			e = fmt.Errorf("%w (%w: check QuoteWerks before retrying, a retried create can duplicate the row)", e, errWriteMayHaveCommitted)
		}
		return qw.Resource{}, apiErr(e)
	}
	return p.Data[0], nil
}

// errWriteMayHaveCommitted marks a create whose outcome is unknown.
var errWriteMayHaveCommitted = errors.New("the row may have been created before the failure")

// jsonapiBody is what a preview shows for one pending write.
func jsonapiBody(method, table, id string, attrs map[string]any) map[string]any {
	data := map[string]any{"type": table, "attributes": attrs}
	path := qwTablePath(table)
	if method == "PATCH" {
		data["id"] = id
		path = qwTablePath(table, id)
	}
	return map[string]any{"method": method, "path": path, "body": map[string]any{"data": data}}
}

func project(row map[string]any, keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := row[k]; ok {
			out[k] = v
		}
	}
	return out
}

// qwPreviewNote is printed in preview envelopes.
const qwPreviewNote = "preview only; nothing was written. Re-run with --confirm to write (needs an API key with AllowWrite)"

// printWriteResult prints a write preview or receipt. Compaction is turned
// off because --agent's --compact would strip the request body or the
// per-line results, which are the whole point of the output.
func printWriteResult(w io.Writer, v any, flags *rootFlags) error {
	saved := flags.compact
	flags.compact = false
	defer func() { flags.compact = saved }()
	return printJSONFiltered(w, v, flags)
}

// sc scrubs server-supplied text before it reaches a terminal.
func sc(v any) string { return cliutil.ScrubTerminal(qw.Str(v)) }

// writableColumns reads a table's schema and returns the columns a client may
// set on create (not primary key, not read-only, not server-calculated).
func writableColumns(ctx context.Context, c *client.Client, table string) (map[string]bool, error) {
	raw, err := c.GetNoCache(ctx, qwTablePath(table, "schema"), nil)
	if err != nil {
		return nil, classifyAPIErrorOnly(err)
	}
	var schema struct {
		Columns []struct {
			Name       string `json:"name"`
			PrimaryKey bool   `json:"primary_key"`
			ReadOnly   *bool  `json:"readonly"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil || len(schema.Columns) == 0 {
		return nil, apiErr(fmt.Errorf("could not read the %s schema to know which columns can be copied", table))
	}
	serverSide := map[string]bool{"RecGUID": true, "ID": true, "DocRecGUID": true, "ExtendedPrice": true, "ExtendedCost": true, "ExtendedList": true, "LineNumberActual": true}
	out := map[string]bool{}
	for _, col := range schema.Columns {
		if col.PrimaryKey || (col.ReadOnly != nil && *col.ReadOnly) || serverSide[col.Name] {
			continue
		}
		out[col.Name] = true
	}
	return out, nil
}

// writeModeEnvelope wraps a write command result.
func writeModeEnvelope(confirm bool, payload map[string]any) map[string]any {
	if confirm {
		payload["mode"] = "written"
	} else {
		payload["mode"] = "preview"
		payload["note"] = qwPreviewNote
	}
	return payload
}

// --- local mirror access ---------------------------------------------------

func mirrorDBPath(p string) string {
	if p != "" {
		return p
	}
	return defaultDBPath(qwCLIName)
}

// openMirror opens the local mirror read-only. ok is false (with a stderr
// hint) when no mirror exists yet; callers then print an empty result. The
// first resource is the one the command reads rows from: a database file
// whose mirror never completed for it (a failed first run still creates the
// file) counts as no mirror, so the JSON does not report "mirrored": true
// over rows that were never read.
func openMirror(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (*store.Store, bool, error) {
	dbPath = mirrorDBPath(dbPath)
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: %s mirror --db %s\n", dbPath, qwCLIName, dbPath)
		return nil, false, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening mirror %s: %w", dbPath, err)
	}
	for _, r := range resources {
		mirrorHint(cmd, db, r, flags.maxAge)
	}
	if len(resources) > 0 {
		if state, stateErr := readSyncHintState(db, resources[0]); stateErr == nil && !state.hasState {
			_ = db.Close()
			return nil, false, nil
		}
	}
	return db, true, nil
}

// partialResources lists resources whose last mirror stopped before reading
// everything (sync_state.last_attempt_complete = 0).
func partialResources(db *store.Store, resources ...string) []string {
	out := make([]string, 0)
	for _, r := range resources {
		var complete int
		err := db.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, r).Scan(&complete)
		if err == nil && complete == 0 {
			out = append(out, r)
		}
	}
	return out
}

// mirrorHint is the mirror-aware form of the generated sync hints: these rows
// come from 'mirror', not 'sync', so the hint names the right command.
func mirrorHint(cmd *cobra.Command, db *store.Store, resource string, maxAge time.Duration) {
	state, err := readSyncHintState(db, resource)
	if err != nil {
		return
	}
	if !state.hasState {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s have not been mirrored yet. Run '%s mirror' before trusting local results.\n", resource, qwCLIName)
		return
	}
	if p := partialResources(db, resource); len(p) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: the last mirror of %s stopped before reading everything (--max-pages); results are partial. Run '%s mirror' with a higher --max-pages.\n", resource, qwCLIName)
	}
	if maxAge > 0 {
		if age := time.Since(state.lastSynced); age > maxAge {
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: mirrored %s are %s old, older than --max-age=%s. Run '%s mirror' to refresh.\n", resource, syncHintRoundAge(age), maxAge, qwCLIName)
		}
	}
}

// loadMirror reads every mirrored row of one resource type. ListScan drains
// its own rows before returning, so callers can query again afterwards.
func loadMirror(db *store.Store, resource string) ([]map[string]any, error) {
	out := make([]map[string]any, 0)
	var decodeErr error
	err := db.ListScan(resource, func(_ string, data json.RawMessage) bool {
		var m map[string]any
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			decodeErr = err
			return false
		}
		out = append(out, m)
		return true
	})
	if err != nil {
		return nil, err
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("decoding mirrored %s: %w", resource, decodeErr)
	}
	return out, nil
}

// mirroredProducts reads mirrored product rows, optionally one database only.
func mirroredProducts(db *store.Store, productDB string) ([]qw.Product, error) {
	rows, err := loadMirror(db, qwResProducts)
	if err != nil {
		return nil, err
	}
	out := make([]qw.Product, 0, len(rows))
	for _, r := range rows {
		p := qw.ProductFromAttrs(qw.Str(r["source_table"]), qw.Str(r["source_id"]), r)
		if productDB != "" && !strings.EqualFold(p.DB, productDB) && !strings.EqualFold(p.Table, productDB) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// mirroredProductIndex indexes mirrored products; nil when none are mirrored.
func mirroredProductIndex(db *store.Store, productDB string) (*qw.ProductIndex, error) {
	products, err := mirroredProducts(db, productDB)
	if err != nil || len(products) == 0 {
		return nil, err
	}
	return qw.NewProductIndex(products), nil
}

// sinceCutoff parses a --since duration ("90d") into a cutoff time.
func sinceCutoff(since string, now time.Time) (time.Time, bool, error) {
	if strings.TrimSpace(since) == "" {
		return time.Time{}, false, nil
	}
	d, err := cliutil.ParseDurationLoose(since)
	if err != nil {
		return time.Time{}, false, usageErr(fmt.Errorf("--since %q: %w", since, err))
	}
	return now.Add(-d), true, nil
}

// inWindow reports whether a document's DocDate falls on or after cutoff's
// date: the same midnight boundary the live DocDate filter sends
// (qw.FilterDate), so local and live --since agree.
func inWindow(doc map[string]any, cutoff time.Time, has bool) bool {
	if !has {
		return true
	}
	boundary, _ := qw.ParseDate(qw.FilterDate(cutoff))
	d, ok := qw.ParseDate(doc["DocDate"])
	return ok && !d.Before(boundary)
}

func docTypeIs(doc map[string]any, types map[string]bool) bool {
	if len(types) == 0 {
		return true
	}
	return types[strings.ToUpper(qw.Str(doc["DocType"]))]
}

// partCSVSet normalizes comma-separated part numbers for matching.
func partCSVSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if v := qw.NormalizePart(p); v != "" {
			out[v] = true
		}
	}
	return out
}

func upperCSVSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if v := strings.ToUpper(strings.TrimSpace(p)); v != "" {
			out[v] = true
		}
	}
	return out
}

// mirroredDocs returns non-removed mirrored documents filtered by type and window.
func mirroredDocs(db *store.Store, types map[string]bool, cutoff time.Time, hasCutoff bool) ([]map[string]any, error) {
	rows, err := loadMirror(db, qwResDocuments)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if !qw.IsActiveDocument(r) || !docTypeIs(r, types) || !inWindow(r, cutoff, hasCutoff) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// groupKey derives the grouping value for --by on a document.
func groupKey(doc map[string]any, by string) string {
	var k string
	switch by {
	case "customer":
		k = qw.Str(doc["SoldToCompany"])
	case "rep":
		k = qw.Str(doc["SalesRep"])
	case "month":
		if d, ok := qw.ParseDate(doc["DocDate"]); ok {
			k = d.Format("2006-01")
		}
	}
	if k == "" {
		return "(none)"
	}
	return k
}

func validateBy(by string, allowed ...string) error {
	for _, a := range allowed {
		if by == a {
			return nil
		}
	}
	return usageErr(fmt.Errorf("--by %q: use one of %s", by, strings.Join(allowed, ", ")))
}

func pctPtr(num, den float64) *float64 {
	if den == 0 {
		return nil
	}
	v := qw.Round2(num / den * 100)
	return &v
}

func parseLooseDurationFlag(name, v string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil {
		return 0, usageErr(fmt.Errorf("%s %q: %w", name, v, err))
	}
	return d, nil
}

// validateDocTypes rejects document types QuoteWerks does not have, so a typo
// fails loudly instead of matching nothing.
func validateDocTypes(set map[string]bool, flag string) error {
	for t := range set {
		if t != "QUOTE" && t != "ORDER" && t != "INVOICE" {
			return usageErr(fmt.Errorf("%s %q: use QUOTE, ORDER or INVOICE", flag, t))
		}
	}
	return nil
}

// filterEcho returns the command-local flags the caller set, so every local
// analytics command echoes its inputs the same way. It checks f.Changed over
// VisitAll: LocalFlags() is a derived FlagSet that never records which flags
// were parsed, so its Visit sees nothing.
func filterEcho(cmd *cobra.Command) map[string]string {
	out := map[string]string{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Changed && f.Name != "db" {
			out[f.Name] = f.Value.String()
		}
	})
	return out
}

// noMirrorResult prints the empty result of a local command run before any
// mirror exists: a one-line hint for people, the empty envelope for machines.
func noMirrorResult(cmd *cobra.Command, flags *rootFlags, view any) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		fmt.Fprintf(cmd.OutOrStdout(), "No local mirror yet; run '%s mirror' first.\n", qwCLIName)
		return nil
	}
	return printJSONFiltered(cmd.OutOrStdout(), view, flags)
}

// coveredDocs returns the documents whose lines the mirror read in full.
func coveredDocs(db *store.Store) (map[string]bool, error) {
	rows, err := loadMirror(db, qwResItemCoverage)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[qw.Str(r["id"])] = true
	}
	return out, nil
}

// lineMargin is a line's margin percent. With a unit of pricing factor the
// unit price and unit cost sit on different bases (only price is scaled), so
// the stored extended amounts are used, or the margin is unknown.
func lineMargin(l map[string]any) *float64 {
	if qw.HasPricingFactor(l) {
		return marginOf(l["ExtendedPrice"], l["ExtendedCost"])
	}
	return marginOf(l["UnitPrice"], l["UnitCost"])
}

// marginOf returns the margin percent only when both amounts are known.
func marginOf(price, cost any) *float64 {
	p, okP := qw.Num(price)
	c, okC := qw.Num(cost)
	if !okP || !okC {
		return nil
	}
	if m, ok := qw.MarginPct(p, c); ok {
		return &m
	}
	return nil
}
