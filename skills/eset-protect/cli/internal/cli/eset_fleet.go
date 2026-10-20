// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output). Shared plumbing for the ESET PROTECT
// novel commands: the MSP customer tree, typed views over synced devices and
// groups, name resolution, and a bounded live pager for ESET Connect lists.
//
// Customer model: device groups carry linkedEntityType
// (DEVICE_GROUP_ENTITY_TYPE_COMPANY | _MSP | _SITE | _HOUSEHOLD) and
// parentGroupUuid. A device's customer is the nearest ancestor group linked to
// a COMPANY. A tree with no COMPANY group at all (a single ESET PROTECT Hub
// tenant) rolls every device up to one "(tenant)" row; devices outside every
// COMPANY subtree of an MSP tree (the MSP's own machines included) land in
// "(unassigned)".
package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/config"
	"eset-protect-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

const (
	esetCompanyEntityType = "DEVICE_GROUP_ENTITY_TYPE_COMPANY"
	esetMSPEntityType     = "DEVICE_GROUP_ENTITY_TYPE_MSP"
	esetCustomerTenant    = "(tenant)"
	esetCustomerNone      = "(unassigned)"
	// esetCustomerUnknownGroup holds rows whose device group, or one of its
	// ancestors below the hidden scope root of a scoped API user, is not in
	// the synced tree (created after the last sync), or whose ancestry loops. Recursive group scoping cannot prove which
	// customer owns them, so they are never credited to the customer a
	// command was scoped to.
	esetCustomerUnknownGroup = "(group not in synced tree)"
	// esetCustomerParentHidden holds rows whose device group is synced but
	// sits under a parent group the mirror does not hold and that is not the
	// hidden scope root: with visible top-level groups under two or more
	// different missing parents the mirror cannot tell which one is the
	// scope root, so the walk stays owner-unknown. The usual cause is the API
	// user's permission scope, which no sync changes (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	esetCustomerParentHidden = "(parent group not visible)"
	esetCLIName              = "eset-protect-cli"
	esetMaxPageSize          = 1000
)

// esetHost builds an ESET Connect service URL; the client substitutes {region}.
func esetHost(service, path string) string {
	return "https://{region}." + service + ".eset.systems" + path
}

// esetPreviewURL is the URL a --yes run would send path to (handfixes.json:
// preview-url-is-the-sent-url): the client's own resolution (effective
// region, or the ESET_PROTECT_BASE_URL host) on a client built from the same
// config without the client hooks, so a preview never mints a token or
// sends anything. An invalid region fails here as it would on --yes.
func esetPreviewURL(flags *rootFlags, path string) (string, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return "", configErr(err)
	}
	if _, err := esetRegion(cfg); err != nil {
		return "", configErr(err)
	}
	u, err := client.New(cfg, flags.timeout, 0).DisplayRequestURL(path)
	if err != nil {
		return "", configErr(err)
	}
	return u, nil
}

type esetGroup struct {
	UUID       string `json:"uuid"`
	Name       string `json:"displayName"`
	ParentUUID string `json:"parentGroupUuid"`
	EntityType string `json:"linkedEntityType"`
}

// esetTree resolves groups to their owning customer.
type esetTree struct {
	groups       map[string]esetGroup
	hasCompanies bool
	// scopeRoot is the hidden root of a scoped API user's view: the one
	// parentGroupUuid that every visible top-level group (a group whose
	// parent the mirror does not hold) names and the mirror does not hold.
	// It is "" when a visible group has an empty parent (the real root is
	// visible) or when the top-level groups name two or more missing parents
	// (at least one of them is a missing interior node, and the mirror
	// cannot say which).
	scopeRoot string
	memo      map[string]esetOwner
}

// esetOwner is the verdict of one ancestry walk: the owning customer (or
// bucket) and whether the mirror can prove it.
type esetOwner struct {
	group esetGroup
	known bool
}

func newEsetTree(groups []esetGroup) *esetTree {
	t := &esetTree{groups: make(map[string]esetGroup, len(groups)), memo: map[string]esetOwner{}}
	for _, g := range groups {
		if g.UUID == "" {
			continue
		}
		t.groups[g.UUID] = g
		if g.EntityType == esetCompanyEntityType {
			t.hasCompanies = true
		}
	}
	t.scopeRoot = esetScopeRoot(t.groups)
	return t
}

