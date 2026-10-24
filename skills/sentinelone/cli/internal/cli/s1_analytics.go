// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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

// agentOS is the agent's OS type as SentinelOne reports it (osType: windows,
// macos, linux, windows_legacy), lowercased; "unknown" when absent.
func agentOS(a map[string]any) string {
	if os := strings.ToLower(strings.TrimSpace(gstr(a, "osType"))); os != "" {
		return os
	}
	return "unknown"
}

// modalVersionByOS returns, per OS type, the most common agentVersion among
// live (non-decommissioned) agents: the de facto "current" build for that OS.
// Agent packages are per OS, so a version is only ever compared with versions
// of the same OS, and a retired agent never votes for what is current.
func modalVersionByOS(agents []map[string]any) map[string]string {
	counts := map[string]map[string]int{}
	for _, a := range agents {
		if agentDecommissioned(a) {
			continue
		}
		v := strings.TrimSpace(gstr(a, "agentVersion"))
		if v == "" {
			continue
		}
		os := agentOS(a)
		if counts[os] == nil {
			counts[os] = map[string]int{}
		}
		counts[os][v]++
	}
	out := map[string]string{}
	for os, byVersion := range counts {
		best, bestN := "", -1
		for v, n := range byVersion {
			// Tie-break toward the higher version so "current" trends forward.
			if n > bestN || (n == bestN && compareS1Version(v, best) > 0) {
				best, bestN = v, n
			}
		}
		out[os] = best
	}
	return out
}

// agentBehindCurrent reports whether a live agent's version is below the
// current build of its own OS. An agent with no version, or an OS with no
// current build, is not judged.
func agentBehindCurrent(a map[string]any, current map[string]string) bool {
	v := strings.TrimSpace(gstr(a, "agentVersion"))
	cur := current[agentOS(a)]
	return v != "" && cur != "" && compareS1Version(v, cur) < 0
}

// ---- agent state predicates (shared across fleet-health / reclaim / posture / versions plan / whatchanged) ----

// liveness is whether SentinelOne says an agent is checking in to the
// management console.
type liveness int

const (
	livenessUnknown liveness = iota
	livenessOnline
	livenessOffline
)

func (l liveness) String() string {
	switch l {
	case livenessOnline:
		return "online"
	case livenessOffline:
		return "offline"
	}
	return "unknown"
}

// agentLiveness reads isActive and nothing else: GET /agents documents it as
// "Include only active Agents", and the threat filter agentIsActive as "Agents
// currently connected to the Management Console"; lastActiveDate is the last
// check-in. networkStatus is NOT liveness. It is the state of the "Disconnect
// from network" isolation action (spec.yaml POST /agents/actions/disconnect
// isolates an endpoint, and the agent keeps talking to the console), so a
// powered-off laptop that was never isolated still reads "connected" and an
// isolated live server reads "disconnected".
//
// Only a JSON boolean is read. A missing, null, string or numeric isActive is
// livenessUnknown, and callers fail closed on it: unknown is never counted
// online, and never treated as offline enough to act on.
func agentLiveness(a map[string]any) liveness {
	v, ok := gval(a, "isActive")
	if !ok {
		return livenessUnknown
	}
	b, isBool := v.(bool)
	switch {
	case !isBool:
		return livenessUnknown
	case b:
		return livenessOnline
	default:
		return livenessOffline
	}
}

// agentOnline reports whether the agent is explicitly active. Unknown
// liveness is not online.
func agentOnline(a map[string]any) bool {
	return agentLiveness(a) == livenessOnline
}

