// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Package syncer implements the Afi fleet fan-out sync. The Afi public API
// exposes exactly one flat list (application installations); every other
// entity is path-scoped under an org or tenant ID. The framework sync command
// therefore only covers installations, and this package walks the hierarchy:
//
//	installations -> orgs -> child orgs -> tenants -> per-tenant entities
//
// Everything is stored through the generic store.Upsert(resourceType, id,
// data) API (generic `resources` table + FTS). The typed per-entity tables the
// generator emitted are intentionally bypassed: the Afi entity literally named
// "resources" collides with the framework's generic resources table, so the
// typed UpsertResources path cannot work against the real schema. Generic
// rows keyed by resource_type are sufficient for every novel query and give
// full-text search for free.
//
// Parent IDs are injected into the stored JSON (org_id / tenant_id) whenever
// the upstream object lacks them, so json_extract joins always have a key.
package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"time"
)

// Getter is the slice of the generated API client the fleet sync needs.
type Getter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// Sink is the slice of the local store the fleet sync writes through.
type Sink interface {
	Upsert(resourceType, id string, data json.RawMessage) error
	SaveSyncState(resourceType, cursor string, count int) error
}

// Options tunes a fleet sync run.
type Options struct {
	// PageLimit is the per-page `limit` query value (server may clamp).
	PageLimit int
	// MaxPages bounds pages fetched per list endpoint (scan cap, not output cap).
	MaxPages int
	// Tenants optionally restricts per-tenant syncing to these tenant IDs.
	Tenants []string
	// SkipArchives skips the per-tenant archives walk (the heaviest list).
	SkipArchives bool
	// TaskStatsWindowDays is the lookback window for the per-tenant task
	// statistics snapshot. 0 means 7 days.
	TaskStatsWindowDays int
	// MaxOrgDepth bounds the child-org recursion. 0 means 5.
	MaxOrgDepth int
	// Progress receives one line per major step when non-nil.
	Progress io.Writer
	// Now allows tests to pin the clock. Nil means time.Now.
	Now func() time.Time
}

// Summary reports what a fleet sync run stored.
type Summary struct {
	Counts   map[string]int `json:"counts"`
	Orgs     int            `json:"orgs"`
	Tenants  int            `json:"tenants"`
	APICalls int            `json:"api_calls"`
	Warnings []string       `json:"warnings,omitempty"`
}

type runner struct {
	g    Getter
	db   Sink
	opts Options
	sum  *Summary
}

