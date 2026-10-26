// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ninjaone-pp-cli/internal/ninjapage"
	"ninjaone-pp-cli/internal/platform"
)

// NinjaOne's documented list shapes for every CLI paging reader: sync (both loops), export,
// tail and the endpoint `--all` paginator. The rule itself lives in internal/ninjapage, shared
// with the MCP pages.
//
//   - Keyset lists return a bare array and page by the last row's key: ?after=<id> on the
//     after-id lists, ?anchorNaturalId=<naturalId> on /v2/ticketing/app-user-contact.
//   - Report lists return {"cursor":{"name":..,"offset":..},"results":[..]}; the token stays
//     the same while the offset advances. In export and tail the offset rides along in the
//     internal cursor (token NUL offset) so the did-not-advance guard compares progress, and
//     only the token is sent on the wire.
//
// A page ends a walk only when it is empty or shorter than the size asked for.
//
// Hand-fix (handfixes.json ninja-paging-every-reader).

// extractResourcePage wraps the rendered page reader with both rules.
func extractResourcePage(data json.RawMessage, config resourceReadConfig) ([]json.RawMessage, string, bool) {
	items, next, hasMore := extractResourcePageRendered(data, config)
	size := config.pageSize
	if size <= 0 {
		size = 100
	}
	if strings.Contains(next, "\x00") && !ninjapage.Full(len(items), size) {
		return items, "", false
	}
	if next == "" && ninjapage.Full(len(items), size) && ninjapage.IsBareArray(data) {
		if k, ok := ninjapage.KeysetForParam(config.cursorParam); ok {
			if lastID, ok := k.LastID(items); ok {
				return items, lastID, true
			}
		}
	}
	return items, next, hasMore
}

// resourceReadConfigFor is the rendered read config, with the keyset the generated config never
// modelled (/v2/ticketing/app-user-contact rendered as offset paging without an offset
// parameter, so every page re-read page one).
func resourceReadConfigFor(resource string) resourceReadConfig {
	config := resourceReadConfigForRendered(resource)
	path, err := resourceReadPath(resource)
	if err != nil {
		return config
	}
	if k, ok := ninjapage.KeysetForPath(path); ok {
		config.paginationType = "cursor"
		config.cursorParam = k.Param
		if config.limitParam == "" {
			config.limitParam = "pageSize"
		}
	}
	return config
}

// NinjaListDefaultPageSize is the default page size sync uses for the list endpoint at path,
// 0 when no sync partition lists it. MCP compares pages against min(requested, this).
func NinjaListDefaultPageSize(path string) int {
	if partition, ok := ninjaSyncPartitionForPath(path); ok {
		return determinePaginationDefaults(partition).limit
	}
	return 0
}

// syncKeysetParam and syncKeysetLastID are the keyset sync's after-id fallback sends for a
// full bare-array page of path: the documented keyset of the endpoint, `after` otherwise.
func syncKeysetParam(path string) string {
	if k, ok := ninjapage.KeysetForPath(path); ok {
		return k.Param
	}
	return afterIDParam
}

func syncKeysetLastID(path string, items []json.RawMessage) (string, bool) {
	if k, ok := ninjapage.KeysetForPath(path); ok {
		return k.LastID(items)
	}
	return extractLastItemID(items)
}

// resourceObjectCursor reads an object-valued cursor: its token, plus the offset
// when the object carries one. "" when raw is not such an object.
func resourceObjectCursor(raw json.RawMessage) string {
	token := cursorTokenFromObject(raw)
	if token == "" {
		return ""
	}
	var inner map[string]json.RawMessage
	if json.Unmarshal(raw, &inner) == nil {
		if offset, ok := jsonScalarString(inner["offset"]); ok {
			return token + "\x00" + offset
		}
	}
	return token
}

// resourceCursorWireValue is the part of an internal cursor sent to the API.
func resourceCursorWireValue(cursor string) string {
	token, _, _ := strings.Cut(cursor, "\x00")
	return token
}

// fetchAndEmit is tail's poll: every page of the resource, walked with export's reader
// (resourcePageParams, extractResourcePage, resourceNextCursor), instead of page one only.
func fetchAndEmit(ctx context.Context, c interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
}, path string, config resourceReadConfig, enc *json.Encoder) error {
	if config.paginationType == "" {
		return fetchAndEmitRendered(ctx, c, path, config, enc)
	}
	cursor := ""
	for page := 0; ; page++ {
		if page > 100000 {
			return fmt.Errorf("tail pagination exceeded 100000 pages for %s", path)
		}
		params := resourcePageParams(config, cursor, page, 0)
		data, err := c.Get(ctx, path, params)
		if err != nil {
			return err
		}
		items, next, hasMore := extractResourcePage(data, config)
		if items == nil {
			return enc.Encode(tailEvent(json.RawMessage(data)))
		}
		for _, item := range items {
			if err := enc.Encode(tailEvent(item)); err != nil {
				return err
			}
		}
		if len(items) == 0 {
			return nil
		}
		var stop bool
		cursor, stop, err = resourceNextCursor(config, cursor, next, hasMore)
		if err != nil {
			return fmt.Errorf("tail pagination for %s: %w", path, err)
		}
		if stop {
			return nil
		}
		if config.paginationType == "offset" || config.paginationType == "page" {
			// Numbered paging advances only through a cursor parameter; without one every
			// page would be page one again.
			if config.cursorParam == "" {
				return nil
			}
			if requested, _ := strconv.Atoi(params[config.limitParam]); !hasMore && !ninjapage.Full(len(items), requested) {
				return nil
			}
		}
	}
}

