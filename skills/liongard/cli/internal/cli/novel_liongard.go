// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared helpers for Liongard's store-backed transcendence commands (drift,
// launchpoints stale / run-stale, environments overview, agents offline,
// coverage, metrics pivot / breach). These read the local SQLite store the
// generated `sync` command hydrates and perform cross-entity joins the live
// Liongard API never returns in a single call.
//
// Field extraction is intentionally defensive: Liongard objects use PascalCase
// keys, but exact casing and cross-entity foreign-key names vary by resource
// and API version, so every accessor tries several candidate keys (and a
// case-insensitive fallback). Missing data yields empty results with an honest
// reason, never fabricated rows. The exact field mapping is confirmed by real
// tenant USE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"liongard-pp-cli/internal/store"
)

// Liongard resource_type keys as written into the generic `resources` table by
// the generated UpsertXxx functions. Listing by these keys returns the full
// synced JSON object for each entity.
const (
	rtEnvironments = "environments"
	rtSystems      = "systems"
	rtLaunchpoints = "launchpoints"
	rtAgents       = "agents"
	rtDetections   = "detections"
	rtTimeline     = "timeline"
	rtMetrics      = "metrics"
)

// novelListCap is far above any realistic per-resource estate size; it exists
// only to override store.List's default 200-row cap so full-estate joins see
// every synced row.
const novelListCap = 1000000

// openNovelStore opens the local store, returning an actionable error that
// points the user at `sync` when the DB is missing.
func openNovelStore(cmd *cobra.Command, flags *rootFlags) (*store.Store, error) {
	if flags.dataSource == "live" {
		return nil, fmt.Errorf("this command reads only synced local data and has no live equivalent; drop --data-source live (auto and local both work) and run 'liongard-cli sync' to refresh the local copy")
	}
	dbPath := defaultDBPath("liongard-cli")
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'liongard-cli sync' first.", err)
	}
	maybeEmitSyncHints(cmd, db, "", flags.maxAge)
	return db, nil
}

// loadObjs lists and decodes every synced object of the given resource_type.
// It refuses a resource whose local copy is not a complete snapshot (Codex r1
// class incomplete-snapshot-read-as-complete: with the timeline never synced,
// run-stale read "no timeline row" as "never inspected" and triggered runs),
// whose last finished sync was narrowed (Codex r2: a fields[]-projected
// agents sync left no LastHeartbeat and `agents offline` reported zero), a
// row that does not decode, and a row missing a field the commands need.
// Every novel reads the store through here, so none reads a row that was
// never fetched, or a field that was never fetched, as one that is absent.
func loadObjs(db *store.Store, resourceType string) ([]map[string]any, error) {
	if err := requireCompleteSnapshot(db, resourceType); err != nil {
		return nil, err
	}
	raws, err := db.List(resourceType, novelListCap)
	if err != nil {
		return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
	}
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		var m map[string]any
		if err := json.Unmarshal(r, &m); err != nil || m == nil {
			return nil, fmt.Errorf("%w of %s: a stored row does not decode as an object; %s", errIncompleteSnapshot, resourceType, fullSyncRemedy(resourceType))
		}
		if isMetricEvaluationRow(resourceType, m) {
			continue
		}
		out = append(out, m)
	}
	if err := requireNovelFields(resourceType, out); err != nil {
		return nil, err
	}
	return out, nil
}

// errIncompleteSnapshot marks a novel refused because a resource it reads has
// no complete local copy.
var errIncompleteSnapshot = errors.New("the local store has no complete copy")

// fullSyncRemedy names the sync that gives a resource a full local copy.
// --full drops any resume cursor and since window; the narrowing flags are
// named so the user does not repeat them.
func fullSyncRemedy(resourceType string) string {
	return fmt.Sprintf("run 'liongard-cli sync --full --resources %s' (without --param, --global-param, --resource-param, --since or --max-pages) and try again", resourceType)
}

