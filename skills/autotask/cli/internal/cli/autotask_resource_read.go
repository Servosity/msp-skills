// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strconv"
	"time"
)

// autotask handfix export-tail-read-records: Autotask lists an entity's records only through
// POST /<Entity>/query (or GET ?search=). The only parameterless GET under each entity is
// /<Entity>/entityInformation/fields, so the press mapped export and tail there and both wrote
// the field definitions as if they were records. Every readable resource is pointed at the
// query sync walks instead, paged the same way: by id, maxRecords at a time.

// resourceIDCursorParam carries the previous page's last id between the generated pager
// (resourcePageParams, resourceNextCursor) and readResourcePage. It never reaches the wire.
const resourceIDCursorParam = "autotask-id-after"

type resourcePageClient interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
	PostWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error)
	PostQueryWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error)
}

func init() {
	for resource := range resourceReadPaths {
		walk, ok := syncResourceIDWalkConfig(resource)
		path, err := syncResourcePath(resource)
		if !ok || err != nil || syncResourceMethod(resource) != "POST" {
			delete(resourceReadPaths, resource) // no record query: refuse it rather than export its field list
			continue
		}
		resourceReadPaths[resource] = path
		resourceReadConfigs[resource] = resourceReadConfig{responsePath: "items", paginationType: "cursor", cursorParam: resourceIDCursorParam, limitParam: walk.limitParam, pageSize: walk.limit}
	}
}

// readResourceFor is the resource whose record query path is, or "" (a single-record path).
func readResourceFor(path string) string {
	for r, p := range resourceReadPaths {
		if p == path {
			return r
		}
	}
	return ""
}

// readResourcePage fetches one page of the resource read at path. While a full page comes back,
// the reply gains the next page's cursor under resourceIDCursorParam. A single-record path
// (export <resource> <id>: GET /<Entity>/{id}) is read as it is.
func readResourcePage(ctx context.Context, c resourcePageClient, path string, params map[string]string) (json.RawMessage, error) {
	resource := readResourceFor(path)
	walk, ok := syncResourceIDWalkConfig(resource)
	if !ok {
		return c.Get(ctx, path, params)
	}
	query := map[string]string{}
	for k, v := range params {
		query[k] = v
	}
	cursor := query[resourceIDCursorParam]
	delete(query, resourceIDCursorParam)
	limit, err := strconv.Atoi(query[walk.limitParam])
	if err != nil || limit <= 0 {
		limit = walk.limit
		query[walk.limitParam] = strconv.Itoa(limit)
	}
	data, err := syncFetch(ctx, c, resource, path, query, walk, cursor)
	if err != nil {
		return nil, err
	}
	items, _, _ := extractResourcePage(data, resourceReadConfigFor(resource))
	next, more := nextIDWalkPage(resource, "", items, limit, walk, false, io.Discard)
	var envelope map[string]json.RawMessage
	if !more || json.Unmarshal(data, &envelope) != nil {
		return data, nil
	}
	envelope[resourceIDCursorParam], _ = json.Marshal(next)
	return json.Marshal(envelope)
}

// tailChangeField is each tailed resource's documented modification time
// (spec.json: a date-time every edit moves; a lastActivity field is not one).
var tailChangeField = map[string]string{
	"appointments":        "updateDateTime",
	"companies":           "lastTrackedModifiedDateTime",
	"company-notes":       "lastModifiedDate",
	"configuration-items": "lastModifiedTime",
	"contacts":            "lastModifiedDate",
	"currencies":          "lastModifiedDateTime",
	"service-bundles":     "lastModifiedDate",
	"service-calls":       "lastModifiedDateTime",
	"services":            "lastModifiedDate",
	"tickets":             "lastTrackedModificationDateTime",
	"time-entries":        "lastModifiedDateTime",
}

// tailState is what one tailed path has emitted in this process (each
// record's content hash) and, from the last poll that read every page, the
// highest id its walk by id read and the newest modification time it read.
type tailState struct {
	seen     map[string][32]byte
	complete bool // a poll read every page: afterID and since bound the next poll
	afterID  int64
	since    time.Time
	sinceRaw string
}

var tailStates = map[string]*tailState{}

