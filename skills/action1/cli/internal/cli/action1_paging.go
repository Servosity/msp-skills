// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strconv"
	"strings"
)

// withNextPage makes sync follow Action1's own paging (action1 handfix:
// paging-follows-next-page). Action1 lists page with `from` (first record) and
// `limit`, and every ResultPage carries next_page, the URL of the next page
// (empty on the last one). Reading the next `from` from that URL keeps sync
// complete when the server caps `limit` below the requested page size, where
// counting by the requested size would stop early or skip records: as offset
// paging, a page shorter than the requested size ended the walk even while
// next_page named the next one. As cursor paging the walk ends only when
// next_page is empty, and a full page without one is reported incomplete.
func withNextPage(p paginationDefaults) paginationDefaults {
	if p.cursorParam != "from" {
		return p
	}
	if p.nextCursorPath == "" {
		p.nextCursorPath = "next_page"
	}
	if p.nextCursorPath == "next_page" {
		p.cursorType = "cursor"
	}
	return p
}

// action1HasNextPage reports a page that says more records remain (a set
// next_page, or total_items above the records read so far): the list
// continues even when the spec declares no paging parameters for it.
func action1HasNextPage(data []byte) bool {
	var page struct {
		NextPage string `json:"next_page"`
	}
	if json.Unmarshal(data, &page) == nil && strings.TrimSpace(page.NextPage) != "" {
		return true
	}
	_, more, _ := action1OffsetNext(data, "", action1PageItems(data))
	return more
}

// action1DependentScope fills the scope column a walked row's typed table
// expects (action1 handfix org-fan-out-sync). The typed table generated for
// /installed-software/{orgId}/data names its path scope installed_software_id
// and holds the orgId there, as a live read of that path stores it.
func action1DependentScope(dep string, obj map[string]json.RawMessage, parentID json.RawMessage) {
	if dep == "installed_software_data" {
		if _, ok := obj["installed_software_id"]; !ok {
			obj["installed_software_id"] = parentID
		}
	}
}

// action1PageContinuation is the one continuation rule for an Action1 list
// page, applied by both sync loops right after a page is read (action1 handfix
// paging-follows-next-page; Codex r1 action1 class incomplete paging read as
// complete). Action1 documents two page shapes: ResultPage names the next page
// (next_page, followed as the next `from`), and ResultPageAbridged
// (vulnerabilities, roles, permissions) names none, only its position
// (total_items, limit, from). Any signal that more records remain wins: a set
// next_page, else total_items above from plus the records this page held (the
// next `from` is that sum, so a server cap below the requested limit skips
// nothing). A page that states neither is left to the generated rule.
func action1PageContinuation(data []byte, cursor string, items int, nextCursor string, hasMore bool) (string, bool) {
	if nextCursor != "" {
		return nextCursor, hasMore
	}
	if next, more, ok := action1OffsetNext(data, cursor, items); ok {
		return next, more
	}
	return nextCursor, hasMore
}

// action1OffsetNext reads a page's position: ok when it states total_items
// (a number or a numeric string, as Action1 sends it); more while from (the
// page's own, else the request's) plus items is below it.
func action1OffsetNext(data []byte, cursor string, items int) (next string, more, ok bool) {
	var page struct {
		Total json.RawMessage `json:"total_items"`
		From  json.RawMessage `json:"from"`
	}
	if json.Unmarshal(data, &page) != nil {
		return "", false, false
	}
	total, ok := action1Int(page.Total)
	if !ok {
		return "", false, false
	}
	from, fromOK := action1Int(page.From)
	if !fromOK {
		from, _ = strconv.Atoi(strings.TrimSpace(cursor))
	}
	if items <= 0 || from+items >= total {
		return "", false, true
	}
	return strconv.Itoa(from + items), true, true
}

// action1Int reads a JSON number or numeric string as an int.
func action1Int(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 || n != float64(int(n)) {
		return 0, false
	}
	return int(n), true
}

// action1PageItems counts a ResultPage's items.
func action1PageItems(data []byte) int {
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(data, &page) != nil {
		return 0
	}
	return len(page.Items)
}