// requireCompleteSnapshot passes only a resource whose last sync finished
// as a full, unnarrowed snapshot (sync_state: last_attempt_complete = 1 with
// a sync time, and snapshot_full = 1). Any other signal refuses: no sync
// record (rows a live read cached are one page, not the estate), a sync that
// stopped early (a --max-pages cap, an error, a stuck cursor) or was reset
// for another tenant, a finished sync that was narrowed (fields[],
// conditions[], any --param, --global-param or --resource-param, a since
// window over a partial copy, a resumed cursor), or a sync state that cannot
// be read.
func requireCompleteSnapshot(db *store.Store, resourceType string) error {
	snap, err := db.SyncSnapshotState(resourceType)
	if err != nil {
		return fmt.Errorf("%w of %s: reading its sync state: %v", errIncompleteSnapshot, resourceType, err)
	}
	if snap.Full {
		return nil
	}
	if !snap.Recorded && !slicesContains(knownSyncResourceNames(), resourceType) {
		return fmt.Errorf("%w of %s: this CLI's sync does not fetch %s yet, so this command cannot answer from local data (rows a live read cached are not the whole estate)", errIncompleteSnapshot, resourceType, resourceType)
	}
	if snap.Complete {
		return fmt.Errorf("%w of %s: its last finished sync was narrowed (query parameters such as fields[] or conditions[], a since window, a resumed or capped walk), so the store holds only some of its rows or fields; %s", errIncompleteSnapshot, resourceType, fullSyncRemedy(resourceType))
	}
	return fmt.Errorf("%w of %s (never synced, or its last sync did not finish); %s", errIncompleteSnapshot, resourceType, fullSyncRemedy(resourceType))
}

// novelFieldGroups are the documented fields (spec.yaml components/schemas)
// the store-backed commands read from each resource, where a missing field
// changes an answer instead of erroring: no LastHeartbeat reads as "not
// offline", no Launchpoint as "uninspected", no run timestamps as "never
// inspected" (and a run-stale target), no Status as "not failed". Each group
// lists the spellings the commands accept for one field (lgGet: a nested
// reference or its scalar ID, any case); a row passes when every group has
// at least one key present, null included (a documented nullable field the
// API returned as null is data; a key a projection dropped is not).
var novelFieldGroups = map[string][][]string{
	rtAgents:       {{"ID"}, {"Environment", "EnvironmentID"}, {"LastHeartbeat"}},
	rtEnvironments: {{"ID"}, {"Name"}},
	rtSystems:      {{"ID"}, {"Environment", "EnvironmentID"}, {"Launchpoint", "LaunchpointID"}, {"Inspector", "InspectorID"}},
	rtLaunchpoints: {{"ID"}, {"Environment", "EnvironmentID"}, {"Inspector", "InspectorID"}, {"LastScheduledRun"}, {"LastAdHocRun"}, {"TotalScheduledRuns"}, {"TotalAdHocRuns"}},
	rtTimeline:     {{"ID"}, {"Environment", "EnvironmentID"}, {"Launchpoint", "LaunchpointID"}, {"System", "SystemID"}, {"Status"}, {"FinishedAt"}, {"CreatedOn"}},
	rtDetections:   {{"ID"}, {"Environment", "EnvironmentID"}, {"System", "SystemID"}, {"CreatedOn"}},
	rtMetrics:      {{"ID"}, {"Name"}, {"Inspector", "InspectorID"}, {"Environment", "EnvironmentID"}},
	rtInspectors:   {{"ID"}, {"Name"}},
}

