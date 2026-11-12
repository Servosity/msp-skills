// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json sync-delta-search-walk, issue #695): HubSpot's
// object list (GET /crm/v3/objects/{type}) has no modified-since parameter,
// so the generated walk re-reads every record on every sync. Once a resource
// has a watermark, hsSyncDelta walks POST /crm/v3/objects/{type}/search
// instead: modified-date GTE the watermark minus hsSyncDeltaOverlap, sorted
// ascending, the same properties the list read sends. It never prunes and
// stamps a watermark only when the walk ends on its last page. Deleted
// records are reconciled by `sync --full`, which is unchanged.
//
// Every page is a fresh query from the previous page's last timestamp (GTE;
// upserts absorb the repeated rows), never an `after` offset. Search pages by
// position, so a record updated mid-walk moves to the end and shifts every row
// behind it one place forward: an offset walk skips one, then stamps a
// watermark past it. A fresh query per page cannot skip, and never nears
// search's 10,000-result window. A full page on one timestamp cannot advance:
// that resource falls back to the list walk.
//
// A list walk of these resources always starts from the top: one that resumed
// an earlier run's cursor would stamp a watermark newer than the rows that run
// read, and the next delta would trust it (Codex round 2 P1). --max-pages caps
// a delta walk instead, stamping the last timestamp it read.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"hubspot-pp-cli/internal/store"
)

// hsSyncDeltaOverlap covers HubSpot's search indexing lag ("a few moments").
const hsSyncDeltaOverlap = 5 * time.Minute

const hsSearchPageSize = 200

type hsSearcher interface {
	PostQueryWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error)
}

// hsSyncDeltaProperty is the modified-date property a resource's search can
// filter and sort on, or "" when the resource is not a CRM object.
func hsSyncDeltaProperty(resource string) string {
	path, err := syncResourcePath(resource)
	if err != nil || !strings.HasPrefix(path, "/crm/v3/objects/") {
		return ""
	}
	if resource == "hubspot-contacts-crm" {
		return "lastmodifieddate"
	}
	return "hs_lastmodifieddate"
}

