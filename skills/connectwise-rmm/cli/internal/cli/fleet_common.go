// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared IO for the `fleet` commands (store reads, name joins,
// batched heartbeat, cached per-endpoint fan-out).

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/client"
	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/fleet"
	"connectwise-rmm-pp-cli/internal/store"
)

const (
	cachePatchSummary = "endpoint_patch_summary"
	cacheSystemState  = "endpoint_system_state"
	cacheTaskRun      = "task_run_targets"
	// heartbeatBatch is the documented max endpoints per heartbeat call.
	heartbeatBatch = 400
	// fanoutWorkers stays low: Asio rate limits are per token and per family.
	fanoutWorkers = 4
)

// fetchFailure is one per-endpoint (or per-batch) fetch that failed.
type fetchFailure struct {
	ID    string   `json:"id"`
	Error string   `json:"error"`
	IDs   []string `json:"-"` // endpoints covered by a failed batch call
}

// openFleetStore opens the local mirror. missing is true when no mirror exists
// yet; callers then emit an empty result plus a sync hint instead of failing.
func openFleetStore(ctx context.Context, cmd *cobra.Command, dbPath string) (db *store.Store, missing bool, err error) {
	if dbPath == "" {
		dbPath = defaultDBPath("connectwise-rmm-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: connectwise-rmm-cli sync --resources companies,sites,endpoints --db %s\n", dbPath, dbPath)
		return nil, true, nil
	}
	db, err = store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	return db, false, nil
}

// noMirrorNote rides in the JSON of a no-store result so an agent never reads
// an empty list as a healthy fleet.
const noMirrorNote = "no local store yet; run 'connectwise-rmm-cli sync --resources companies,sites,endpoints' first"

// emptyResult prints an empty machine result (or a human note) for a missing mirror.
func emptyResult(cmd *cobra.Command, flags *rootFlags, v any) error {
	if flags != nil {
		// No API call ran: report the result as local, not the command's
		// declared pp:data-source (live).
		flags.agentSource = "local"
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), v, flags)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "No local data yet. Run sync first.")
	return nil
}

// scan drains one resource type into memory (drain-first: no queries run
// while the rows are open).
func scan(db *store.Store, resourceType string) ([]map[string]any, error) {
	out := make([]map[string]any, 0)
	err := db.ListScan(resourceType, func(_ string, data json.RawMessage) bool {
		if m := fleet.Object(data); m != nil {
			out = append(out, m)
		}
		return true
	})
	return out, err
}

// fleetNames returns company and site display names keyed by ID.
func fleetNames(db *store.Store) (companies, sites map[string]string, err error) {
	companies, sites = map[string]string{}, map[string]string{}
	cos, err := scan(db, "companies")
	if err != nil {
		return nil, nil, err
	}
	for _, c := range cos {
		name := fleet.Str(c, "friendlyName")
		if name == "" {
			name = fleet.Str(c, "name")
		}
		companies[fleet.Str(c, "id")] = name
	}
	ss, err := scan(db, "sites")
	if err != nil {
		return nil, nil, err
	}
	for _, s := range ss {
		name := fleet.Str(s, "friendlyName")
		if name == "" {
			name = fleet.Str(s, "name")
		}
		sites[fleet.Str(s, "id")] = name
	}
	return companies, sites, nil
}

// loadFleet returns every synced endpoint joined to company, site and
// (when cached from a system-state read) machine names.
func loadFleet(db *store.Store) ([]fleet.Endpoint, error) {
	raw, err := scan(db, "endpoints")
	if err != nil {
		return nil, fmt.Errorf("reading endpoints: %w", err)
	}
	companies, sites, err := fleetNames(db)
	if err != nil {
		return nil, fmt.Errorf("reading names: %w", err)
	}
	stateNames := map[string]string{}
	states, err := scan(db, cacheSystemState)
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		if st, ok := fleet.ParseSystemState(cachedPayload(s)); ok && st.Name != "" {
			stateNames[fleet.Str(s, "id")] = st.Name
		}
	}
	eps := make([]fleet.Endpoint, 0, len(raw))
	for _, m := range raw {
		e := fleet.ParseEndpoint(m)
		if e.ID == "" {
			continue
		}
		e.CompanyName = companies[e.CompanyID]
		e.SiteName = sites[e.SiteID]
		if e.Name == "" {
			e.Name = stateNames[e.ID]
		}
		eps = append(eps, e)
	}
	return eps, nil
}