// agentNetworkStatus is the agent's network-isolation state as SentinelOne
// reports it ("connected", "disconnected", ...), lowercased. It says whether
// the endpoint is isolated from the network, not whether it is online.
func agentNetworkStatus(a map[string]any) string {
	return strings.ToLower(strings.TrimSpace(gstr(a, "networkStatus")))
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

// ---- site identity ----

// siteRef identifies a site. SentinelOne creates a "Default site" in every
// account (spec.yaml /sites documents isDefault and accountIds), so a site
// NAME is not unique across the tenants an MSP console holds: every grouping
// keys on Key (the siteId when present) and shows Label.
type siteRef struct {
	Key     string
	ID      string
	Name    string
	Account string
}

func newSiteRef(id, name, accountID, account string) siteRef {
	r := siteRef{ID: id, Name: name, Account: account}
	switch {
	case id != "":
		r.Key = "id:" + id
	case name != "":
		acct := account
		if acct == "" {
			acct = accountID
		}
		r.Key = "name:" + acct + "/" + name
	default:
		r.Key = "(unknown)"
	}
	return r
}

// Label is the site as shown to a person: "Account / Site" when the account
// is known, so two accounts' same-named sites read apart.
func (r siteRef) Label() string {
	name := r.Name
	if name == "" {
		name = r.ID
	}
	if name == "" {
		return "(unknown)"
	}
	if r.Account != "" {
		return r.Account + " / " + name
	}
	return name
}

// Matches reports whether q names this site: its name, its id, or its label,
// case-insensitively.
func (r siteRef) Matches(q string) bool {
	q = strings.TrimSpace(q)
	return q != "" && (strings.EqualFold(q, r.Name) || q == r.ID || strings.EqualFold(q, r.Label()))
}

func agentSiteRef(a map[string]any) siteRef {
	return newSiteRef(gstr(a, "siteId"), gstr(a, "siteName"), gstr(a, "accountId"), gstr(a, "accountName"))
}

func threatSiteRef(t map[string]any) siteRef {
	return newSiteRef(
		gstrFirst(t, "agentRealtimeInfo.siteId", "agentDetectionInfo.siteId", "siteId"),
		gstrFirst(t, "agentRealtimeInfo.siteName", "agentDetectionInfo.siteName", "siteName"),
		gstrFirst(t, "agentRealtimeInfo.accountId", "agentDetectionInfo.accountId", "accountId"),
		gstrFirst(t, "agentRealtimeInfo.accountName", "agentDetectionInfo.accountName", "accountName"),
	)
}

// agentSite is the agent's site label (see siteRef.Label).
func agentSite(a map[string]any) string {
	return agentSiteRef(a).Label()
}

// ---- endpoint identity ----

// agentKey identifies one agent: its id, else its uuid.
func agentKey(a map[string]any) string {
	if id := gstr(a, "id"); id != "" {
		return "id:" + id
	}
	if u := gstr(a, "uuid"); u != "" {
		return "uuid:" + u
	}
	return ""
}

func threatAgentID(t map[string]any) string {
	return gstrFirst(t, "agentRealtimeInfo.agentId", "agentDetectionInfo.agentId", "agentId")
}

func threatAgentUUID(t map[string]any) string {
	return gstrFirst(t, "agentRealtimeInfo.agentUuid", "agentDetectionInfo.agentUuid", "agentUuid")
}

// threatEndpointKey identifies the agent a detection belongs to. A hostname is
// not an identity (two customers both have a "DC01"), so the agent id or uuid
// wins, and a bare name is keyed together with its site.
func threatEndpointKey(t map[string]any) string {
	if id := threatAgentID(t); id != "" {
		return "id:" + id
	}
	if u := threatAgentUUID(t); u != "" {
		return "uuid:" + u
	}
	if name := strings.ToLower(strings.TrimSpace(threatEndpoint(t))); name != "" {
		return "name:" + threatSiteRef(t).Key + "/" + name
	}
	return ""
}

// threatOnAgent reports whether detection t belongs to agent a. When the
// detection names its agent (id or uuid) only that counts; a name-only
// detection matches when the name is unique among synced agents and the sites
// do not disagree.
func threatOnAgent(t, a map[string]any, nameUnique bool) bool {
	tid, tuuid := threatAgentID(t), threatAgentUUID(t)
	if tid != "" || tuuid != "" {
		return (tid != "" && tid == gstr(a, "id")) || (tuuid != "" && tuuid == gstr(a, "uuid"))
	}
	name := gstr(a, "computerName")
	if !nameUnique || name == "" || !strings.EqualFold(threatEndpoint(t), name) {
		return false
	}
	ts, as := threatSiteRef(t), agentSiteRef(a)
	return ts.ID == "" || as.ID == "" || ts.ID == as.ID
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
func threatEndpoint(t map[string]any) string {
	return gstrFirst(t, "agentRealtimeInfo.agentComputerName", "agentComputerName", "agentDetectionInfo.agentComputerName")
}
func threatSite(t map[string]any) string {
	return threatSiteRef(t).Label()
}

// mitigationCompletingActions is the allowlist of SentinelOne mitigation
// actions (the /threats/mitigate/{action} enum) whose success means the threat
// itself was mitigated: each neutralizes the threat's file and runs kill first.
// Everything else is refused, so it can never set a completion time:
// "un-quarantine" undoes a quarantine; "kill" (stops the processes) and
// "network-quarantine" (isolates the endpoint) contain the threat but leave it
// on disk; and an action name not listed here is unknown, not mitigating.
var mitigationCompletingActions = map[string]bool{
	"quarantine":           true,
	"remediate":            true,
	"rollback-remediation": true,
}

// normalizeMitigationAction folds case and "_"/" " separators so
// "Rollback_Remediation" matches "rollback-remediation" (and "UN_QUARANTINE"
// still matches the refused "un-quarantine").
func normalizeMitigationAction(s string) string {
	return strings.NewReplacer("_", "-", " ", "-").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// mitigationFieldSet reports whether a record field carries anything: a
// non-blank string, a non-empty list or object, true, or a non-zero number.
func mitigationFieldSet(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case bool:
		return x
	case float64:
		return x != 0
	}
	return true
}

// mitigationCounter reads an actionsCounters.<name> count; ok is false when the
// counter is present but not a number (an unreadable counter is not zero).
func mitigationCounter(m map[string]any, name string) (n float64, present, ok bool) {
	s := gstr(m, "actionsCounters."+name)
	if s == "" {
		return 0, false, true
	}
	n, err := strconv.ParseFloat(s, 64)
	return n, true, err == nil
}

// mitigationActionCompleted reports whether one mitigationStatus[] record is a
// completed mitigation and, if so, when it ended. It fails closed: the action
// must be in mitigationCompletingActions and its status must say "success";
// any failure signal disqualifies it - another or absent status (failed,
// pending, partial), a populated error field, failed or pending-reboot action
// counters, fewer successes than targets, or a missing or unparseable
// mitigationEndedAt.
func mitigationActionCompleted(m map[string]any) (time.Time, bool) {
	if !mitigationCompletingActions[normalizeMitigationAction(gstr(m, "action"))] {
		return time.Time{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(gstr(m, "status")), "success") {
		return time.Time{}, false
	}
	for _, k := range []string{"error", "errors", "errorMessage", "failure", "failureReason"} {
		if mitigationFieldSet(m[k]) {
			return time.Time{}, false
		}
	}
	for _, c := range []string{"failed", "pendingReboot"} {
		if n, _, ok := mitigationCounter(m, c); !ok || n > 0 {
			return time.Time{}, false
		}
	}
	succ, hasSucc, okS := mitigationCounter(m, "success")
	total, hasTotal, okT := mitigationCounter(m, "total")
	if !okS || !okT || (hasSucc && hasTotal && succ < total) {
		return time.Time{}, false
	}
	return parseS1Time(gstr(m, "mitigationEndedAt"))
}

// threatMitigatedAt is when mitigation of a detection completed: the EARLIEST
// mitigationEndedAt among the threat's completed mitigation actions
// (SentinelOne's top-level mitigationStatus list; see
// mitigationActionCompleted). The threat was mitigated then, so nothing later
// moves it - not an un-quarantine, not a re-run remediate. When the threat
// carries action records they are authoritative: if none qualifies there is
// no completion time, and mitigatedAt is not substituted. Only a threat with
// no action records falls back to an explicit mitigatedAt. A threat's
// updatedAt is NEVER a mitigation time (any later edit, a verdict or a note,
// moves it), so without one of these the duration is unmeasured.
func threatMitigatedAt(t map[string]any) (time.Time, bool) {
	if list, ok := t["mitigationStatus"].([]any); ok && len(list) > 0 {
		var first time.Time
		found := false
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if ts, ok := mitigationActionCompleted(m); ok && (!found || ts.Before(first)) {
				first, found = ts, true
			}
		}
		return first, found
	}
	return parseS1Time(gstrFirst(t, "threatInfo.mitigatedAt", "mitigatedAt"))
}

// threatTimeToMitigate is the time from detection (createdAt) to mitigation
// completion (threatMitigatedAt). ok is false - the duration is unmeasured -
// when either time is missing or the completion precedes detection; a
// pre-detection completion is never swapped for a later action.
func threatTimeToMitigate(t map[string]any) (time.Duration, bool) {
	created, ok := parseS1Time(threatCreatedAt(t))
	if !ok {
		return 0, false
	}
	done, ok := threatMitigatedAt(t)
	if !ok || done.Before(created) {
		return 0, false
	}
	return done.Sub(created), true
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