// hsSyncDelta runs the delta walk and reports done=true with its result, or
// done=false to let syncResource run the list walk, from the top for an
// object resource. A watermarked object resource that falls back clears
// effectiveSince and names the reason, so the list walk does not also report
// it as not incremental.
func hsSyncDelta(ctx context.Context, c any, db *store.Store, resource, sinceTS string, full bool, lastSynced time.Time, maxPages int, withHistory []string, userParams *syncUserParams, scopeKey string, events io.Writer, started time.Time, effectiveSince, existingCursor *string) (syncResult, bool) {
	prop := hsSyncDeltaProperty(resource)
	if prop == "" {
		return syncResult{}, false
	}
	*existingCursor = ""
	if full || sinceTS != "" || lastSynced.IsZero() {
		return syncResult{}, false
	}
	params := map[string]string{}
	userParams.applyTo(resource, params, false)
	userProps := params["properties"]
	delete(params, "properties")
	searcher, ok := c.(hsSearcher)
	preview, _ := c.(interface{ IsDryRun() bool })
	switch {
	case len(withHistory) > 0: // search has no propertiesWithHistory
		return hsSyncDeltaFallBack(events, resource, "with_history", effectiveSince)
	case len(params) > 0: // a filter the search body would not carry
		return hsSyncDeltaFallBack(events, resource, "user_params", effectiveSince)
	case !ok:
		return hsSyncDeltaFallBack(events, resource, "no_search_client", effectiveSince)
	case preview != nil && preview.IsDryRun():
		return hsSyncDeltaFallBack(events, resource, "dry_run", effectiveSince)
	}

	path, _ := syncResourcePath(resource)
	persistedCursor, _, bound, err := db.BindSyncScope(resource, scopeKey)
	if err != nil {
		return syncResult{Resource: resource, Err: err, Duration: time.Since(started)}, true
	}
	if err := db.ClearSyncForbidden(resource); err != nil {
		return syncResult{Resource: resource, Err: fmt.Errorf("clearing the forbidden mark for %s: %w", resource, err), Duration: time.Since(started)}, true
	}
	if err := db.SaveSyncProgressInScope(resource, scopeKey, persistedCursor, bound); err != nil {
		return syncResult{Resource: resource, Err: fmt.Errorf("saving sync progress for %s: starting attempt: %w", resource, err), Duration: time.Since(started)}, true
	}

	var props []string
	if p := hsCollectedProperties(resource, userProps); p != "" {
		props = strings.Split(p, ",")
	}
	from := lastSynced.Add(-hsSyncDeltaOverlap).UnixMilli()
	since := time.UnixMilli(from).UTC().Format(time.RFC3339)
	fetched, requests := 0, 0
	capped := false
	var withheld []string

	// fail keeps the old watermark: progress only, attempt incomplete.
	fail := func(err error) (syncResult, bool) {
		count, _ := db.Count(resource)
		if serr := db.SaveSyncProgressInScope(resource, scopeKey, persistedCursor, count); serr != nil {
			err = errors.Join(err, serr)
		}
		if w, ok := isSyncAccessWarning(err); ok {
			if !humanFriendly {
				fmt.Fprintln(events, syncWarningJSON(resource, "", w.Status, w.Reason, w.Message))
			}
			return syncResult{Resource: resource, Count: count, Warn: fmt.Errorf("skipped due to insufficient access: %s (%s)", resource, w.Reason), ForbiddenKey: syncForbiddenKey(w, requests-1, resource), Duration: time.Since(started)}, true
		}
		if !humanFriendly {
			fmt.Fprintln(events, syncErrorJSON(resource, "", err))
		}
		return syncResult{Resource: resource, Count: count, Err: fmt.Errorf("searching %s: %w", resource, err), Duration: time.Since(started)}, true
	}

	for {
		body := map[string]any{
			"filterGroups": []any{map[string]any{"filters": []any{map[string]any{"propertyName": prop, "operator": "GTE", "value": strconv.FormatInt(from, 10)}}}},
			"sorts":        []any{map[string]any{"propertyName": prop, "direction": "ASCENDING"}},
			"limit":        hsSearchPageSize,
		}
		if len(props) > 0 {
			body["properties"] = props
		}
		requests++
		data, _, err := searcher.PostQueryWithParams(ctx, path+"/search", nil, body)
		if err != nil {
			return fail(err)
		}
		var page struct {
			Results []json.RawMessage `json:"results"`
			Paging  struct {          // presence only: the next page is a fresh query
				Next struct {
					After string `json:"after"`
				} `json:"next"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return fail(fmt.Errorf("decoding a search page: %w", err))
		}
		stored, extractFailures, typedFailures, err := upsertResourceBatch(db, resource, page.Results)
		var held *store.WithheldRowsError
		if errors.As(err, &held) {
			withheld = append(withheld, held.IDs...)
			if !humanFriendly {
				fmt.Fprintln(events, store.WithheldEvent(resource, held.IDs))
			}
			err = nil
		}
		if err != nil {
			return fail(fmt.Errorf("upserting batch for %s: %w", resource, err))
		}
		if extractFailures+typedFailures > 0 || (held == nil && stored < len(page.Results)) {
			return fail(fmt.Errorf("%s search returned %d row(s) and stored %d", resource, len(page.Results), stored))
		}
		fetched += len(page.Results)
		if page.Paging.Next.After == "" {
			break
		}
		if len(page.Results) == 0 {
			return fail(fmt.Errorf("%s search returned an empty page with a next page", resource))
		}
		first, ok1 := hsRowMillis(page.Results[0], prop)
		last, ok2 := hsRowMillis(page.Results[len(page.Results)-1], prop)
		if !ok1 || !ok2 || last <= first || last < from {
			return hsSyncDeltaFallBack(events, resource, "page_one_timestamp", effectiveSince)
		}
		from = last
		if maxPages > 0 && requests >= maxPages {
			capped = true
			break
		}
	}

	count, err := db.Count(resource)
	if err != nil {
		return fail(fmt.Errorf("counting stored %s rows: %w", resource, err))
	}
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "  %s: incremental, %d changed since %s\n", resource, fetched, since)
	} else {
		fmt.Fprintf(events, `{"event":"sync_delta","resource":%q,"fetched":%d,"requests":%d,"since":%q,"capped":%t}`+"\n", resource, fetched, requests, since, capped)
	}
	if len(withheld) > 0 {
		// Same as the list walk: the watermark stays, so the next run reads them again.
		if err := db.SaveSyncProgressInScope(resource, scopeKey, persistedCursor, count); err != nil {
			return syncResult{Resource: resource, Count: count, Err: fmt.Errorf("saving sync progress for %s: %w", resource, err), Duration: time.Since(started)}, true
		}
		return syncResult{Resource: resource, Count: count, Withheld: withheld, Duration: time.Since(started)}, true
	}
	watermark := started.UTC().Add(-syncWatermarkOverlap)
	if capped {
		// Every row before the last one read was read; the next run starts
		// hsSyncDeltaOverlap before it.
		watermark = time.UnixMilli(from).UTC()
	}
	if err := db.SaveSyncStateAtInScope(resource, scopeKey, "", count, watermark); err != nil {
		return syncResult{Resource: resource, Count: count, Err: fmt.Errorf("saving sync state for %s: %w", resource, err), Duration: time.Since(started)}, true
	}
	if !humanFriendly {
		fmt.Fprintf(events, `{"event":"sync_complete","resource":"%s","total":%d,"duration_ms":%d}`+"\n", resource, count, time.Since(started).Milliseconds())
	}
	return syncResult{Resource: resource, Count: count, Duration: time.Since(started)}, true
}

func hsSyncDeltaFallBack(events io.Writer, resource, reason string, effectiveSince *string) (syncResult, bool) {
	*effectiveSince = ""
	if !humanFriendly {
		fmt.Fprintf(events, `{"event":"sync_delta_fallback","resource":%q,"reason":%q}`+"\n", resource, reason)
	}
	return syncResult{}, false
}

// hsRowMillis reads a row's modified date: ISO 8601 from search, or epoch ms.
func hsRowMillis(row json.RawMessage, prop string) (int64, bool) {
	var r struct {
		Properties map[string]any `json:"properties"`
	}
	if json.Unmarshal(row, &r) != nil {
		return 0, false
	}
	s, _ := r.Properties[prop].(string)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli(), true
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	return ms, err == nil
}