// Run executes the fleet fan-out sync. Per-org and per-tenant failures are
// recorded as warnings and skipped (a partial fleet picture beats none); only
// a failure to list installations — the root of the walk — is fatal.
func Run(ctx context.Context, g Getter, db Sink, opts Options) (*Summary, error) {
	if opts.PageLimit <= 0 {
		opts.PageLimit = 100
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 50
	}
	if opts.TaskStatsWindowDays <= 0 {
		opts.TaskStatsWindowDays = 7
	}
	if opts.MaxOrgDepth <= 0 {
		opts.MaxOrgDepth = 5
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	r := &runner{g: g, db: db, opts: opts, sum: &Summary{Counts: map[string]int{}}}

	installs, err := r.pageList(ctx, "/api/v1/applications/me/installations", nil)
	if err != nil {
		return r.sum, fmt.Errorf("listing application installations (the root of the fleet walk): %w", err)
	}

	orgSeen := map[string]bool{}
	tenantOrg := map[string]string{} // tenant id -> org id (when known)
	var orgQueue []orgDepth
	for _, raw := range installs {
		obj, err := decode(raw)
		if err != nil {
			r.warnf("decoding installation: %v", err)
			continue
		}
		orgID, _ := obj["org_id"].(string)
		tenantID, _ := obj["tenant_id"].(string)
		id := orgID + ":" + tenantID
		r.store("installations", id, obj)
		if tenantID != "" {
			if _, ok := tenantOrg[tenantID]; !ok {
				tenantOrg[tenantID] = orgID
			}
		}
		if orgID != "" && !orgSeen[orgID] {
			orgSeen[orgID] = true
			orgQueue = append(orgQueue, orgDepth{id: orgID, depth: 0})
		}
	}
	r.progressf("installations: %d (orgs to walk: %d, direct tenants: %d)", len(installs), len(orgQueue), len(tenantOrg))

	// Walk orgs breadth-first: org detail, child orgs, tenants, subscriptions.
	tenantFetched := map[string]bool{}
	orgParent := map[string]string{} // child org id -> parent org id
	for len(orgQueue) > 0 {
		cur := orgQueue[0]
		orgQueue = orgQueue[1:]

		if data, err := r.get(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id), nil); err != nil {
			r.warnf("org %s: %v", cur.id, err)
		} else if obj, err := decode(data); err == nil {
			// Re-inject the parent linkage discovered during the child walk:
			// the org-detail payload has no parent field, and this store call
			// overwrites the row written when the parent enumerated children.
			if p := orgParent[cur.id]; p != "" {
				obj["parent_org_id"] = p
			}
			r.store("orgs", cur.id, obj)
		}

		if cur.depth < r.opts.MaxOrgDepth {
			children, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/orgs", nil)
			if err != nil {
				r.warnf("org %s children: %v", cur.id, err)
			}
			for _, raw := range children {
				obj, err := decode(raw)
				if err != nil {
					continue
				}
				childID, _ := obj["id"].(string)
				if childID == "" {
					continue
				}
				obj["parent_org_id"] = cur.id
				if _, ok := orgParent[childID]; !ok {
					orgParent[childID] = cur.id
				}
				r.store("orgs", childID, obj)
				if !orgSeen[childID] {
					orgSeen[childID] = true
					orgQueue = append(orgQueue, orgDepth{id: childID, depth: cur.depth + 1})
				}
			}
		}

		tenants, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/tenants", nil)
		if err != nil {
			r.warnf("org %s tenants: %v", cur.id, err)
		}
		for _, raw := range tenants {
			obj, err := decode(raw)
			if err != nil {
				continue
			}
			tid, _ := obj["id"].(string)
			if tid == "" {
				continue
			}
			obj["org_id"] = cur.id
			r.store("tenants", tid, obj)
			tenantFetched[tid] = true
			if _, ok := tenantOrg[tid]; !ok {
				tenantOrg[tid] = cur.id
			}
		}

		subs, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/licensing/subscriptions", nil)
		if err != nil {
			r.warnf("org %s subscriptions: %v", cur.id, err)
		}
		for _, raw := range subs {
			obj, err := decode(raw)
			if err != nil {
				continue
			}
			sid, _ := obj["id"].(string)
			if sid == "" {
				tid, _ := obj["tenant_id"].(string)
				sid = cur.id + ":" + tid
			}
			obj["org_id"] = cur.id
			r.store("subscriptions", sid, obj)
		}
	}
	r.sum.Orgs = len(orgSeen)

	// Tenants from tenant-scoped installations the org walk didn't cover.
	for tid, oid := range tenantOrg {
		if tenantFetched[tid] {
			continue
		}
		data, err := r.get(ctx, "/api/v1/tenants/"+url.PathEscape(tid), nil)
		if err != nil {
			r.warnf("tenant %s: %v", tid, err)
			continue
		}
		obj, err := decode(data)
		if err != nil {
			r.warnf("tenant %s decode: %v", tid, err)
			continue
		}
		if oid != "" {
			obj["org_id"] = oid
		}
		r.store("tenants", tid, obj)
		tenantFetched[tid] = true
	}

	// Per-tenant entity walk.
	want := map[string]bool{}
	for _, t := range r.opts.Tenants {
		want[t] = true
	}
	tenantIDs := make([]string, 0, len(tenantFetched))
	for tid := range tenantFetched {
		if len(want) > 0 && !want[tid] {
			continue
		}
		tenantIDs = append(tenantIDs, tid)
	}
	sort.Strings(tenantIDs)
	r.sum.Tenants = len(tenantIDs)

	now := r.opts.Now().UTC()
	winStart := now.AddDate(0, 0, -r.opts.TaskStatsWindowDays)
	for _, tid := range tenantIDs {
		r.progressf("tenant %s ...", tid)
		r.syncTenant(ctx, tid, winStart, now)
	}

	// Record sync state so framework hints (`hintIfUnsynced`, `stale`,
	// doctor's cache report) see the fleet tables as synced.
	for _, rt := range []string{"installations", "orgs", "tenants", "subscriptions", "resources", "protections", "policies", "archives", "quotas", "task_stats"} {
		if err := r.db.SaveSyncState(rt, "", r.sum.Counts[rt]); err != nil {
			r.warnf("saving sync state for %s: %v", rt, err)
		}
	}
	return r.sum, nil
}

type orgDepth struct {
	id    string
	depth int
}

