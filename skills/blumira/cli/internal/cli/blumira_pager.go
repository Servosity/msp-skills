// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built shared pager for the novel commands' live list reads (evidence
// fetch, MSP sub-account devices). Blumira lists are documented as
// {data, links, meta, status} with page/page_size paging (spec.json
// api_links, api_meta).

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// blumiraPageGetter is the client surface the pager needs (test seam).
type blumiraPageGetter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// blumiraPageSize is the documented page_size maximum.
const blumiraPageSize = 200

// blumiraListLimit is the documented maximum of the `limit` query parameter
// ("Maximum number of records to return"); the default (200) would cap a walk
// early, which the total_items check below then reports as incomplete.
const blumiraListLimit = 5000

// fetchAllBlumiraPages walks a documented paginated list to its end and
// returns every row. It fails closed: a request error, a page that is not a
// JSON object, a status other than OK, an error field, a page without a data
// array, a page that repeats the previous one, an empty page while any signal
// says more rows follow, or a walk cut off at maxPages returns an error and
// no rows, so a caller can never mistake part of a list for all of it.
//
// More rows follow when ANY signal says so: links.next is set, meta.total_pages
// is beyond this page, meta.total_items is above the rows read so far, or a
// page with no paging metadata at all came back full.
func fetchAllBlumiraPages(ctx context.Context, c blumiraPageGetter, path string, maxPages int) ([]json.RawMessage, error) {
	if maxPages <= 0 {
		maxPages = 1
	}
	var all []json.RawMessage
	var prevFirst json.RawMessage
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("%s: more than %d pages; stopped before the end of the list", path, maxPages)
		}
		raw, err := c.Get(ctx, path, map[string]string{
			"page":      strconv.Itoa(page),
			"page_size": strconv.Itoa(blumiraPageSize),
			"limit":     strconv.Itoa(blumiraListLimit),
		})
		if err != nil {
			return nil, err
		}
		items, more, err := parseBlumiraPage(raw, page, len(all))
		if err != nil {
			return nil, fmt.Errorf("%s page %d: %w", path, page, err)
		}
		if len(items) > 0 {
			if prevFirst != nil && bytes.Equal(items[0], prevFirst) {
				return nil, fmt.Errorf("%s page %d repeats page %d; the API ignored the page parameter", path, page, page-1)
			}
			prevFirst = items[0]
		}
		all = append(all, items...)
		if !more {
			return all, nil
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%s page %d: empty page while the response says more rows follow", path, page)
		}
	}
}

// parseBlumiraPage reads one documented list page. seen is the number of rows
// read on earlier pages.
func parseBlumiraPage(raw json.RawMessage, page, seen int) ([]json.RawMessage, bool, error) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return nil, false, fmt.Errorf("response is not a JSON object")
	}
	if st, ok := env["status"]; ok && !isJSONNull(st) {
		var status string
		if err := json.Unmarshal(st, &status); err != nil || !strings.EqualFold(strings.TrimSpace(status), "OK") {
			return nil, false, fmt.Errorf("API reported status %s", strings.TrimSpace(string(st)))
		}
	}
	for _, key := range []string{"error", "errors"} {
		if v, ok := env[key]; ok && !isJSONNull(v) && !isEmptyJSONValue(v) {
			return nil, false, fmt.Errorf("API reported %s: %s", key, truncateForError(string(v)))
		}
	}
	data, ok := env["data"]
	if !ok {
		return nil, false, fmt.Errorf("response has no data array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil || items == nil {
		return nil, false, fmt.Errorf("response data is not an array")
	}

	more := false
	hasPaging := false
	if v, ok := env["links"]; ok && !isJSONNull(v) {
		var links map[string]json.RawMessage
		if err := json.Unmarshal(v, &links); err != nil {
			return nil, false, fmt.Errorf("response links is not an object")
		}
		hasPaging = true
		if next, ok := links["next"]; ok && !isJSONNull(next) {
			var s string
			if err := json.Unmarshal(next, &s); err != nil {
				return nil, false, fmt.Errorf("response links.next is not a string")
			}
			if strings.TrimSpace(s) != "" {
				more = true
			}
		}
	}
	if v, ok := env["meta"]; ok && !isJSONNull(v) {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(v, &meta); err != nil {
			return nil, false, fmt.Errorf("response meta is not an object")
		}
		hasPaging = true
		if n, ok, err := metaInt(meta, "total_pages"); err != nil {
			return nil, false, err
		} else if ok && n > page {
			more = true
		}
		if n, ok, err := metaInt(meta, "total_items"); err != nil {
			return nil, false, err
		} else if ok && n > seen+len(items) {
			more = true
		}
	}
	if !hasPaging && len(items) >= blumiraPageSize {
		more = true
	}
	return items, more, nil
}

func metaInt(meta map[string]json.RawMessage, key string) (int, bool, error) {
	v, ok := meta[key]
	if !ok || isJSONNull(v) {
		return 0, false, nil
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err != nil {
		return 0, false, fmt.Errorf("response meta.%s is not a number", key)
	}
	i, err := strconv.Atoi(n.String())
	if err != nil {
		return 0, false, fmt.Errorf("response meta.%s is not an integer", key)
	}
	return i, true, nil
}

func isEmptyJSONValue(v json.RawMessage) bool {
	s := strings.TrimSpace(string(v))
	return s == `""` || s == "[]" || s == "{}" || s == "false"
}

func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// blumiraSyncPageSaysMore reports whether a documented list page says more
// rows follow (blumira handfix sync-documented-paging): links.next is set,
// meta.total_pages is beyond this page, or meta.total_items is above the rows
// read so far (seen, this page included). cursor is the page number sent.
func blumiraSyncPageSaysMore(raw json.RawMessage, cursor string, seen int) bool {
	page, err := strconv.Atoi(strings.TrimSpace(cursor))
	if err != nil || page < 1 {
		page = 1
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil {
		return false
	}
	if v, ok := env["links"]; ok {
		var links map[string]json.RawMessage
		if json.Unmarshal(v, &links) == nil {
			var next string
			if json.Unmarshal(links["next"], &next) == nil && strings.TrimSpace(next) != "" {
				return true
			}
		}
	}
	if v, ok := env["meta"]; ok {
		var meta map[string]json.RawMessage
		if json.Unmarshal(v, &meta) == nil {
			if n, ok, err := metaInt(meta, "total_pages"); err == nil && ok && n > page {
				return true
			}
			if n, ok, err := metaInt(meta, "total_items"); err == nil && ok && n > seen {
				return true
			}
		}
	}
	return false
}