// walkNewResourceRecords hands emit every record at path that is new or has
// changed since this process last emitted it. The first poll walks every page
// (Codex r1 class incomplete-paging-read-as-complete: tail emitted the first
// 100 records and dropped the rest). A later poll (Codex r2 class documented
// updates omitted: asking only for higher ids never emitted a ticket completed
// after the first poll) asks for the records with a higher id plus those
// modified since the newest modification the last complete poll read, when
// the resource documents a modification time (tailChangeField), and walks
// every page otherwise; a record whose content did not change is not emitted
// again.
//
// Codex r1 class incomplete-read-advances-cursor (handfix
// r1k-tail-cursor-advances-only-on-a-complete-poll): the cursors moved page
// by page, so a poll that stopped part-way (a 401 on its second page) kept
// the newest modification time of the page it read, and every later poll
// skipped an update with an earlier time on a page it never read. The highest
// id and the newest modification time now move only when a poll reads every
// page of every pass, the highest id only on a walk by id (a modified-since
// pass skips every record not modified since, so its last id is not where the
// ids were read up to), and a record is marked emitted only once emit
// returned. A poll that stops part-way reads the same window again next time;
// the content hashes keep it from emitting a record twice.
// ponytail: a whole walk costs one query per 100 records each poll; raise
// --interval for a large resource with no modification time.
func walkNewResourceRecords(ctx context.Context, c resourcePageClient, path string, config resourceReadConfig, emit func(data json.RawMessage, items []json.RawMessage) error) error {
	resource := readResourceFor(path)
	walk, _ := syncResourceIDWalkConfig(resource)
	st, ok := tailStates[path]
	if !ok {
		st = &tailState{seen: map[string][32]byte{}}
		tailStates[path] = st
	}
	field := tailChangeField[resource]
	type pass struct{ filter, cursor string }
	passes := []pass{{}}
	if st.complete && field != "" {
		passes = []pass{{cursor: strconv.FormatInt(st.afterID, 10)}}
		if st.sinceRaw != "" {
			f, _ := json.Marshal([]map[string]any{{"op": "gte", "field": field, "value": st.sinceRaw}})
			passes = append(passes, pass{filter: string(f)})
		}
	}
	read := tailState{afterID: st.afterID, since: st.since, sinceRaw: st.sinceRaw} // what this poll read; st takes it once every page is read
	for _, p := range passes {
		cursor := p.cursor
		for page := 0; ; page++ {
			params := resourcePageParams(config, cursor, page, 0)
			if p.filter != "" {
				params[walk.filterParam] = p.filter
			}
			data, err := readResourcePage(ctx, c, path, params)
			if err != nil {
				return err
			}
			items, next, hasMore := extractResourcePage(data, config)
			var changed []json.RawMessage
			sums := map[string][32]byte{}
			for _, item := range items {
				id, _ := nextIDWalkCursor([]json.RawMessage{item}, walk.idField)
				if n, err := strconv.ParseInt(id, 10, 64); err == nil && p.filter == "" && n > read.afterID {
					read.afterID = n
				}
				if sum := sha256.Sum256(item); st.seen[id] != sum && sums[id] != sum {
					sums[id] = sum
					changed = append(changed, item)
				}
				read.noteModified(item, field)
			}
			if len(changed) > 0 {
				if err := emit(data, changed); err != nil {
					return err
				}
			}
			for id, sum := range sums {
				st.seen[id] = sum
			}
			var stop bool
			if cursor, stop, err = resourceNextCursor(config, cursor, next, hasMore); err != nil || stop {
				if err != nil {
					return err
				}
				break
			}
		}
	}
	st.complete, st.afterID, st.since, st.sinceRaw = true, read.afterID, read.since, read.sinceRaw
	return nil
}

// noteModified keeps the newest modification time a record carries.
func (st *tailState) noteModified(item json.RawMessage, field string) {
	if field == "" {
		return
	}
	var rec map[string]any
	if json.Unmarshal(item, &rec) != nil {
		return
	}
	raw, _ := rec[field].(string)
	if at, err := time.Parse(time.RFC3339Nano, raw); err == nil && at.After(st.since) {
		st.since, st.sinceRaw = at, raw
	}
}