func (r *runner) syncTenant(ctx context.Context, tid string, winStart, winEnd time.Time) {
	base := "/api/v1/tenants/" + url.PathEscape(tid)

	if data, err := r.get(ctx, base+"/quotas", nil); err != nil {
		r.warnf("tenant %s quotas: %v", tid, err)
	} else if obj, err := decode(data); err == nil {
		if quotas, ok := obj["quotas"].([]any); ok {
			for _, q := range quotas {
				qo, ok := q.(map[string]any)
				if !ok {
					continue
				}
				kind, _ := qo["kind"].(string)
				qo["tenant_id"] = tid
				r.store("quotas", tid+":"+kind, qo)
			}
		}
	}

	lists := []struct {
		path     string
		rtype    string
		skip     bool
		injectID bool // synthesize id from tenant when item has none
	}{
		{path: base + "/resources", rtype: "resources"},
		{path: base + "/protections", rtype: "protections"},
		{path: base + "/policies", rtype: "policies"},
		{path: base + "/archives", rtype: "archives", skip: r.opts.SkipArchives},
	}
	for _, l := range lists {
		if l.skip {
			continue
		}
		items, err := r.pageList(ctx, l.path, nil)
		if err != nil {
			r.warnf("tenant %s %s: %v", tid, l.rtype, err)
			continue
		}
		for _, raw := range items {
			obj, err := decode(raw)
			if err != nil {
				continue
			}
			if _, ok := obj["tenant_id"]; !ok {
				obj["tenant_id"] = tid
			}
			id, _ := obj["id"].(string)
			if id == "" {
				continue
			}
			r.store(l.rtype, id, obj)
		}
	}

	stats, err := r.get(ctx, base+"/tasks/statistics/summary", map[string]string{
		"start_time": winStart.Format(time.RFC3339),
		"end_time":   winEnd.Format(time.RFC3339),
	})
	if err != nil {
		r.warnf("tenant %s task stats: %v", tid, err)
		return
	}
	obj, err := decode(stats)
	if err != nil {
		r.warnf("tenant %s task stats decode: %v", tid, err)
		return
	}
	obj["tenant_id"] = tid
	obj["window_start"] = winStart.Format(time.RFC3339)
	obj["window_end"] = winEnd.Format(time.RFC3339)
	obj["fetched_at"] = r.opts.Now().UTC().Format(time.RFC3339)
	r.store("task_stats", tid, obj)
}

// pageList walks a paginated Afi list endpoint (limit/page_token ->
// next_page_token) until the token runs dry or MaxPages is hit.
func (r *runner) pageList(ctx context.Context, path string, params map[string]string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	token := ""
	for page := 0; page < r.opts.MaxPages; page++ {
		p := map[string]string{"limit": fmt.Sprintf("%d", r.opts.PageLimit)}
		for k, v := range params {
			p[k] = v
		}
		if token != "" {
			p["page_token"] = token
		}
		data, err := r.get(ctx, path, p)
		if err != nil {
			return out, err
		}
		var pageObj struct {
			Items         []json.RawMessage `json:"items"`
			NextPageToken string            `json:"next_page_token"`
		}
		if err := json.Unmarshal(data, &pageObj); err != nil {
			return out, fmt.Errorf("decoding page of %s: %w", path, err)
		}
		out = append(out, pageObj.Items...)
		if pageObj.NextPageToken == "" {
			return out, nil
		}
		token = pageObj.NextPageToken
	}
	r.warnf("%s: stopped at max-pages=%d with more pages remaining", path, r.opts.MaxPages)
	return out, nil
}

func (r *runner) get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	r.sum.APICalls++
	return r.g.Get(ctx, path, params)
}

func (r *runner) store(resourceType, id string, obj map[string]any) {
	data, err := json.Marshal(obj)
	if err != nil {
		r.warnf("marshaling %s %s: %v", resourceType, id, err)
		return
	}
	if err := r.db.Upsert(resourceType, id, data); err != nil {
		r.warnf("storing %s %s: %v", resourceType, id, err)
		return
	}
	r.sum.Counts[resourceType]++
}

func (r *runner) warnf(format string, args ...any) {
	r.sum.Warnings = append(r.sum.Warnings, fmt.Sprintf(format, args...))
}

func (r *runner) progressf(format string, args ...any) {
	if r.opts.Progress == nil {
		return
	}
	fmt.Fprintf(r.opts.Progress, format+"\n", args...)
}

func decode(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}
