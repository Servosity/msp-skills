// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the SentinelOne transcendence (novel) commands. These
// commands query the locally-synced `resources` table (agents, threats, sites,
// groups, ranger) and the `fleet_snapshots` history table, parse the raw
// SentinelOne JSON, and compute cross-entity / time-diff analytics the live
// API cannot return in a single call. Everything reads from the local store, so
// every command is fully testable against seeded data with no live tenant.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"sentinelone-pp-cli/internal/store"
)

// ---- store access ----

// openS1Store opens the local SQLite store, resolving the default path when
// --db was not provided. The error names the sync command so a first-run user
// knows the next step.
func openS1Store(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("sentinelone-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'sentinelone-cli sync' first", err)
	}
	return db, nil
}

// loadResourceObjects returns every synced object of a resource type, decoded
// into maps. It reads the full set (List() caps at 200, which is too low for a
// fleet), so analytics never silently truncate.
func loadResourceObjects(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err == nil && m != nil {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

func loadAgents(ctx context.Context, db *store.Store) ([]map[string]any, error) {
	return loadResourceObjects(ctx, db, "agents")
}

func loadThreats(ctx context.Context, db *store.Store) ([]map[string]any, error) {
	return loadResourceObjects(ctx, db, "threats")
}

// decodeObjects decodes a slice of raw JSON messages into maps, skipping any
// that fail to parse.
func decodeObjects(raws []json.RawMessage) []map[string]any {
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		var m map[string]any
		if err := json.Unmarshal(r, &m); err == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}

// ---- JSON field access (dotted paths over nested maps) ----

// gval walks a dotted path (e.g. "threatInfo.sha1") through nested maps.
func gval(obj map[string]any, path string) (any, bool) {
	cur := any(obj)
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[seg]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func gstr(obj map[string]any, path string) string {
	v, ok := gval(obj, path)
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// gstrFirst returns the first non-empty value among the given paths.
func gstrFirst(obj map[string]any, paths ...string) string {
	for _, p := range paths {
		if s := gstr(obj, p); s != "" {
			return s
		}
	}
	return ""
}

func gbool(obj map[string]any, path string) bool {
	v, ok := gval(obj, path)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	}
	return false
}

// ---- time + version utilities ----

// parseS1Time parses the timestamp shapes SentinelOne returns (RFC3339 with or
// without sub-second precision, always UTC "Z").
func parseS1Time(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999Z07:00",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// daysSince returns whole days between an RFC3339 timestamp and now. Returns
// (-1, false) when the timestamp is missing or unparseable.
func daysSince(now time.Time, ts string) (int, bool) {
	t, ok := parseS1Time(ts)
	if !ok {
		return -1, false
	}
	return int(now.Sub(t).Hours() / 24), true
}

// compareS1Version compares dotted numeric agent versions (e.g. "23.4.2.14").
// Returns -1 if a < b, 0 if equal, 1 if a > b. Non-numeric segments fall back
// to string comparison so it never panics on odd builds.
func compareS1Version(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aerr := strconv.Atoi(av)
		bi, berr := strconv.Atoi(bv)
		if aerr == nil && berr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
			continue
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// modalVersion returns the most common agentVersion across the fleet (the de
// facto "current" version), used to flag out-of-date agents.
func modalVersion(agents []map[string]any) string {
	counts := map[string]int{}
	for _, a := range agents {
		if v := gstr(a, "agentVersion"); v != "" {
			counts[v]++
		}
	}
	best, bestN := "", -1
	for v, n := range counts {
		// Tie-break toward the higher version so "current" trends forward.
		if n > bestN || (n == bestN && compareS1Version(v, best) > 0) {
			best, bestN = v, n
		}
	}
	return best
}

// ---- agent state predicates (shared across fleet-health / coverage / posture) ----

func agentOnline(a map[string]any) bool {
	return strings.EqualFold(gstr(a, "networkStatus"), "connected")
}

func agentDecommissioned(a map[string]any) bool {
	return gbool(a, "isDecommissioned")
}

// agentInProtect reports whether the agent's mitigation mode is "protect"
// (full prevention) rather than "detect" (alert-only). Missing mode is treated
// as not-protect so coverage gaps surface the unknown rather than hide it.
func agentInProtect(a map[string]any) bool {
	return strings.EqualFold(gstr(a, "mitigationMode"), "protect")
}

func agentSite(a map[string]any) string {
	return gstrFirst(a, "siteName", "siteId")
}

// ---- threat field accessors (tolerant of nested threatInfo / flat shapes) ----

func threatID(t map[string]any) string {
	return gstrFirst(t, "id", "threatId")
}
func threatName(t map[string]any) string {
	return gstrFirst(t, "threatInfo.threatName", "threatName")
}
func threatSHA1(t map[string]any) string {
	return gstrFirst(t, "threatInfo.sha1", "fileContentHash", "fileSha1", "sha1")
}
func threatVerdict(t map[string]any) string {
	return gstrFirst(t, "threatInfo.analystVerdict", "analystVerdict")
}
func threatConfidence(t map[string]any) string {
	return gstrFirst(t, "threatInfo.confidenceLevel", "confidenceLevel")
}
func threatIncident(t map[string]any) string {
	return gstrFirst(t, "threatInfo.incidentStatus", "incidentStatus")
}
func threatMitigation(t map[string]any) string {
	return gstrFirst(t, "threatInfo.mitigationStatus", "mitigationStatus")
}
func threatCreatedAt(t map[string]any) string {
	return gstrFirst(t, "threatInfo.createdAt", "createdAt", "threatInfo.identifiedAt", "identifiedAt")
}
func threatUpdatedAt(t map[string]any) string {
	return gstrFirst(t, "threatInfo.updatedAt", "updatedAt", "updatedDate")
}
func threatEndpoint(t map[string]any) string {
	return gstrFirst(t, "agentRealtimeInfo.agentComputerName", "agentComputerName", "agentDetectionInfo.agentComputerName")
}
func threatSite(t map[string]any) string {
	return gstrFirst(t, "agentRealtimeInfo.siteName", "siteName")
}

// threatActive reports whether a threat is still open (not resolved and not
// mitigated). Used by triage/posture/blast-radius to separate live from
// handled threats.
func threatActive(t map[string]any) bool {
	if strings.EqualFold(threatIncident(t), "resolved") {
		return false
	}
	switch strings.ToLower(threatMitigation(t)) {
	case "mitigated", "marked_as_benign", "resolved":
		return false
	}
	return true
}

// ---- output helpers ----

// honestEmptyJSON writes a structured "no data / why" payload (exit 0) for the
// absence-of-correctness contract: a command whose correct answer can be empty
// must say so explicitly rather than fabricate or error.
func honestEmptyJSON(cmd *cobra.Command, flags *rootFlags, reason string, extra map[string]any) error {
	payload := map[string]any{
		"count":  0,
		"items":  []any{},
		"reason": reason,
	}
	for k, v := range extra {
		payload[k] = v
	}
	if !flags.asJSON {
		fmt.Fprintln(cmd.OutOrStdout(), reason)
		return nil
	}
	return flags.printJSON(cmd, payload)
}

// sortByScoreDesc sorts items by a score (desc), stable for ties.
func sortByScoreDesc[T any](items []T, score func(T) float64) {
	sort.SliceStable(items, func(i, j int) bool {
		return score(items[i]) > score(items[j])
	})
}

// clip truncates s to n runes, appending an ellipsis when it had to cut.
func clip(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// orUnknown returns s, or "(unknown)" when s is empty, for stable grouping.
func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
