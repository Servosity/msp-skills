// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	// Prune deletes the stored rows of resourceType whose top-level JSON field
	// scopeField equals scopeValue ("" field: every row) and whose id is not in
	// keep. The sync calls it only after reading that list in full.
	Prune(resourceType, scopeField, scopeValue string, keep []string) error
	// ScopeValues lists the distinct non-null values of scopeField among the
	// stored rows of resourceType.
	ScopeValues(resourceType, scopeField string) ([]string, error)
}

// GapResourceType holds one row per list the last fleet-sync could not read
// in full (an API error, the --max-pages cap, the org depth limit, or
// --skip-archives). The rows stored for such a list may be stale or partial,
// so every novel names its gaps; a later complete read of the list clears the
// row. Rows carry gap_scope ("fleet" or the tenant id), list and reason, and
// tenant_id for a tenant's own lists.
const GapResourceType = "fleet_sync_gaps"

// tenantListTypes are the per-tenant collections, pruned with their tenant.
var tenantListTypes = []string{"resources", "protections", "policies", "archives", "quotas", "task_stats", GapResourceType}

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
	// Incomplete names every list this run could not read in full (see
	// GapResourceType); the novels report the same set from the store.
	Incomplete []string `json:"incomplete,omitempty"`
}

type runner struct {
	g    Getter
	db   Sink
	opts Options
	sum  *Summary
	// membershipIncomplete is set when the set of orgs or tenants could not be
	// enumerated in full; rows of orgs and tenants that were not seen are then
	// kept, because an unseen one may still exist.
	membershipIncomplete bool
	fleetGaps            []string
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
	// A run that stops before its end (a failed installations read, a canceled
	// context, a killed process) leaves this gap, so every report says the
	// latest refresh did not finish; the end of a run replaces the fleet gaps
	// with its own (Codex r1 afi review class failed-sync-read-as-complete).
	r.storeGap("fleet", "", "fleet-sync", "a fleet-sync started at "+opts.Now().UTC().Format(time.RFC3339)+" and did not finish")

	installs, installsComplete, err := r.pageList(ctx, "/api/v1/applications/me/installations", nil)
	if err != nil {
		return r.sum, fmt.Errorf("listing application installations (the root of the fleet walk): %w", err)
	}
	if !installsComplete {
		r.fleetGap("installations", r.capReason())
		r.membershipIncomplete = true
	}
	var installIDs []string

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
		installIDs = append(installIDs, id)
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

		if cur.depth >= r.opts.MaxOrgDepth {
			r.fleetGap("org "+cur.id+" child orgs", fmt.Sprintf("not listed: the org is at the depth limit (%d)", r.opts.MaxOrgDepth))
			r.membershipIncomplete = true
		} else {
			children, complete, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/orgs", nil)
			if err != nil {
				r.warnf("org %s children: %v", cur.id, err)
				r.fleetGap("org "+cur.id+" child orgs", err.Error())
			} else if !complete {
				r.fleetGap("org "+cur.id+" child orgs", r.capReason())
			}
			if err != nil || !complete {
				r.membershipIncomplete = true
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

		tenants, complete, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/tenants", nil)
		if err != nil {
			r.warnf("org %s tenants: %v", cur.id, err)
			r.fleetGap("org "+cur.id+" tenants", err.Error())
		} else if !complete {
			r.fleetGap("org "+cur.id+" tenants", r.capReason())
		}
		if err != nil || !complete {
			r.membershipIncomplete = true
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

		subs, complete, err := r.pageList(ctx, "/api/v1/orgs/"+url.PathEscape(cur.id)+"/licensing/subscriptions", nil)
		if err != nil {
			r.warnf("org %s subscriptions: %v", cur.id, err)
			r.fleetGap("org "+cur.id+" subscriptions", err.Error())
		} else if !complete {
			r.fleetGap("org "+cur.id+" subscriptions", r.capReason())
		}
		var subIDs []string
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
			subIDs = append(subIDs, sid)
		}
		if err == nil && complete {
			r.prune("subscriptions", "org_id", cur.id, subIDs)
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
			r.tenantGap(tid, "tenant", err.Error())
			continue
		}
		obj, err := decode(data)
		if err != nil {
			r.warnf("tenant %s decode: %v", tid, err)
			r.tenantGap(tid, "tenant", err.Error())
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

	// Rows the API no longer returns are removed once the list that would
	// carry them was read in full, so a removed org, tenant or installation
	// (and a removed tenant's collections) never reads as current.
	if installsComplete {
		r.prune("installations", "", "", installIDs)
	}
	if !r.membershipIncomplete {
		live := map[string]bool{}
		for tid := range tenantFetched {
			live[tid] = true
		}
		for tid := range tenantOrg {
			live[tid] = true
		}
		r.pruneUnlisted("tenants", "tenant_id", live, tenantListTypes)
		r.pruneUnlisted("orgs", "org_id", orgSeen, []string{"subscriptions"})
	}
	r.prune(GapResourceType, "gap_scope", "fleet", r.fleetGaps)

	// Record sync state so framework hints (`hintIfUnsynced`, `stale`,
	// doctor's cache report) see the fleet tables as synced.
	for _, rt := range []string{"installations", "orgs", "tenants", "subscriptions", "resources", "protections", "policies", "archives", "quotas", "task_stats"} {
		if err := r.db.SaveSyncState(rt, "", r.sum.Counts[rt]); err != nil {
			r.warnf("saving sync state for %s: %v", rt, err)
		}
	}
	return r.sum, nil
}

// pruneUnlisted removes the rows of parentType whose id is not in live, and
// every row of childTypes whose scopeField names such a parent.
func (r *runner) pruneUnlisted(parentType, scopeField string, live map[string]bool, childTypes []string) {
	keep := make([]string, 0, len(live))
	for id := range live {
		keep = append(keep, id)
	}
	sort.Strings(keep)
	for _, ct := range childTypes {
		values, err := r.db.ScopeValues(ct, scopeField)
		if err != nil {
			r.warnf("listing stored %s by %s: %v", ct, scopeField, err)
			continue
		}
		for _, v := range values {
			if !live[v] {
				r.prune(ct, scopeField, v, nil)
			}
		}
	}
	r.prune(parentType, "", "", keep)
}

func (r *runner) prune(resourceType, scopeField, scopeValue string, keep []string) {
	if err := r.db.Prune(resourceType, scopeField, scopeValue, keep); err != nil {
		r.warnf("removing %s the API no longer returns: %v", resourceType, err)
	}
}

func (r *runner) capReason() string {
	return fmt.Sprintf("stopped at --max-pages=%d with more pages remaining", r.opts.MaxPages)
}

// fleetGap records a fleet-wide list this run could not read in full.
func (r *runner) fleetGap(list, reason string) {
	r.fleetGaps = append(r.fleetGaps, r.recordGap("fleet", "", list, reason))
}

// tenantGap records one of a tenant's lists this run could not read in full
// and returns its row id.
func (r *runner) tenantGap(tid, list, reason string) string {
	return r.recordGap(tid, tid, list, reason)
}

func (r *runner) recordGap(scope, tid, list, reason string) string {
	label := "fleet " + list
	if tid != "" {
		label = "tenant " + tid + " " + list
	}
	r.sum.Incomplete = append(r.sum.Incomplete, label+": "+reason)
	return r.storeGap(scope, tid, list, reason)
}

// storeGap writes a gap row (recordGap without the run summary line).
func (r *runner) storeGap(scope, tid, list, reason string) string {
	obj := map[string]any{"gap_scope": scope, "list": list, "reason": reason, "recorded_at": r.opts.Now().UTC().Format(time.RFC3339)}
	if tid != "" {
		obj["tenant_id"] = tid
	}
	id := scope + ":" + list
	if data, err := json.Marshal(obj); err == nil {
		if err := r.db.Upsert(GapResourceType, id, data); err != nil {
			r.warnf("recording the incomplete %s: %v", id, err)
		}
	}
	return id
}

type orgDepth struct {
	id    string
	depth int
}

func (r *runner) syncTenant(ctx context.Context, tid string, winStart, winEnd time.Time) {
	base := "/api/v1/tenants/" + url.PathEscape(tid)
	var gaps []string
	defer func() { r.prune(GapResourceType, "gap_scope", tid, gaps) }()

	if data, err := r.get(ctx, base+"/quotas", nil); err != nil {
		r.warnf("tenant %s quotas: %v", tid, err)
		gaps = append(gaps, r.tenantGap(tid, "quotas", err.Error()))
	} else if obj, err := decode(data); err != nil {
		gaps = append(gaps, r.tenantGap(tid, "quotas", err.Error()))
	} else {
		var seen []string
		quotas, _ := obj["quotas"].([]any)
		for _, q := range quotas {
			qo, ok := q.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := qo["kind"].(string)
			qo["tenant_id"] = tid
			r.store("quotas", tid+":"+kind, qo)
			seen = append(seen, tid+":"+kind)
		}
		r.prune("quotas", "tenant_id", tid, seen)
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
			gaps = append(gaps, r.tenantGap(tid, l.rtype, "not read: --skip-archives; stored rows are from an earlier sync"))
			continue
		}
		items, complete, err := r.pageList(ctx, l.path, nil)
		if err != nil {
			r.warnf("tenant %s %s: %v", tid, l.rtype, err)
			gaps = append(gaps, r.tenantGap(tid, l.rtype, err.Error()))
			continue
		}
		var seen []string
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
			seen = append(seen, id)
		}
		if complete {
			r.prune(l.rtype, "tenant_id", tid, seen)
		} else {
			gaps = append(gaps, r.tenantGap(tid, l.rtype, r.capReason()))
		}
	}

	stats, err := r.get(ctx, base+"/tasks/statistics/summary", map[string]string{
		"start_time": winStart.Format(time.RFC3339),
		"end_time":   winEnd.Format(time.RFC3339),
	})
	if err != nil {
		r.warnf("tenant %s task stats: %v", tid, err)
		gaps = append(gaps, r.tenantGap(tid, "task_stats", err.Error()))
		return
	}
	obj, err := decode(stats)
	if err != nil {
		r.warnf("tenant %s task stats decode: %v", tid, err)
		gaps = append(gaps, r.tenantGap(tid, "task_stats", err.Error()))
		return
	}
	obj["tenant_id"] = tid
	obj["window_start"] = winStart.Format(time.RFC3339)
	obj["window_end"] = winEnd.Format(time.RFC3339)
	obj["fetched_at"] = r.opts.Now().UTC().Format(time.RFC3339)
	r.store("task_stats", tid, obj)
}

// pageList walks a paginated Afi list endpoint (limit/page_token ->
// next_page_token) until the token runs dry or MaxPages is hit. complete is
// false when it stopped at MaxPages with more pages remaining.
func (r *runner) pageList(ctx context.Context, path string, params map[string]string) ([]json.RawMessage, bool, error) {
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
			return out, false, err
		}
		var pageObj struct {
			Items         []json.RawMessage `json:"items"`
			NextPageToken string            `json:"next_page_token"`
		}
		if err := json.Unmarshal(data, &pageObj); err != nil {
			return out, false, fmt.Errorf("decoding page of %s: %w", path, err)
		}
		out = append(out, pageObj.Items...)
		if pageObj.NextPageToken == "" {
			return out, true, nil
		}
		token = pageObj.NextPageToken
	}
	r.warnf("%s: stopped at max-pages=%d with more pages remaining", path, r.opts.MaxPages)
	return out, false, nil
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