// requireNovelFields refuses the resource when any row lacks a field group
// (novelFieldGroups). A resource with no listed groups passes.
func requireNovelFields(resourceType string, rows []map[string]any) error {
	groups := novelFieldGroups[resourceType]
	if len(groups) == 0 {
		return nil
	}
	missing := map[string]int{}
	for _, row := range rows {
		for _, g := range groups {
			if !rowHasAnyKey(row, g) {
				missing[g[0]]++
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	for name, n := range missing {
		names = append(names, fmt.Sprintf("%s (%d of %d rows)", name, n, len(rows)))
	}
	sort.Strings(names)
	return fmt.Errorf("%w of %s: stored rows lack %s, so this command cannot answer from them (a projected or filtered sync or live read stored them); %s. If rows still lack it after a full sync, Liongard did not return the field and this command cannot answer from local data", errIncompleteSnapshot, resourceType, strings.Join(names, ", "), fullSyncRemedy(resourceType))
}

// isMetricEvaluationRow reports a row of the metrics resource that is a
// metric evaluation, not a metric definition: `metrics evaluation` reads
// GET /metrics/bulk under the metrics resource, and its live-read cache
// stores each documented components/schemas/MetricEvaluation entry
// ({"TimelineID":..., "Metric":{"ID","Name","err","value"}}) beside the
// definitions. It is not a definition with fields missing, so it is
// skipped rather than refused. Closed rule: no ID, a Metric object and a
// TimelineID.
func isMetricEvaluationRow(resourceType string, row map[string]any) bool {
	if resourceType != rtMetrics || rowHasAnyKey(row, []string{"ID"}) {
		return false
	}
	_, hasMetric := lgGet(row, "Metric").(map[string]any)
	return hasMetric && rowHasAnyKey(row, []string{"TimelineID"})
}

// rowHasAnyKey reports whether row carries one of keys (any case), null
// values included.
func rowHasAnyKey(row map[string]any, keys []string) bool {
	for _, k := range keys {
		if _, ok := row[k]; ok {
			return true
		}
	}
	for rowKey := range row {
		for _, k := range keys {
			if strings.EqualFold(rowKey, k) {
				return true
			}
		}
	}
	return false
}

func slicesContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// lastRunByLaunchpoint is each launchpoint's newest known inspection: the
// newest timeline entry that references it, or the launchpoint's own
// documented last-run timestamps (components/schemas/Launchpoints
// LastScheduledRun and LastAdHocRun), whichever is newer. The timeline alone
// misses a run older than the timeline the store holds.
func lastRunByLaunchpoint(timeline, launchpoints []map[string]any) map[string]time.Time {
	latest := map[string]time.Time{}
	seen := func(id string, ts time.Time) {
		if cur, ok := latest[id]; !ok || ts.After(cur) {
			latest[id] = ts
		}
	}
	for _, t := range timeline {
		if lpID := lgRefID(t, "Launchpoint", "LaunchpointID"); lpID != "" {
			if ts, ok := lgTime(t, tsKeysInspection...); ok {
				seen(lpID, ts)
			}
		}
	}
	for _, lp := range launchpoints {
		id := lgID(lp)
		if id == "" {
			continue
		}
		for _, k := range launchpointLastRunKeys {
			if ts, ok := lgTime(lp, k); ok {
				seen(id, ts)
			}
		}
	}
	return latest
}

// launchpointLastRunKeys are the launchpoint's own documented last-run
// timestamps (components/schemas/Launchpoints).
var launchpointLastRunKeys = []string{"LastScheduledRun", "LastAdHocRun"}

// ranBefore reports a launchpoint whose documented run counters
// (components/schemas/Launchpoints TotalScheduledRuns, TotalAdHocRuns) say it
// has run at least once.
func ranBefore(lp map[string]any) bool {
	for _, k := range []string{"TotalScheduledRuns", "TotalAdHocRuns"} {
		if n, ok := lgFloat(lp, k); ok && n > 0 {
			return true
		}
	}
	return false
}

// lgGet returns the first present, non-nil value among keys, falling back to a
// case-insensitive match so callers can list canonical PascalCase keys without
// worrying about exact casing.
func lgGet(obj map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			return v
		}
	}
	for objKey, v := range obj {
		if v == nil {
			continue
		}
		for _, k := range keys {
			if strings.EqualFold(objKey, k) {
				return v
			}
		}
	}
	return nil
}

// lgStr coerces the first present value among keys to a string. Integer-valued
// JSON numbers render without a decimal point so IDs join cleanly. Nested
// objects (e.g. an embedded Environment) collapse to their Name/ID.
func lgStr(obj map[string]any, keys ...string) string {
	v := lgGet(obj, keys...)
	return coerceStr(v)
}

func coerceStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case map[string]any:
		if s := lgStr(t, "Name", "name", "Label", "label", "ID", "Id", "id"); s != "" {
			return s
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// lgFloat extracts a numeric value, accepting JSON numbers or numeric strings
// (Liongard and many feeds encode scalars as strings). ok is false on
// missing/null/unparseable.
func lgFloat(obj map[string]any, keys ...string) (float64, bool) {
	switch t := lgGet(obj, keys...).(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

// lgTime parses the first present timestamp among keys across the layouts
// Liongard responses are observed to use.
func lgTime(obj map[string]any, keys ...string) (time.Time, bool) {
	switch t := lgGet(obj, keys...).(type) {
	case string:
		s := strings.TrimSpace(t)
		for _, layout := range []string{
			// Liongard responses mix ISO 8601 ("2021-09-10T19:21:03.195Z") and a
			// slash-separated form ("2021/09/10 19:21:03"); accept both plus common
			// variants so time-window filters actually match.
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02T15:04:05.999999Z07:00",
			"2006-01-02T15:04:05.999999",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006/01/02 15:04:05",
			"2006/01/02T15:04:05",
			"2006-01-02",
			"2006/01/02",
		} {
			if ts, err := time.Parse(layout, s); err == nil {
				// Normalize to UTC so RFC3339-formatted strings sort by instant
				// even when sources mix timezone offsets.
				return ts.UTC(), true
			}
		}
	case float64:
		// Treat as Unix seconds (or ms when clearly out of second-range).
		secs := int64(t)
		if secs > 1e12 {
			secs /= 1000
		}
		return time.Unix(secs, 0).UTC(), true
	}
	return time.Time{}, false
}

// lgID returns the canonical identifier of an object.
func lgID(obj map[string]any) string {
	return lgStr(obj, "ID", "Id", "id")
}

// lgRefID resolves a reference to another entity to that entity's ID. Liongard
// carries cross-entity references as nested {ID,Name} objects (e.g. a System's
// "Environment") and sometimes as scalar IDs (e.g. "EnvironmentID"). It returns
// the referenced entity's ID whether the value is an object or a scalar.
func lgRefID(obj map[string]any, keys ...string) string {
	switch t := lgGet(obj, keys...).(type) {
	case map[string]any:
		return lgStr(t, "ID", "Id", "id")
	case nil:
		return ""
	default:
		return coerceStr(t)
	}
}

// lgRefName resolves a reference to another entity to that entity's display
// name (Name/Alias), falling back to its ID for scalar references.
func lgRefName(obj map[string]any, keys ...string) string {
	switch t := lgGet(obj, keys...).(type) {
	case map[string]any:
		return lgStr(t, "Name", "name", "Alias", "alias", "Label", "label")
	case nil:
		return ""
	default:
		return coerceStr(t)
	}
}

// parseLookbackDuration parses durations including the day suffix "Nd" that
// time.ParseDuration does not support (e.g. "24h", "7d", "30m", "90m"). It
// returns a Duration (distinct from sync.go's parseSinceDuration, which returns
// a cutoff time.Time) because the novel commands need the span itself.
func parseLookbackDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration; use a value like 24h, 7d, or 90m")
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid days duration %q; use a value like 7d", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q; use a value like 24h, 7d, or 90m", s)
	}
	return d, nil
}

// envNameIndex builds id->Name and name->id lookups for environments so other
// joins can resolve an environment reference whether it is carried as an ID or
// a name.
func envNameIndex(envs []map[string]any) (byID map[string]string, byName map[string]string) {
	byID = map[string]string{}
	byName = map[string]string{}
	for _, e := range envs {
		id := lgID(e)
		name := lgStr(e, "Name", "name")
		if id != "" {
			byID[id] = name
		}
		if name != "" {
			byName[strings.ToLower(name)] = id
		}
	}
	return byID, byName
}

// staleLaunchpoint is one launchpoint that has not produced a fresh inspection.
type staleLaunchpoint struct {
	LaunchpointID  string  `json:"launchpoint_id"`
	Alias          string  `json:"alias,omitempty"`
	Inspector      string  `json:"inspector,omitempty"`
	EnvironmentID  string  `json:"environment_id,omitempty"`
	Environment    string  `json:"environment,omitempty"`
	Status         string  `json:"status,omitempty"`
	LastInspection string  `json:"last_inspection,omitempty"`
	DaysStale      float64 `json:"days_stale"`
	NeverInspected bool    `json:"never_inspected"`
}

// computeStaleLaunchpoints joins launchpoints to the newest timeline entry that
// references them (via the timeline's Launchpoint object) and returns those
// whose latest inspection is older than cutoff (or that have no inspection at
// all). envFilter, when non-empty, restricts to a single environment id.
func computeStaleLaunchpoints(db *store.Store, olderThan time.Duration, envFilter string) ([]staleLaunchpoint, error) {
	launchpoints, err := loadObjs(db, rtLaunchpoints)
	if err != nil {
		return nil, err
	}
	timeline, err := loadObjs(db, rtTimeline)
	if err != nil {
		return nil, err
	}
	envs, err := loadObjs(db, rtEnvironments)
	if err != nil {
		return nil, err
	}
	envByID, _ := envNameIndex(envs)

	// Newest inspection per launchpoint: the timeline entries that reference
	// it and its own documented last-run timestamps.
	latestByLP := lastRunByLaunchpoint(timeline, launchpoints)

	now := time.Now().UTC()
	cutoff := now.Add(-olderThan)
	var out []staleLaunchpoint
	for _, lp := range launchpoints {
		id := lgID(lp)
		if id == "" {
			continue
		}
		envID := lgRefID(lp, "Environment", "EnvironmentID")
		if envFilter != "" && envID != envFilter {
			continue
		}
		envName := lgRefName(lp, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		item := staleLaunchpoint{
			LaunchpointID: id,
			Alias:         lgStr(lp, "Alias", "Name", "name"),
			Inspector:     lgRefName(lp, "Inspector"),
			EnvironmentID: envID,
			Environment:   envName,
			Status:        lgStr(lp, "Status", "State"),
		}
		last, seen := latestByLP[id]
		if !seen {
			// No run time anywhere. Its documented run counts
			// (TotalScheduledRuns, TotalAdHocRuns) above zero say it did run:
			// there is no evidence it is stale, so it is neither "never
			// inspected" nor a run-stale target (fail closed for the write).
			if ranBefore(lp) {
				continue
			}
			item.NeverInspected = true
			item.DaysStale = -1
			out = append(out, item)
			continue
		}
		if last.Before(cutoff) {
			item.LastInspection = last.Format(time.RFC3339)
			item.DaysStale = roundTo(now.Sub(last).Hours()/24, 1)
			out = append(out, item)
		}
	}
	// Most stale first; never-inspected (DaysStale -1) sort to the end unless
	// we special-case them, so push them to the top as the most urgent.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NeverInspected != out[j].NeverInspected {
			return out[i].NeverInspected
		}
		return out[i].DaysStale > out[j].DaysStale
	})
	return out, nil
}