// esetScopeRoot returns the hidden scope root of groups (see
// esetTree.scopeRoot): the single missing parent shared by every top-level
// group, or "" when a top-level group has an empty parent or the top-level
// groups name more than one missing parent.
func esetScopeRoot(groups map[string]esetGroup) string {
	root := ""
	for _, g := range groups {
		if _, held := groups[g.ParentUUID]; held {
			continue
		}
		if g.ParentUUID == "" {
			return ""
		}
		if root != "" && root != g.ParentUUID {
			return ""
		}
		root = g.ParentUUID
	}
	return root
}

// resolve is the one ancestry walk every customer view uses (handfixes.json:
// customer-views-count-unattributed-rows). A tree with no COMPANY group is
// single-tenant: every group, synced or not, is the tenant's. Otherwise the
// walk climbs parentGroupUuid from groupUUID through groups the mirror holds:
//
//   - it reaches a COMPANY group: that customer, known (whatever sits above
//     the nearest COMPANY cannot change it);
//   - it reaches, without passing a COMPANY, a group with no parent (the
//     root), a group linked to an MSP (DEVICE_GROUP_ENTITY_TYPE_MSP: ESET
//     places customers under MSP groups, never an MSP group under a
//     customer), or the hidden scope root of a scoped API user (scopeRoot:
//     every visible COMPANY sits below it, so a row whose walk reaches it
//     past no COMPANY is no visible customer's): "(unassigned)", known. The
//     MSP's own machines land here;
//   - it meets any other group the mirror does not hold, an empty group
//     UUID, or a group it already visited (a cycle): owner UNKNOWN. The row
//     may be any customer's, so views count it as unattributed and never
//     credit or drop it. The bucket says which: the starting group missing
//     (created after the last sync), an empty UUID or a cycle is "(group not
//     in synced tree)"; a missing parent of a synced group is "(parent group
//     not visible)", because a synced group names a parent this API user
//     cannot see (its permission scope) unless a device-groups sync stopped
//     part way.
func (t *esetTree) resolve(groupUUID string) (esetGroup, bool) {
	if !t.hasCompanies {
		return esetGroup{Name: esetCustomerTenant}, true
	}
	if o, ok := t.memo[groupUUID]; ok {
		return o.group, o.known
	}
	o := esetOwner{group: esetGroup{Name: esetCustomerUnknownGroup}}
	unassigned := esetOwner{group: esetGroup{Name: esetCustomerNone}, known: true}
	seen := map[string]bool{}
	for cur := groupUUID; cur != "" && !seen[cur]; {
		seen[cur] = true
		g, ok := t.groups[cur]
		if !ok {
			switch {
			case cur == t.scopeRoot:
				o = unassigned
			case cur != groupUUID:
				o = esetOwner{group: esetGroup{Name: esetCustomerParentHidden}}
			}
			break
		}
		if g.EntityType == esetCompanyEntityType {
			o = esetOwner{group: g, known: true}
			break
		}
		if g.ParentUUID == "" || g.EntityType == esetMSPEntityType {
			o = unassigned
			break
		}
		cur = g.ParentUUID
	}
	t.memo[groupUUID] = o
	return o.group, o.known
}

