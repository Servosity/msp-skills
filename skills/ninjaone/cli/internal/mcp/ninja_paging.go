// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"sort"
	"strconv"
	"strings"

	"ninjaone-pp-cli/internal/cli"
	"ninjaone-pp-cli/internal/ninjapage"
)

// codeOrchRequestedPageSize is the pageSize the caller asked NinjaOne for (its
// documented page-size query parameter), or 0 when none was sent. bound uses it
// to tell a short final page from a full one.
// Hand-fix (handfixes.json ninja-paging-every-reader).
func codeOrchRequestedPageSize(query map[string]string) int {
	n, err := strconv.Atoi(query["pageSize"])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// The generated descriptor of /v2/ticketing/app-user-contact carries no paging config, so
// ninjaone_read returned page one with no next_cursor. Give every endpoint whose keyset the
// generated code never modelled its documented cursor parameter (internal/ninjapage).
func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if ep.Method != "GET" || ep.Page.CursorParam != "" {
			continue
		}
		if k, ok := ninjapage.KeysetForPath(ep.Path); ok {
			ep.Page = mcpPageConfig{CursorParam: k.Param}
		}
	}
}

// ninjaRetainScope binds a retained page to the request that fetched it: the resolved path
// and the query as sent. A continuation is served from that page only when resent with the
// same request. Class mcp-continuation-never-replays-a-stateful-cursor.
func ninjaRetainScope(path string, query map[string]string) string {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("GET ")
	b.WriteString(path)
	for _, k := range keys {
		b.WriteString("\x00")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(query[k])
	}
	return b.String()
}

// codeOrchEffectivePageSize is the page size a continuation decision compares a page against:
// min(requested pageSize, the endpoint's sync default) when both are known (ninjapage.Limit),
// so a request above what NinjaOne serves per page never reads its first full page as the
// last. 0 = unknown: only an empty page ends the walk.
func codeOrchEffectivePageSize(path string, query map[string]string) int {
	return ninjapage.Limit(codeOrchRequestedPageSize(query), cli.NinjaListDefaultPageSize(path))
}
