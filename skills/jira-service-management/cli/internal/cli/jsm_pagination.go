// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strconv"
	"strings"
)

// jsmMaxPageSize is the largest page JSM's PagedDTO endpoints return. Some
// (customers, JSDCLOUD-8012) silently cap any larger limit at 50, so asking for
// 100 yields a "short" 50-row page and the walker stops early with data missing.
const jsmMaxPageSize = 50

func isAssetsPath(path string) bool { return strings.Contains(path, "/jsm/assets/") }

// The authored spec renames JSM's `start` / Assets' `startAt` to the press
// vocabulary (`offset`) and Assets' `resultPerPage` to `per_page`, keeping the
// real wire key via x-url-name. The generated pager and sync loop key their
// cursor writes by the public name, so without this mapping `--all` and sync
// send `?offset=N`, which JSM ignores, and re-fetch page one forever.
// ponytail: path-based table; move into the generator when the press honours
// x-url-name for pagination params.
func jsmWireParam(path, name string) string {
	switch name {
	case "offset":
		if isAssetsPath(path) {
			return "startAt"
		}
		return "start"
	case "per_page":
		if isAssetsPath(path) {
			return "resultPerPage"
		}
	}
	return name
}

// jsmWirePagination maps a sync page config to wire names and caps JSM pages
// at jsmMaxPageSize (hooked into sync.go's dependent walk).
func jsmWirePagination(p paginationDefaults, path string) paginationDefaults {
	p.cursorParam = jsmWireParam(path, p.cursorParam)
	p.limitParam = jsmWireParam(path, p.limitParam)
	p.limit = jsmPageCap(path, p.limit)
	return p
}

// jsmPageCap caps a JSM page size at jsmMaxPageSize; Assets pages are not capped.
func jsmPageCap(path string, n int) int {
	if !isAssetsPath(path) && n > jsmMaxPageSize {
		return jsmMaxPageSize
	}
	return n
}

// jsmSyncPagination is the flat-resource variant (hooked into sync.go's
// per-resource walk); it resolves the path from the sync resource name.
func jsmSyncPagination(resource string) paginationDefaults {
	path, _ := syncResourcePath(resource)
	return jsmWirePagination(determinePaginationDefaults(resource), path)
}

// jsmClampAllLimit caps an --all walk's page size at JSM's real maximum so a
// server-capped page is never mistaken for the last page.
func jsmClampAllLimit(path string, params map[string]string, limitParam string, fetchAll bool) {
	if !fetchAll || limitParam == "" {
		return
	}
	if n, err := strconv.Atoi(params[limitParam]); err == nil && jsmPageCap(path, n) != n {
		params[limitParam] = strconv.Itoa(jsmPageCap(path, n))
	}
}