func roundTo(v float64, places int) float64 {
	p := 1.0
	for i := 0; i < places; i++ {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}

// ctxOf returns the command context or a background context as a fallback.
func ctxOf(cmd *cobra.Command) context.Context {
	if c := cmd.Context(); c != nil {
		return c
	}
	return context.Background()
}

// tsKeysInspection / tsKeysDetection are the canonical timestamp-key
// candidate lists for timeline (inspection) and detection objects. Every
// call site spreads the same var so the ordering can never silently diverge
// (the environments-overview drift bug class).
var tsKeysInspection = []string{"FinishedAt", "CompletedOn", "CompletedAt", "CreatedOn", "UpdatedOn", "ScheduledAt"}
var tsKeysDetection = []string{"CreatedOn", "DetectedOn", "CreatedAt", "Date", "UpdatedOn"}

// metricValueKeys are the fields a Liongard metric object may carry an
// evaluated value under. Definitions-only objects carry none.
var metricValueKeys = []string{"Value", "Result", "MetricValue", "Output", "CurrentValue", "LastValue", "NumericValue", "value"}

// metricRow is one (metric, system) pair from the cross-system metric pivot.
type metricRow struct {
	Metric        string   `json:"metric"`
	MetricID      string   `json:"metric_id,omitempty"`
	InspectorID   string   `json:"inspector_id,omitempty"`
	SystemID      string   `json:"system_id,omitempty"`
	System        string   `json:"system,omitempty"`
	EnvironmentID string   `json:"environment_id,omitempty"`
	Environment   string   `json:"environment,omitempty"`
	Value         string   `json:"value,omitempty"`
	NumericValue  *float64 `json:"numeric_value,omitempty"`
	HasValue      bool     `json:"has_value"`
}

// buildMetricRows pivots a named metric across the systems it applies to,
// reading the local store. It matches metric definitions by case-insensitive
// substring on Name, then resolves the systems each metric covers: directly
// from a SystemID on the metric object when present, otherwise via the metric's
// InspectorID -> launchpoints (same inspector) -> system. Evaluated values are
// populated when the synced metric object carries them. Returns the rows, how
// many metric definitions matched, and whether any row carried a value.
func buildMetricRows(db *store.Store, nameQuery string) (rows []metricRow, matchedDefs int, anyValue bool, err error) {
	metrics, err := loadObjs(db, rtMetrics)
	if err != nil {
		return nil, 0, false, err
	}
	systems, err := loadObjs(db, rtSystems)
	if err != nil {
		return nil, 0, false, err
	}
	envs, err := loadObjs(db, rtEnvironments)
	if err != nil {
		return nil, 0, false, err
	}
	envByID, _ := envNameIndex(envs)

	// A Liongard metric is scoped to an Environment + Inspector; the systems it
	// covers are those carrying the same Inspector (and Environment, when the
	// metric names one). Systems carry both as nested objects.
	type sysScope struct {
		id, name, inspID, envID, envName string
	}
	sysScopes := make([]sysScope, 0, len(systems))
	for _, s := range systems {
		id := lgID(s)
		if id == "" {
			continue
		}
		envID := lgRefID(s, "Environment", "EnvironmentID")
		envName := lgRefName(s, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		sysScopes = append(sysScopes, sysScope{
			id:      id,
			name:    lgStr(s, "Name", "name", "Hostname", "FQDN"),
			inspID:  lgRefID(s, "Inspector", "InspectorID"),
			envID:   envID,
			envName: envName,
		})
	}

	q := strings.ToLower(strings.TrimSpace(nameQuery))
	mkRow := func(metric map[string]any, sc *sysScope) metricRow {
		r := metricRow{
			Metric:      lgStr(metric, "Name", "name"),
			MetricID:    lgID(metric),
			InspectorID: lgRefID(metric, "Inspector", "InspectorID"),
		}
		if sc != nil {
			r.SystemID = sc.id
			r.System = sc.name
			r.EnvironmentID = sc.envID
			r.Environment = sc.envName
		}
		// Per-system metric values are evaluated server-side and are not part of
		// the metric definition object; populate a value only if one is present.
		if f, ok := lgFloat(metric, metricValueKeys...); ok {
			v := f
			r.NumericValue = &v
			r.HasValue = true
			r.Value = coerceStr(lgGet(metric, metricValueKeys...))
		} else if s := lgStr(metric, metricValueKeys...); s != "" {
			r.Value = s
			r.HasValue = true
		}
		return r
	}

	for _, m := range metrics {
		name := strings.ToLower(lgStr(m, "Name", "name"))
		if q != "" && !strings.Contains(name, q) {
			continue
		}
		matchedDefs++
		inspID := lgRefID(m, "Inspector", "InspectorID")
		mEnvID := lgRefID(m, "Environment", "EnvironmentID")
		expanded := false
		for i := range sysScopes {
			sc := &sysScopes[i]
			if inspID != "" && sc.inspID != inspID {
				continue
			}
			if mEnvID != "" && sc.envID != mEnvID {
				continue
			}
			expanded = true
			r := mkRow(m, sc)
			anyValue = anyValue || r.HasValue
			rows = append(rows, r)
		}
		if !expanded {
			// Definition with no resolvable covered system yet — surface it anyway.
			r := mkRow(m, nil)
			anyValue = anyValue || r.HasValue
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Metric != rows[j].Metric {
			return rows[i].Metric < rows[j].Metric
		}
		return rows[i].System < rows[j].System
	})
	return rows, matchedDefs, anyValue, nil
}

// compareMetric evaluates a numeric comparison op against a threshold.
func compareMetric(v float64, op string, threshold float64) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "gt", ">":
		return v > threshold
	case "ge", ">=", "gte":
		return v >= threshold
	case "lt", "<":
		return v < threshold
	case "le", "<=", "lte":
		return v <= threshold
	case "eq", "==", "=":
		return v == threshold
	case "ne", "!=", "<>":
		return v != threshold
	default:
		return false
	}
}