// parentNotVisible returns the sorted display names of the synced groups
// whose parent group the mirror does not hold, other than the hidden scope
// root, and whose rows therefore end owner-unknown (a COMPANY or MSP group
// with a hidden parent still owns its rows): the visible top-level groups
// whose parent this API user cannot see.
// Rows under them are owner-unknown ("(parent group not visible)") and stay
// so after any sync while that parent is outside the API user's
// permissions. nil for a single-tenant tree, where every row is the
// tenant's.
func (t *esetTree) parentNotVisible() []string {
	if t == nil || !t.hasCompanies {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range t.groups {
		if g.ParentUUID == "" || g.ParentUUID == t.scopeRoot {
			continue
		}
		if _, held := t.groups[g.ParentUUID]; held {
			continue
		}
		// A COMPANY (or MSP) group still owns its rows whatever sits above it.
		if _, known := t.resolve(g.UUID); known {
			continue
		}
		name := g.Name
		if strings.TrimSpace(name) == "" {
			name = g.UUID
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// esetGroupRef names a device group in JSON output.
type esetGroupRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// parentNotVisibleGroups is parentNotVisible for agents (handfixes.json:
// unattributed-cause-names-hidden-parent): the same groups, each with its
// uuid, so two groups sharing a name stay apart. Sorted by name, then uuid.
func (t *esetTree) parentNotVisibleGroups() []esetGroupRef {
	if t == nil || !t.hasCompanies {
		return nil
	}
	var out []esetGroupRef
	for _, g := range t.groups {
		if g.ParentUUID == "" || g.ParentUUID == t.scopeRoot {
			continue
		}
		if _, held := t.groups[g.ParentUUID]; held {
			continue
		}
		if _, known := t.resolve(g.UUID); known {
			continue
		}
		out = append(out, esetGroupRef{UUID: g.UUID, Name: g.Name})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].UUID < out[j].UUID
	})
	return out
}

// customerOf returns the customer (or bucket) owning groupUUID; see resolve.
// The returned group has an empty UUID for the "(tenant)", "(unassigned)",
// "(group not in synced tree)" and "(parent group not visible)" buckets.
func (t *esetTree) customerOf(groupUUID string) esetGroup {
	g, _ := t.resolve(groupUUID)
	return g
}

// path renders "MSP / Customer / Site" for a group.
func (t *esetTree) path(groupUUID string) string {
	var parts []string
	seen := map[string]bool{}
	for cur := groupUUID; cur != "" && !seen[cur]; {
		seen[cur] = true
		g, ok := t.groups[cur]
		if !ok {
			break
		}
		parts = append([]string{g.Name}, parts...)
		cur = g.ParentUUID
	}
	return strings.Join(parts, " / ")
}

func (t *esetTree) companies() []esetGroup {
	out := make([]esetGroup, 0)
	for _, g := range t.groups {
		if g.EntityType == esetCompanyEntityType {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// resolveCustomer accepts a COMPANY group UUID, an exact name
// (case-insensitive) or a unique name substring.
func (t *esetTree) resolveCustomer(q string) (esetGroup, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return esetGroup{}, fmt.Errorf("customer name or UUID is empty")
	}
	if !t.hasCompanies {
		return esetGroup{}, fmt.Errorf("no customer (COMPANY) groups in the synced device-group tree; this looks like a single-tenant account, so omit --customer")
	}
	if g, ok := t.groups[q]; ok && g.EntityType == esetCompanyEntityType {
		return g, nil
	}
	lq := strings.ToLower(q)
	var exact, partial []esetGroup
	for _, g := range t.companies() {
		name := strings.ToLower(g.Name)
		if name == lq {
			exact = append(exact, g)
		}
		if strings.Contains(name, lq) {
			partial = append(partial, g)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		partial = exact
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return esetGroup{}, fmt.Errorf("no customer matches %q (run 'fleet health' to list customers)", q)
	default:
		names := make([]string, 0, len(partial))
		for _, g := range partial {
			names = append(names, esetFlatten(g.Name))
		}
		return esetGroup{}, fmt.Errorf("%q matches %d customers (%s); use the full name or the group UUID", q, len(partial), strings.Join(names, ", "))
	}
}

// flexInt decodes protobuf-JSON integers, which arrive as numbers or (for
// 64-bit fields) decimal strings. Anything that is not an exact integer
// (garbage, a fraction, NaN/Inf, out of range) is a decode error, so the
// caller counts the row as rejected instead of reading it as zero.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		if s == "" {
			*f = 0
			return nil
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*f = flexInt(n)
		return nil
	}
	// JSON numbers such as 3.0 or 1e3 are integral; 2.5 is not. Parse the
	// decimal exactly (big.Rat) so float rounding cannot turn 1.0000000000000001
	// into 1 or accept 9223372036854775808.
	r, ok := new(big.Rat).SetString(s)
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return fmt.Errorf("not an integer: %s", string(b))
	}
	*f = flexInt(r.Num().Int64())
	return nil
}

// esetIsObject reports whether a raw payload row is a JSON object. A null or
// scalar row decodes "successfully" into a zero struct, so callers reject it.
func esetIsObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// esetRejects counts payload rows a command could not decode, so a malformed
// row is reported instead of silently shrinking a total. Commands add
// `rejected_rows` to their JSON and call warn() once.
type esetRejects struct {
	N     int
	First string
}

func (r *esetRejects) add(what string, err error) {
	r.N++
	if r.First == "" {
		r.First = fmt.Sprintf("%s: %v", what, err)
	}
}

// warn prints one stderr line when rows were rejected.
func (r *esetRejects) warn(cmd *cobra.Command) {
	if r != nil && r.N > 0 {
		esetFprintf(cmd.ErrOrStderr(), "warning: %d row(s) could not be decoded and were left out of the totals (first: %s)\n", r.N, r.First)
	}
}

type esetVersion struct {
	Name  string  `json:"name"`
	Major flexInt `json:"major"`
	Minor flexInt `json:"minor"`
	Patch flexInt `json:"patch"`
}

func (v esetVersion) String() string {
	if v.Name != "" {
		return v.Name
	}
	if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// less orders versions by major.minor.patch.
func (v esetVersion) less(o esetVersion) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

type esetComponent struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"displayName"`
	Version     esetVersion `json:"version"`
}

type esetDevice struct {
	UUID                string            `json:"uuid"`
	DisplayName         string            `json:"displayName"`
	Hostname            string            `json:"systemHostname"`
	ParentGroupUUID     string            `json:"parentGroupUuid"`
	FunctionalityStatus string            `json:"functionalityStatus"`
	ProblemCount        flexInt           `json:"functionalityProblemCount"`
	IsMuted             bool              `json:"isMuted"`
	LastSyncTime        string            `json:"lastSyncTime"`
	EnrollmentStatus    string            `json:"enrollmentStatus"`
	Components          []esetComponent   `json:"deployedComponents"`
	ActiveProducts      []json.RawMessage `json:"activeProducts"`
	PrimaryIP           string            `json:"primaryLocalIpAddress"`
	OperatingSystem     struct {
		DisplayName string `json:"displayName"`
	} `json:"operatingSystem"`
}

func (d esetDevice) name() string {
	if d.DisplayName != "" {
		return d.DisplayName
	}
	if d.Hostname != "" {
		return d.Hostname
	}
	return d.UUID
}

// lastSeen parses lastSyncTime; ok is false when it is absent or malformed.
func (d esetDevice) lastSeen() (time.Time, bool) {
	if d.LastSyncTime == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, d.LastSyncTime)
	return t, err == nil
}

// shortEnum trims ESET's enum prefixes: DEVICE_FUNCTIONALITY_STATUS_OK -> OK.
func shortEnum(v, prefix string) string {
	return strings.TrimPrefix(v, prefix)
}

// openEsetStore opens the local mirror read-only. A missing mirror is an
// empty-cache state: it prints the sync hint and returns (nil, nil). The
// hint carries the "hint:" prefix because the MCP shell-out forwards only
// "hint:"/"warning:" stderr lines to the calling agent.
func openEsetStore(cmd *cobra.Command, dbPath string, resources ...string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath(esetCLIName)
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		esetFprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; run '%s sync --resources %s' first\n", dbPath, esetCLIName, strings.Join(resources, ","))
		return nil, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database %s: %w", dbPath, err)
	}
	return db, nil
}

// esetMirrorState is the completeness verdict for the resources one mirror
// view reads. It is the single check every local-mirror reader uses (the JSON
// synced_hint, a report section's incomplete flag and the stderr hint all
// come from it), so a reader cannot fall back to a timestamp-only test that
// reads an interrupted sync as a finished one.
type esetMirrorState struct {
	// Incomplete lists the needed resources with no completed sync.
	Incomplete []string
	// Hint is "" when the mirror can answer, otherwise what to sync.
	Hint string
}

// incomplete reports whether any of resources lacks a completed sync.
func (s esetMirrorState) incomplete(resources ...string) bool {
	for _, r := range resources {
		for _, m := range s.Incomplete {
			if m == r {
				return true
			}
		}
	}
	return false
}

// hintFor is the synced hint for the subset of resources one report section
// is built from ("" when every one of them completed a sync).
func (s esetMirrorState) hintFor(resources ...string) string {
	var missing []string
	for _, r := range resources {
		if s.incomplete(r) {
			missing = append(missing, r)
		}
	}
	return esetMirrorIncompleteHint(missing)
}

// esetMirrorCheck reads sync_state for every resource a view needs. It
// reports a resource as incomplete when the mirror file is missing (db ==
// nil), the resource was never synced, its last sync did not finish, its sync
// state cannot be read, or a --full/--latest-only reset (complete=1 with a
// zero timestamp) never finished a fetch. Only a completed sync makes an empty
// result a real zero.
func esetMirrorCheck(db *store.Store, resources ...string) esetMirrorState {
	if db == nil {
		return esetMirrorState{
			Incomplete: append([]string(nil), resources...),
			Hint:       "no local mirror; run '" + esetCLIName + " sync --resources " + strings.Join(resources, ",") + "'",
		}
	}
	var missing []string
	for _, r := range resources {
		var complete int
		var at sql.NullTime
		err := db.DB().QueryRow(`SELECT last_attempt_complete, last_synced_at FROM sync_state WHERE resource_type = ?`, r).Scan(&complete, &at)
		// No row, no table, unreadable state, an interrupted sync, or a
		// --full/--latest-only reset (complete=1 with a zero timestamp) that
		// never finished a fetch.
		if err != nil || complete != 1 || !at.Valid || at.Time.Year() <= 1 {
			missing = append(missing, r)
		}
	}
	return esetMirrorState{Incomplete: missing, Hint: esetMirrorIncompleteHint(missing)}
}

func esetMirrorIncompleteHint(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return "no completed sync for " + strings.Join(missing, ", ") + "; results may be incomplete, run '" + esetCLIName + " sync --resources " + strings.Join(missing, ",") + "'"
}

// esetMirrorHint is the synced_hint for a view built from the local mirror:
// "" when the mirror can answer, otherwise what to sync (see esetMirrorCheck).
func esetMirrorHint(db *store.Store, resources ...string) string {
	return esetMirrorCheck(db, resources...).Hint
}

// esetJoinHints joins non-empty hints with "; " so a more specific note (an
// empty mirror, a missing tree) is added to the completeness verdict, never
// substituted for it.
func esetJoinHints(hints ...string) string {
	kept := make([]string, 0, len(hints))
	for _, h := range hints {
		if h = strings.TrimSpace(h); h != "" {
			kept = append(kept, h)
		}
	}
	return strings.Join(kept, "; ")
}

// esetSyncHints prints at most one hint to stderr for the resources a command
// reads: the completeness verdict first (the same check as synced_hint), then
// staleness against --max-age. The "hint:" prefix is what MCP forwards.
func esetSyncHints(cmd *cobra.Command, db *store.Store, maxAge time.Duration, resources ...string) {
	if cmd == nil || db == nil {
		return // openEsetStore already said the mirror is missing
	}
	if hint := esetMirrorHint(db, resources...); hint != "" {
		esetFprintf(cmd.ErrOrStderr(), "hint: %s\n", hint)
		return
	}
	for _, r := range resources {
		if hintIfStale(cmd, db, r, maxAge) {
			return
		}
	}
}

// esetIncompleteSyncHint is the completeness hint the generated local-read
// paths (sync_hint.go emitSyncHints: list commands with --data-source local or
// the auto fallback, analytics, search, store diagnostics) print when a
// resource HAS a sync timestamp but its last attempt did not complete, which
// the timestamp-only check reads as synced. "" means nothing to add. An empty
// resourceType (every resource) checks every row of sync_state.
func esetIncompleteSyncHint(db *store.Store, resourceType string) string {
	if db == nil {
		return ""
	}
	if strings.TrimSpace(resourceType) != "" {
		return esetMirrorHint(db, resourceType)
	}
	rows, err := db.DB().Query(`SELECT resource_type FROM sync_state WHERE last_attempt_complete != 1 ORDER BY resource_type`)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var r string
		if rows.Scan(&r) == nil && r != "" {
			missing = append(missing, r)
		}
	}
	return esetMirrorIncompleteHint(missing)
}

// esetDecodeRows decodes every stored row of one resource type. A row that
// does not decode, or has no uuid, fails the load rather than silently
// shrinking totals (a lost COMPANY group would misfile its whole subtree).
func esetDecodeRows[T any](db *store.Store, resourceType string, uuidOf func(T) string) ([]T, error) {
	out := make([]T, 0)
	var bad []string
	err := db.ListScan(resourceType, func(id string, data json.RawMessage) bool {
		var v T
		if derr := json.Unmarshal(data, &v); derr != nil {
			bad = append(bad, fmt.Sprintf("%s (%v)", id, derr))
			return true
		}
		if uuidOf(v) == "" {
			bad = append(bad, id+" (no uuid)")
			return true
		}
		out = append(out, v)
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%d stored %s rows could not be read (first: %s); re-sync with '%s sync --resources %s --full'", len(bad), resourceType, bad[0], esetCLIName, resourceType)
	}
	return out, nil
}

func loadEsetGroups(db *store.Store) ([]esetGroup, error) {
	return esetDecodeRows(db, "device-groups", func(g esetGroup) string { return g.UUID })
}

func loadEsetDevices(db *store.Store) ([]esetDevice, error) {
	return esetDecodeRows(db, "devices", func(d esetDevice) string { return d.UUID })
}

// loadEsetRaw returns every stored row of one resource type.
func loadEsetRaw(db *store.Store, resourceType string) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0)
	err := db.ListScan(resourceType, func(_ string, data json.RawMessage) bool {
		out = append(out, append(json.RawMessage(nil), data...))
		return true
	})
	return out, err
}

// resolveEsetDevice matches a UUID, or an exact display name or hostname
// (case-insensitive). Ambiguous names are refused rather than guessed.
func resolveEsetDevice(devices []esetDevice, q string) (esetDevice, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return esetDevice{}, fmt.Errorf("device name or UUID is empty")
	}
	var matches []esetDevice
	for _, d := range devices {
		if d.UUID == q {
			return d, nil
		}
		if strings.EqualFold(d.DisplayName, q) || strings.EqualFold(d.Hostname, q) {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return esetDevice{}, fmt.Errorf("no synced device is named %q (exact display name, hostname or UUID; run 'sync --resources devices' if it was added recently)", q)
	default:
		ids := make([]string, 0, len(matches))
		for _, d := range matches {
			ids = append(ids, esetFlatten(d.UUID))
		}
		return esetDevice{}, fmt.Errorf("%d devices are named %q (%s); pass the UUID", len(matches), q, strings.Join(ids, ", "))
	}
}

// esetPageResult is one bounded walk of an ESET Connect list endpoint.
type esetPageResult struct {
	Items     []json.RawMessage
	Pages     int
	Truncated bool // stopped at maxPages with more pages remaining
}

// esetFetchAll walks pageToken pagination (pageSize 1000) up to maxPages.
func esetFetchAll(ctx context.Context, c *client.Client, url string, params map[string]string, itemsKey string, maxPages int) (esetPageResult, error) {
	res := esetPageResult{Items: make([]json.RawMessage, 0)}
	if maxPages <= 0 {
		maxPages = 1
	}
	q := make(map[string]string, len(params)+2)
	for k, v := range params {
		if v != "" {
			q[k] = v
		}
	}
	q["pageSize"] = strconv.Itoa(esetMaxPageSize)
	token := ""
	for res.Pages < maxPages {
		if token != "" {
			q["pageToken"] = token
		} else {
			delete(q, "pageToken")
		}
		data, err := c.GetNoCache(ctx, url, q)
		if err != nil {
			return res, err
		}
		res.Pages++
		if !esetIsObject(data) {
			return res, fmt.Errorf("decoding %s page %d: response is not a JSON object", itemsKey, res.Pages)
		}
		var page map[string]json.RawMessage
		if err := json.Unmarshal(data, &page); err != nil {
			return res, fmt.Errorf("decoding %s page %d: %w", itemsKey, res.Pages, err)
		}
		// Hand-fix sync-unexpected-list-shape (live pager twin): an error
		// envelope or foreign wrapper is not an empty final page; reading it
		// as one truncated exposure results and customer membership.
		if esetUnexpectedListShape(data, itemsKey) {
			return res, fmt.Errorf("decoding %s page %d: the response is not a %s list page", itemsKey, res.Pages, itemsKey)
		}
		var items []json.RawMessage
		if raw, ok := page[itemsKey]; ok && len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &items); err != nil {
				return res, fmt.Errorf("decoding %s page %d items: %w", itemsKey, res.Pages, err)
			}
		}
		res.Items = append(res.Items, items...)
		token = ""
		if raw, ok := page["nextPageToken"]; ok && string(bytes.TrimSpace(raw)) != "null" {
			// A malformed continuation token must not read as "last page".
			if err := json.Unmarshal(raw, &token); err != nil {
				return res, fmt.Errorf("decoding %s page %d: nextPageToken is not a string: %w", itemsKey, res.Pages, err)
			}
		}
		if token == "" {
			return res, nil
		}
	}
	res.Truncated = true
	return res, nil
}

// esetSince parses --since style durations (7d, 24h, 2w) into a start time.
func esetSince(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	d, err := cliutil.ParseDurationLoose(s)
	if err != nil {
		return time.Time{}, usageErr(fmt.Errorf("invalid --since %q: use a duration such as 24h, 7d or 2w", s))
	}
	return time.Now().Add(-d), nil
}
