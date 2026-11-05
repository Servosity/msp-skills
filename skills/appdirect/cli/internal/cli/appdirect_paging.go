// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// appdirect handfix number-paged-lists-sync-every-page (Codex r1 4.34.2 class
// incomplete-paging-read-as-complete). Two parts:
//
//   - Nine lists page with `number` and `size` and answer AppDirect's
//     PageMetadata ({"content":[...],"page":{"number","size","totalElements",
//     "totalPages"}}: OpportunityPage and the other paged lists in spec.yaml).
//     Press 4.34.2 does not read `number` as a page parameter, so sync fetched
//     their first page (ten opportunities by default) and declared it the
//     whole set. appdirectNumberPaging pages them: the first page number the
//     spec documents (its default, its description or its example), and the
//     page size it documents, so no server cap can shorten a page.
//   - Whatever ends a walk (a short page, an empty page, a list sync does not
//     page, a single page), a partition is complete only when its last
//     response's documented paging metadata agrees: page.totalElements and
//     page.totalPages (PageMetadata), totalCount, pageInfo.hasNextPage, a
//     "next" link or has_more. When the metadata reports more records than
//     sync read, the partition is incomplete: the sync warns, prunes nothing
//     and keeps the earlier watermark (the generated incomplete-outcome path).

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// appdirectNumberPaged are the lists AppDirect pages with number/size, with
// their documented first page number and page size.
var appdirectNumberPaged = map[string]paginationDefaults{
	// number default 1, size default 10 (OpportunityPage example page.number 1)
	"assisted-sales-v1-opportunities": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 10},
	// example page.number 1 on a one-page result
	"app-reseller-v1-orders":        {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 10},
	"app-reseller-v1-subscriptions": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 10},
	// "Page number starting at 1. The default value is 1." / "The default value is 50."
	"app-reseller-v1-payment-methods": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 50},
	// example page.number 0 on a one-page result
	"app-reseller-v1-company-associations": {cursorParam: "number", cursorType: "page", initialCursor: "0", limitParam: "size", limit: 10},
	// number default 0, size default 50 (maximum 250)
	"channel-v1-company-groups": {cursorParam: "number", cursorType: "page", initialCursor: "0", limitParam: "size", limit: 50},
	// "1-based page index", size default 10
	"checkout": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 10},
	// "The first page has index 1", size default 50
	"reporting-v2-reports": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 50},
	// number default 1, size default 10
	"reseller": {cursorParam: "number", cursorType: "page", initialCursor: "1", limitParam: "size", limit: 10},
}

func appdirectNumberPaging(resource string) (paginationDefaults, bool) {
	d, ok := appdirectNumberPaged[resource]
	return d, ok
}

// appdirectPageShortfall reads the paging metadata of a list's last response
// and reports, as text, how it says the walk was short: more records than
// read (consumed) or more pages than fetched (pages). "" when it says nothing
// more exists or carries no metadata.
func appdirectPageShortfall(data json.RawMessage, consumed, pages int) string {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var env map[string]any
	if dec.Decode(&env) != nil {
		return ""
	}
	more := func(total any, read int) bool {
		n, ok := total.(json.Number)
		if !ok {
			return false
		}
		r, ok := new(big.Rat).SetString(n.String())
		return ok && r.Cmp(big.NewRat(int64(read), 1)) > 0
	}
	if page, ok := env["page"].(map[string]any); ok {
		if more(page["totalElements"], consumed) {
			return fmt.Sprintf("its page metadata reports %v records and sync read %d", page["totalElements"], consumed)
		}
		if more(page["totalPages"], pages) {
			return fmt.Sprintf("its page metadata reports %v pages and sync read %d", page["totalPages"], pages)
		}
	}
	for _, k := range []string{"totalCount", "totalElements", "total_count"} {
		if more(env[k], consumed) {
			return fmt.Sprintf("its %s reports %v records and sync read %d", k, env[k], consumed)
		}
	}
	if info, ok := env["pageInfo"].(map[string]any); ok {
		if v, _ := info["hasNextPage"].(bool); v {
			return "its pageInfo.hasNextPage is true"
		}
	}
	for _, k := range []string{"has_more", "hasMore", "has_next", "hasNext"} {
		if v, _ := env[k].(bool); v {
			return "its " + k + " is true"
		}
	}
	if links, ok := env["links"].([]any); ok {
		for _, l := range links {
			if m, ok := l.(map[string]any); ok {
				if rel, _ := m["rel"].(string); strings.EqualFold(rel, "next") {
					return "it links a next page"
				}
			}
		}
	}
	return ""
}