// filterFleet keeps endpoints in a company (ID or case-insensitive name) and of a type.
func filterFleet(eps []fleet.Endpoint, company, endpointType string) []fleet.Endpoint {
	out := make([]fleet.Endpoint, 0, len(eps))
	for _, e := range eps {
		if company != "" && !strings.EqualFold(e.CompanyID, company) && !strings.EqualFold(e.CompanyName, company) {
			continue
		}
		if endpointType != "" && !strings.EqualFold(e.Type, endpointType) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// resolveEndpoint finds one endpoint by ID or case-insensitive name.
func resolveEndpoint(eps []fleet.Endpoint, ref string) (fleet.Endpoint, error) {
	ref = strings.TrimSpace(ref)
	var byName []fleet.Endpoint
	for _, e := range eps {
		if strings.EqualFold(e.ID, ref) {
			return e, nil
		}
		if e.Name != "" && strings.EqualFold(e.Name, ref) {
			byName = append(byName, e)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return fleet.Endpoint{}, notFoundErr(fmt.Errorf("endpoint %q not found in the local store; pass the endpoint ID, or run 'fleet whois' to cache machine names", ref))
	}
	return fleet.Endpoint{}, usageErr(fmt.Errorf("endpoint name %q matches %d endpoints; pass the endpoint ID instead", ref, len(byName)))
}

// fetchHeartbeats calls the v2 heartbeat API in 400-ID batches, bypassing the
// HTTP response cache (availability is a live question). Failed batches and
// per-endpoint failedRecords both come back as failures carrying the IDs whose
// availability is unknown.
func fetchHeartbeats(ctx context.Context, c *client.Client, ids []string) (map[string]fleet.Heartbeat, []fetchFailure) {
	out := map[string]fleet.Heartbeat{}
	failures := make([]fetchFailure, 0)
	for start := 0; start < len(ids); start += heartbeatBatch {
		end := start + heartbeatBatch
		if end > len(ids) {
			end = len(ids)
		}
		raw, err := c.GetNoCache(ctx, "/api/platform/v2/device/endpoints/heartbeat", map[string]string{
			"resourceType": "endpoints",
			"resources":    strings.Join(ids[start:end], ","),
		})
		if err != nil {
			failures = append(failures, fetchFailure{ID: fmt.Sprintf("heartbeat batch %d-%d", start, end-1), Error: err.Error(), IDs: ids[start:end]})
			continue
		}
		hb, failed, ok := fleet.ParseHeartbeats(raw)
		if !ok {
			failures = append(failures, fetchFailure{ID: fmt.Sprintf("heartbeat batch %d-%d", start, end-1), Error: "unreadable heartbeat response", IDs: ids[start:end]})
			continue
		}
		for id, h := range hb {
			out[id] = h
		}
		for id, msg := range failed {
			failures = append(failures, fetchFailure{ID: id, Error: "heartbeat: " + msg, IDs: []string{id}})
		}
	}
	return out, failures
}

// unknownIDs is the set of endpoints a failure left without an answer.
func unknownIDs(failures []fetchFailure) map[string]bool {
	out := map[string]bool{}
	for _, f := range failures {
		for _, id := range f.IDs {
			out[id] = true
		}
	}
	return out
}

// allHeartbeatsFailed reports a heartbeat read that answered nothing at all.
func allHeartbeatsFailed(hb map[string]fleet.Heartbeat, failures []fetchFailure) error {
	if len(hb) == 0 && len(failures) > 0 {
		return classifyAPIErrorOnly(fmt.Errorf("heartbeat API: %s", failures[0].Error))
	}
	return nil
}

// unsyncedNote explains an empty scope so an agent never reads it as "all clear".
func unsyncedNote(scanned int) string {
	if scanned == 0 {
		return "no endpoints in scope in the local store; run 'connectwise-rmm-cli sync --resources companies,sites,endpoints' (or check --company)"
	}
	return ""
}

// cachedPayload unwraps a cache row written by cachedEndpointData. The payload
// is stored as a JSON string so the store's recursive merge replaces it whole.
func cachedPayload(m map[string]any) json.RawMessage {
	if s, ok := m["payload"].(string); ok {
		return json.RawMessage(s)
	}
	b, _ := json.Marshal(m["data"])
	return b
}

// cachedEndpointData returns per-endpoint payloads of one kind, serving rows
// younger than ttl from the local cache and fetching the rest (bounded fan-out).
// Fresh fetches are written back after the fan-out completes, outside any
// open transaction. A failed fetch is reported, never cached.
func cachedEndpointData(ctx context.Context, cmd *cobra.Command, db *store.Store, c *client.Client, kind string, ids []string, ttl time.Duration, refresh bool, path func(id string) string) (map[string]json.RawMessage, []fetchFailure) {
	out := map[string]json.RawMessage{}
	need := make([]string, 0)
	cached := map[string]map[string]any{}
	if rows, err := scan(db, kind); err == nil {
		for _, r := range rows {
			cached[fleet.Str(r, "id")] = r
		}
	}
	for _, id := range ids {
		if r, ok := cached[id]; ok && !refresh && time.Since(fleet.ParseTime(fleet.Str(r, "fetchedAt"))) < ttl {
			out[id] = cachedPayload(r)
			continue
		}
		need = append(need, id)
	}
	failures := make([]fetchFailure, 0)
	if len(need) == 0 {
		return out, failures
	}
	fetch := func(ctx context.Context, id string) (json.RawMessage, error) {
		// Our own cache decides freshness; never serve the HTTP response cache.
		return c.GetNoCache(ctx, path(id), nil)
	}
	// One serial fetch first so the token is minted before workers start.
	results := make([]cliutil.FanoutResult[json.RawMessage], 0, len(need))
	var errs []cliutil.FanoutError
	if v, err := fetch(ctx, need[0]); err != nil {
		errs = append(errs, cliutil.FanoutError{Source: need[0], Err: err})
	} else {
		results = append(results, cliutil.FanoutResult[json.RawMessage]{Source: need[0], Value: v})
	}
	if len(need) > 1 {
		more, moreErrs := cliutil.FanoutRun(ctx, need[1:], func(id string) string { return id }, fetch, cliutil.WithConcurrency(fanoutWorkers))
		results = append(results, more...)
		errs = append(errs, moreErrs...)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		out[r.Source] = r.Value
		row, _ := json.Marshal(map[string]any{"id": r.Source, "fetchedAt": now, "payload": string(r.Value)})
		if err := db.Upsert(kind, r.Source, row); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: caching %s for %s failed: %v\n", kind, r.Source, err)
		}
	}
	for _, e := range errs {
		failures = append(failures, fetchFailure{ID: e.Source, Error: e.Err.Error()})
	}
	return out, failures
}

// liveEndpointData fetches one endpoint payload live, caching it when a local
// store exists (brief works without one when given an endpoint ID).
func liveEndpointData(ctx context.Context, cmd *cobra.Command, db *store.Store, c *client.Client, kind, id string, path func(string) string) (map[string]json.RawMessage, []fetchFailure) {
	if db != nil {
		return cachedEndpointData(ctx, cmd, db, c, kind, []string{id}, 0, true, path)
	}
	raw, err := c.GetNoCache(ctx, path(id), nil)
	if err != nil {
		return map[string]json.RawMessage{}, []fetchFailure{{ID: id, Error: err.Error(), IDs: []string{id}}}
	}
	return map[string]json.RawMessage{id: raw}, nil
}

func patchPath(id string) string {
	return "/api/platform/v1/patching/" + url.PathEscape(id) + "/patches"
}

func systemStatePath(id string) string {
	return "/api/platform/v1/device/endpoints/" + url.PathEscape(id) + "/system-state-info"
}

// patchSummaries parses cached-or-fetched patch payloads; a payload without a
// readable summary is reported, never treated as a clean endpoint.
func patchSummaries(raw map[string]json.RawMessage) (map[string]fleet.PatchSummary, []fetchFailure) {
	out := map[string]fleet.PatchSummary{}
	bad := make([]fetchFailure, 0)
	for id, r := range raw {
		if s, ok := fleet.ParsePatchSummary(r); ok {
			out[id] = s
		} else {
			bad = append(bad, fetchFailure{ID: id, Error: "patches: no readable summary", IDs: []string{id}})
		}
	}
	return out, bad
}

// systemStates parses cached-or-fetched system-state payloads. Unreadable
// payloads are reported, never treated as "nobody logged on".
func systemStates(raw map[string]json.RawMessage) (map[string]fleet.SystemState, []fetchFailure) {
	out := map[string]fleet.SystemState{}
	bad := make([]fetchFailure, 0)
	for id, r := range raw {
		if st, ok := fleet.ParseSystemState(r); ok {
			out[id] = st
		} else {
			bad = append(bad, fetchFailure{ID: id, Error: "system state: unreadable payload", IDs: []string{id}})
		}
	}
	return out, bad
}

// capEndpoints bounds per-endpoint fan-out; dogfood runs get a tiny cap so the
// 30s matrix budget holds. Returns the kept slice and whether the cap hit.
func capEndpoints(eps []fleet.Endpoint, max int) ([]fleet.Endpoint, bool) {
	if cliutil.IsDogfoodEnv() && (max <= 0 || max > 5) {
		max = 5
	}
	if max > 0 && len(eps) > max {
		return eps[:max], true
	}
	return eps, false
}

func endpointIDs(eps []fleet.Endpoint) []string {
	ids := make([]string, len(eps))
	for i, e := range eps {
		ids[i] = e.ID
	}
	return ids
}

func warnFailures(cmd *cobra.Command, what string, failures []fetchFailure, total int) {
	if len(failures) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d %s fetches failed; results exclude them (see fetch_failures)\n", len(failures), total, what)
	}
}

// parseDurationFlag parses a d/w-aware duration flag, naming the flag on error.
func parseDurationFlag(name, value string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil || d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a positive duration (try 24h, 7d or 2w)", name, value))
	}
	return d, nil
}