func tailEvent(data json.RawMessage) map[string]any {
	return map[string]any{
		"event":     "data",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}
}

// ninjaAllMaxPages bounds an `--all` walk. The rendered paginator stops at 100 pages, which is
// 10,000 rows of a 100-row report (one fleet's software inventory); a NinjaOne walk carries its
// own progress guard (a repeated keyset or cursor offset stops it), so the bound is only a
// backstop, the same one export uses.
const ninjaAllMaxPages = 100000

// ninjaPaginatedGet is the endpoint `--all` walk for NinjaOne's two shapes; handled is false
// for any other shape (activities, numbered pages), which keeps the rendered paginator. A
// page-one shape this walk does not recognise also goes back to the rendered paginator.
func ninjaPaginatedGet(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, path string, params map[string]string, headers map[string]string, fetchAll bool, cursorParam, limitParam string, defaultPageSize int, responsePath string) (json.RawMessage, bool, error) {
	if !fetchAll {
		return nil, false, nil
	}
	keyset, isKeyset := ninjapage.KeysetForPath(path)
	if !isKeyset {
		keyset, isKeyset = ninjapage.KeysetForParam(cursorParam)
	}
	if !isKeyset && cursorParam != "cursor" {
		return nil, false, nil
	}
	get := c.GetWithHeaders
	// Report pages resend the same URL while NinjaOne advances the offset; a cached body
	// would replay page two forever.
	if nc, ok := c.(interface {
		GetWithHeadersNoCache(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
	}); ok {
		get = nc.GetWithHeadersNoCache
	}
	clean := map[string]string{}
	for k, v := range params {
		if v == "" || ((k == cursorParam || (isKeyset && k == keyset.Param)) && v == "0") {
			continue
		}
		clean[k] = v
	}
	requested, _ := strconv.Atoi(clean[limitParam])
	limit := ninjapage.Limit(requested, defaultPageSize)
	all := make([]json.RawMessage, 0)
	seen := map[string]bool{}
	pages := 0
	truncated := ""
	for page := 1; ; page++ {
		pages = page
		if humanFriendly {
			fmt.Fprintf(os.Stderr, "fetching page %d...\n", page)
		} else {
			fmt.Fprintf(os.Stderr, `{"event":"page_fetch","page":%d}`+"\n", page)
		}
		data, err := get(ctx, path, clean, headers)
		if err != nil {
			return nil, true, err
		}
		if isDryRunResponseForClient(c, data) {
			return data, true, nil
		}
		var items []json.RawMessage
		next, progress, sendParam := "", "", ""
		if ninjapage.IsBareArray(data) && isKeyset {
			_ = json.Unmarshal(data, &items)
			if id, ok := keyset.LastID(items); ok && ninjapage.Full(len(items), limit) {
				next, progress, sendParam = id, keyset.Param+"="+id, keyset.Param
			}
		} else if obj, ok := ninjaReportPage(data, responsePath); ok && cursorParam == "cursor" {
			items = obj.items
			if obj.hasCursor && ninjapage.Full(len(items), limit) {
				next, progress, sendParam = obj.token, obj.progress, cursorParam
			}
		} else if page == 1 {
			return nil, false, nil
		} else {
			return nil, true, fmt.Errorf("--all: page %d of %s changed shape mid-walk", page, path)
		}
		all = append(all, items...)
		if next == "" {
			break
		}
		if seen[progress] {
			truncated = "pagination_cursor_repeated"
			break
		}
		seen[progress] = true
		if page >= ninjaAllMaxPages {
			truncated = "max_pages_cap_hit"
			break
		}
		clean[sendParam] = next
	}
	if truncated != "" {
		// A walk that could not reach its end is never reported complete.
		platform.MarkContextTruncated(ctx, platform.TruncationReason{Kind: truncated, Configured: ninjaAllMaxPages, Observed: pages, MoreAvailable: true})
		if humanFriendly {
			fmt.Fprintf(os.Stderr, "warning: --all stopped after %d pages (%s) with %d items; more remain.\n", pages, truncated, len(all))
		} else {
			fmt.Fprintf(os.Stderr, `{"event":"truncated","reason":%q,"pages":%d,"fetched":%d,"message":"--all could not reach the end of the list; returning fetched pages only"}`+"\n", truncated, pages, len(all))
		}
	} else if humanFriendly {
		fmt.Fprintf(os.Stderr, "fetched %d items across %d pages\n", len(all), pages)
	} else {
		fmt.Fprintf(os.Stderr, `{"event":"complete","total":%d,"pages":%d}`+"\n", len(all), pages)
	}
	out, err := json.Marshal(all)
	if err != nil {
		return nil, true, err
	}
	return out, true, nil
}

type ninjaReport struct {
	items           []json.RawMessage
	token, progress string
	hasCursor       bool
}

// ninjaReportPage reads a report page: its rows (at responsePath, else "results") and its cursor.
func ninjaReportPage(data json.RawMessage, responsePath string) (ninjaReport, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil {
		return ninjaReport{}, false
	}
	field := responsePath
	if field == "" {
		field = "results"
	}
	raw, ok := obj[field]
	if !ok {
		return ninjaReport{}, false
	}
	var r ninjaReport
	if json.Unmarshal(raw, &r.items) != nil {
		return ninjaReport{}, false
	}
	if c, ok := obj["cursor"]; ok {
		r.token, r.progress, r.hasCursor = ninjapage.ReportCursor(c)
	}
	return r, true
}
