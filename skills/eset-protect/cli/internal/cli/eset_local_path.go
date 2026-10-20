// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "local-read-absolute-paths".
//
// Every ESET Connect service except device-management lives on its own host,
// so the generated commands carry absolute paths such as
// "https://{region}.incident-management.eset.systems/v2/incidents". The
// generated local reader split that whole string on "/", so "https:" and the
// host became a parent filter ({Type: "https:", ID: "<host>"}) that no stored
// row matches: `incidents list --data-source local`, and the default auto mode
// whenever ESET was unreachable, printed "0 results (cached ...)" over a full
// mirror. localPathSegments (data_source.go) now derives segments from the
// path alone, through this one helper.
//
// A few generated list commands put their filter into the path's query
// string instead of the params map (appendArrayQueryParam: devices list
// --display-names, policy-assignments list --policy-uuids, devices-batch-get
// --devices-uuids). The local reader never sees that query, so it used to
// return the whole collection as if unfiltered, with exit 0 (adversarial
// round 4, P1). resolveLocal now refuses every path that carries a query
// (handfixes.json: local-read-path-query-filters), the same closed failure
// as any other row filter local data cannot apply.
package cli

import (
	"net/url"
	"sort"
	"strings"
)

// esetLocalPathQueryKeys returns the distinct parameter names in path's
// query string, sorted; nil when the path carries no query. A name that does
// not unescape is returned raw, and a piece with no name is reported as the
// whole piece, so a query that names nothing recognizable still refuses.
func esetLocalPathQueryKeys(path string) []string {
	i := strings.IndexByte(path, '?')
	if i < 0 {
		return nil
	}
	seen := map[string]bool{}
	var keys []string
	for _, piece := range strings.Split(path[i+1:], "&") {
		if piece == "" {
			continue
		}
		key, _, _ := strings.Cut(piece, "=")
		if k, err := url.QueryUnescape(key); err == nil {
			key = k
		}
		if key == "" {
			key = piece
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// esetLocalPathOnly returns the path part of an absolute URL ("" when it has
// none) and any other path unchanged. The query string, if any, is dropped:
// resolveLocal refuses a path with a query before it gets here
// (esetLocalPathQueryKeys).
func esetLocalPathOnly(path string) string {
	if i := strings.Index(path, "://"); i >= 0 {
		rest := path[i+3:]
		j := strings.IndexByte(rest, '/')
		if j < 0 {
			return ""
		}
		path = rest[j:]
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return path
}
