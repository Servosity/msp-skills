// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"microsoft-graph-pp-cli/internal/client"
)

// microsoft-graph handfix (teams-phone-reads-every-page): `phone-numbers list`
// and `teams-users list` answer "which numbers are unassigned" and "who has
// Enterprise Voice", so a first page shown as the whole tenant is a wrong
// answer an MSP acts on. Graph pages numberAssignments with $skip (default
// 500, at most 1,000 per query) and userConfigurations with @odata.nextLink;
// both commands now read to the end of the collection, or fail.

// teamsSkipPageSize is the $top asked of a $skip-paged collection.
const teamsSkipPageSize = "999"

// teamsListRead reads a Teams Phone collection in full from the live API. A
// --dry-run preview and an explicit --data-source local keep the generated
// read path.
func teamsListRead(ctx context.Context, c *client.Client, flags *rootFlags, resourceType, path string, params map[string]string, skipPaged bool, hintWriter io.Writer) (json.RawMessage, DataProvenance, error) {
	if c.IsDryRun() || flags.dataSource == "local" {
		return resolveReadWithStrategyAndResponsePath(ctx, c, flags, "auto", resourceType, true, path, params, nil, "value", hintWriter)
	}
	read := graphGetAllPages
	if skipPaged {
		read = graphGetAllSkipPages
	}
	items, err := read(ctx, c, path, params)
	if err != nil {
		return nil, DataProvenance{}, err
	}
	data, err := marshalJSONArray(items)
	if err != nil {
		return nil, DataProvenance{}, err
	}
	return data, attachFreshness(DataProvenance{Source: "live"}, flags), nil
}

// graphGetAllSkipPages reads a collection Graph pages with $skip. Each window
// asks for a full page (following any nextLink it carries) and the next one
// starts after what came back, until a window is empty, so a server that caps
// the page below $top still reads to the end. A window that starts with the
// same item as the one before means the server ignored $skip: that is an
// error, never a complete read.
func graphGetAllSkipPages(ctx context.Context, c *client.Client, path string, params map[string]string) ([]json.RawMessage, error) {
	return graphReadSkipWindows(ctx, c, path, params, nil)
}

// graphReadSkipWindows is graphGetAllSkipPages under a read policy (nil, or
// sync's graphRead; see graphReadPages). Under sync's policy every request
// counts toward --max-pages, and a cap that falls before the empty window
// that proves the end stops the read as capped.
func graphReadSkipWindows(ctx context.Context, c *client.Client, path string, params map[string]string, rd *graphRead) ([]json.RawMessage, error) {
	const maxWindows = 10000
	skip := 0
	if s, ok := params["$skip"]; ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s: $skip %q is not a count", path, s)
		}
		skip = n
	}
	var all []json.RawMessage
	var prevFirst json.RawMessage
	for w := 0; w < maxWindows; w++ {
		p := make(map[string]string, len(params)+2)
		for k, v := range params {
			p[k] = v
		}
		if p["$top"] == "" {
			p["$top"] = teamsSkipPageSize
		}
		delete(p, "$skip")
		if skip > 0 {
			p["$skip"] = strconv.Itoa(skip)
		}
		items, err := graphReadPages(ctx, c, path, p, nil, w > 0, rd)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return all, nil
		}
		if prevFirst != nil && bytes.Equal(prevFirst, items[0]) {
			return nil, fmt.Errorf("%s: the page at $skip=%d repeats the one before it; the collection was not read to its end", path, skip)
		}
		prevFirst = items[0]
		all = append(all, items...)
		skip += len(items)
		if rd.isCapped() || rd.capReached() {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%s: more than %d pages; the collection was not read to its end", path, maxWindows)
}

// skipPagedCollections are the Graph collections paged with $skip (no
// @odata.nextLink): GetCollection reads them in $skip windows.
var skipPagedCollections = map[string]bool{
	"/admin/teams/telephoneNumberManagement/numberAssignments": true,
}

// GetCollection is the one live read every list path goes through: the CLI's
// list commands (resolveRead), export, tail and the MCP endpoint tools.
// microsoft-graph handfix (list-reads-every-page, class
// incomplete-read-as-complete): a Graph collection is read to its end
// (every @odata.nextLink through the configured host, or every $skip window
// of a skipPagedCollections path) and returned as one page, or the read
// fails; a first page is never presented as the whole collection. A response
// that is not a paged collection (one resource, a bare array, a dry-run
// preview, a binary body) is returned as the API sent it.
func GetCollection(ctx context.Context, c *client.Client, path string, params, headers map[string]string) (json.RawMessage, error) {
	if c.IsDryRun() {
		return c.GetWithHeaders(ctx, path, params, headers)
	}
	if skipPagedCollections[path] {
		items, err := graphGetAllSkipPages(ctx, c, path, params)
		if err != nil {
			return nil, err
		}
		return collectionPage(nil, items)
	}
	first, err := c.GetWithHeaders(ctx, path, params, headers)
	if err != nil {
		return nil, err
	}
	var page map[string]json.RawMessage
	if json.Unmarshal(first, &page) != nil {
		return first, nil
	}
	rawNext, paged := page["@odata.nextLink"]
	if !paged {
		return first, nil
	}
	var items []json.RawMessage
	var next string
	if json.Unmarshal(page["value"], &items) != nil || items == nil || json.Unmarshal(rawNext, &next) != nil || next == "" {
		return nil, fmt.Errorf("%s: page 1 carries a nextLink but is not a Graph collection; the collection was not read to its end", path)
	}
	u, err := url.Parse(next)
	if err != nil {
		return nil, fmt.Errorf("%s: page 1 carries a nextLink that does not parse: %w", path, err)
	}
	rest, err := graphReadPages(ctx, c, graphRelPath(u.Path), flattenQuery(u.Query()), headers, true, nil)
	if err != nil {
		return nil, err
	}
	return collectionPage(page, append(items, rest...))
}

// collectionPage is the first page's envelope (minus its nextLink) holding
// every item read.
func collectionPage(page map[string]json.RawMessage, items []json.RawMessage) (json.RawMessage, error) {
	if page == nil {
		page = map[string]json.RawMessage{}
	}
	delete(page, "@odata.nextLink")
	value, err := marshalJSONArray(items)
	if err != nil {
		return nil, err
	}
	page["value"] = value
	return json.Marshal(page)
}