// --- Pure join functions (testable; the commands load objects then call these) ---

// filterDriftRows resolves detections within [now-since, now] to drift rows
// joined to their owning environment and system, newest first, optionally
// filtered to one environment and capped.
func filterDriftRows(detections, envs []map[string]any, since time.Duration, now time.Time, envFilter string, limit int) []driftRow {
	envByID, _ := envNameIndex(envs)
	cutoff := now.Add(-since)
	rows := []driftRow{}
	for _, d := range detections {
		ts, ok := lgTime(d, tsKeysDetection...)
		if !ok || ts.Before(cutoff) {
			continue
		}
		envID := lgRefID(d, "Environment", "EnvironmentID")
		envName := lgRefName(d, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		if envFilter != "" && envID != envFilter {
			continue
		}
		rows = append(rows, driftRow{
			DetectionID:   lgID(d),
			Name:          lgStr(d, "Name", "name", "Detection", "Description", "Title"),
			CreatedOn:     ts.Format(time.RFC3339),
			TimelineID:    lgStr(d, "TimelineID", "Timeline", "TimelineId"),
			SystemID:      lgRefID(d, "System", "SystemID"),
			System:        lgRefName(d, "System"),
			EnvironmentID: envID,
			Environment:   envName,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedOn > rows[j].CreatedOn })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

type coverageSysGap struct {
	SystemID      string `json:"system_id"`
	System        string `json:"system,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}
type coverageEnvGap struct {
	EnvironmentID string `json:"environment_id"`
	Environment   string `json:"environment,omitempty"`
}

// findCoverageGaps returns systems with no bound launchpoint and environments
// with no systems, from the synced objects.
func findCoverageGaps(systems, envs []map[string]any, envFilter string) ([]coverageSysGap, []coverageEnvGap) {
	envByID, _ := envNameIndex(envs)
	envsWithSystem := map[string]bool{}
	uninspected := []coverageSysGap{}
	for _, s := range systems {
		envID := lgRefID(s, "Environment", "EnvironmentID")
		if envID != "" {
			envsWithSystem[envID] = true
		}
		if lgRefID(s, "Launchpoint", "LaunchpointID") != "" {
			continue
		}
		id := lgID(s)
		if id == "" {
			continue
		}
		if envFilter != "" && envID != envFilter {
			continue
		}
		envName := lgRefName(s, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		uninspected = append(uninspected, coverageSysGap{
			SystemID:      id,
			System:        lgStr(s, "Name", "name", "Hostname", "FQDN"),
			EnvironmentID: envID,
			Environment:   envName,
		})
	}
	emptyEnvs := []coverageEnvGap{}
	for _, e := range envs {
		id := lgID(e)
		if id == "" || envsWithSystem[id] {
			continue
		}
		if envFilter != "" && id != envFilter {
			continue
		}
		emptyEnvs = append(emptyEnvs, coverageEnvGap{EnvironmentID: id, Environment: lgStr(e, "Name", "name")})
	}
	sort.SliceStable(uninspected, func(i, j int) bool { return uninspected[i].Environment < uninspected[j].Environment })
	sort.SliceStable(emptyEnvs, func(i, j int) bool { return emptyEnvs[i].Environment < emptyEnvs[j].Environment })
	return uninspected, emptyEnvs
}

type offlineAgentRow struct {
	AgentID       string `json:"agent_id"`
	Name          string `json:"name,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	Status        string `json:"status,omitempty"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
	Reason        string `json:"reason"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

// filterOfflineAgents is filterOfflineAgentsAt now, with the default
// heartbeat threshold.
func filterOfflineAgents(agents, envs []map[string]any, envFilter string) []offlineAgentRow {
	return filterOfflineAgentsAt(agents, envs, envFilter, time.Now().UTC(), defaultAgentOfflineAfter)
}

// filterOfflineAgentsAt returns the offline agents (agentOfflineReason)
// joined to their environment.
func filterOfflineAgentsAt(agents, envs []map[string]any, envFilter string, now time.Time, after time.Duration) []offlineAgentRow {
	envByID, _ := envNameIndex(envs)
	rows := []offlineAgentRow{}
	for _, a := range agents {
		reason := agentOfflineReason(a, now, after)
		if reason == "" {
			continue
		}
		envID := lgRefID(a, "Environment", "EnvironmentID")
		envName := lgRefName(a, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		if envFilter != "" && envID != envFilter {
			continue
		}
		rows = append(rows, offlineAgentRow{
			AgentID:       lgID(a),
			Name:          lgStr(a, "Name", "name", "FriendlyName"),
			Hostname:      lgStr(a, "Hostname", "hostname", "FQDN"),
			Status:        lgStr(a, "Status", "State", "ConnectionStatus"),
			LastHeartbeat: lgStr(a, "LastHeartbeat"),
			Reason:        reason,
			EnvironmentID: envID,
			Environment:   envName,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Environment < rows[j].Environment })
	return rows
}

func validMetricOp(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "gt", ">", "ge", ">=", "gte", "lt", "<", "le", "<=", "lte", "eq", "==", "=", "ne", "!=", "<>":
		return true
	}
	return false
}
