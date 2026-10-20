// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written shared helpers for the DNSFilter novel commands (why-blocked,
// allow, agents drift/stale, lists where, policy-audit, unblocks triage).

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"dnsfilter-pp-cli/internal/client"
	"dnsfilter-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// jaRes is one JSON:API resource ({id, type, attributes, relationships}). Flat
// objects (release feed, scheduled policy show) land entirely in Attrs.
type jaRes struct {
	ID    string
	Type  string
	Attrs map[string]any
	Rels  map[string]any
}

func toJARes(m map[string]any) jaRes {
	r := jaRes{ID: idString(m["id"]), Attrs: map[string]any{}, Rels: map[string]any{}}
	r.Type, _ = m["type"].(string)
	if a, ok := m["attributes"].(map[string]any); ok {
		r.Attrs = a
	} else {
		for k, v := range m {
			if k != "id" && k != "type" && k != "relationships" {
				r.Attrs[k] = v
			}
		}
	}
	if rel, ok := m["relationships"].(map[string]any); ok {
		r.Rels = rel
	}
	if r.ID == "" {
		r.ID = idString(r.Attrs["id"])
	}
	return r
}

// decodeJA accepts {data:[...]}, {data:{...}}, a bare array, or a bare object.
func decodeJA(raw json.RawMessage) ([]jaRes, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			v = d
		}
	}
	out := make([]jaRes, 0)
	switch t := v.(type) {
	case []any:
		for i, it := range t {
			m, ok := it.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("decoding response: item %d is not an object", i)
			}
			out = append(out, toJARes(m))
		}
	case map[string]any:
		out = append(out, toJARes(t))
	case nil:
	default:
		return nil, fmt.Errorf("decoding response: unexpected %T where an object or list was expected", v)
	}
	return out, nil
}

func idString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	}
	return ""
}

func (r jaRes) str(k string) string {
	switch t := r.Attrs[k].(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return idString(t)
	}
}

func (r jaRes) boolean(k string) bool { b, _ := r.Attrs[k].(bool); return b }

func (r jaRes) strs(k string) []string {
	arr, _ := r.Attrs[k].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if s := idString(it); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// relID returns relationships.<name>.data.id ("" when absent or null).
func (r jaRes) relID(name string) string {
	rel, _ := r.Rels[name].(map[string]any)
	data, _ := rel["data"].(map[string]any)
	return idString(data["id"])
}

// relIDs returns relationships.<name>.data[].id.
func (r jaRes) relIDs(name string) []string {
	rel, _ := r.Rels[name].(map[string]any)
	arr, _ := rel["data"].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			if id := idString(m["id"]); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

// ponytail: fixed page size; V1 list responses carry no total, so a short page ends the walk.
const dnsfilterPageSize = 100

// fetchAllPages walks a V1 `page[number]` list until a short page or maxPages.
// truncated reports that maxPages stopped the walk with more data likely.
func fetchAllPages(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) (items []jaRes, truncated bool, err error) {
	items = make([]jaRes, 0)
	for page := 1; maxPages <= 0 || page <= maxPages; page++ {
		p := map[string]string{"page[number]": strconv.Itoa(page), "page[size]": strconv.Itoa(dnsfilterPageSize)}
		for k, v := range params {
			p[k] = v
		}
		raw, err := c.GetNoCache(ctx, path, p)
		if err != nil {
			return items, false, err
		}
		batch, err := decodeJA(raw)
		if err != nil {
			return items, false, err
		}
		items = append(items, batch...)
		if len(batch) < dnsfilterPageSize {
			return items, false, nil
		}
	}
	return items, true, nil
}

// resolveOrg accepts a numeric organization id or a name (exact, then unique
// case-insensitive substring) and returns the matching organization.
func resolveOrg(ctx context.Context, c *client.Client, ref string) (jaRes, error) {
	ref = strings.TrimSpace(ref)
	if _, err := strconv.Atoi(ref); err == nil {
		raw, err := c.GetNoCache(ctx, "/v1/organizations/"+url.PathEscape(ref), nil)
		if err != nil {
			return jaRes{}, err
		}
		orgs, err := decodeJA(raw)
		if err != nil || len(orgs) == 0 {
			return jaRes{}, fmt.Errorf("organization %s not found", ref)
		}
		if orgs[0].ID == "" {
			orgs[0].ID = ref
		}
		return orgs[0], nil
	}
	orgs, _, err := fetchAllPages(ctx, c, "/v1/organizations/all", nil, 0)
	if err != nil {
		return jaRes{}, err
	}
	return matchOrg(orgs, ref)
}

func matchOrg(orgs []jaRes, ref string) (jaRes, error) {
	want := strings.ToLower(ref)
	var exact, partial []jaRes
	for _, o := range orgs {
		name := strings.ToLower(o.str("name"))
		switch {
		case o.ID == ref:
			return o, nil
		case name == want:
			exact = append(exact, o)
		case strings.Contains(name, want):
			partial = append(partial, o)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		partial = exact // ambiguous exact names: list them with ids below
	} else if len(partial) == 1 {
		return partial[0], nil
	}
	if len(partial) > 1 {
		names := make([]string, 0, len(partial))
		for _, o := range partial {
			names = append(names, fmt.Sprintf("%s (%s)", humanText(o.str("name")), humanText(o.ID)))
		}
		return jaRes{}, usageErr(fmt.Errorf("--org %q matches several organizations: %s; pass the organization id", ref, strings.Join(names, ", ")))
	}
	return jaRes{}, usageErr(fmt.Errorf("--org %q matches no organization visible to this API key", ref))
}

// normalizeDomain lowercases and strips scheme, path, port, wildcard and trailing dot.
func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "]") {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "*.")
	return strings.TrimSuffix(s, ".")
}

// domainCovers reports whether a list entry applies to fqdn: the entry itself
// or any subdomain of it. ponytail: assumes DNSFilter list entries cover
// subdomains; verify against a live tenant before relying on a parent-domain match.
func domainCovers(entry, fqdn string) bool {
	entry, fqdn = normalizeDomain(entry), normalizeDomain(fqdn)
	return entry != "" && (fqdn == entry || strings.HasSuffix(fqdn, "."+entry))
}

// hasExactEntry reports an allow/block entry for exactly fqdn. DNSFilter
// documents exceptions to subdomain coverage, so only an exact entry proves a
// domain is listed.
func hasExactEntry(entries []string, fqdn string) bool {
	fqdn = normalizeDomain(fqdn)
	for _, e := range entries {
		if normalizeDomain(e) == fqdn {
			return true
		}
	}
	return false
}

func firstCovering(entries []string, fqdn string) string {
	for _, e := range entries {
		if domainCovers(e, fqdn) {
			return e
		}
	}
	return ""
}

// scheduleSlot returns the 0..671 index of the 15-minute block containing t in
// tz, counted from Monday 00:00. The schema says "672 ids representing 15
// minute blocks starting Monday morning 12:01am"; the vendor scheduling guide
// (help.dnsfilter.com 1500008111321) uses quarter-hour boundaries, so the
// first block is 00:00-00:14.
func scheduleSlot(t time.Time, tz string) int {
	if loc, err := time.LoadLocation(tz); err == nil && tz != "" {
		t = t.In(loc)
	}
	day := (int(t.Weekday()) + 6) % 7
	return day*96 + t.Hour()*4 + t.Minute()/15
}

// compareSemver compares dotted numeric versions ("3.1.0" vs "3.10"); missing
// parts count as 0. A prerelease ("3.1.0-beta") sorts before its release;
// two prereleases of the same core compare as strings.
func compareSemver(a, b string) int {
	a, _, _ = strings.Cut(strings.TrimPrefix(a, "v"), "+")
	b, _, _ = strings.Cut(strings.TrimPrefix(b, "v"), "+")
	coreA, preA, _ := strings.Cut(a, "-")
	coreB, preB, _ := strings.Cut(b, "-")
	if c := compareCore(coreA, coreB); c != 0 {
		return c
	}
	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	fa, fb := strings.Split(preA, "."), strings.Split(preB, ".")
	for i := 0; i < len(fa) && i < len(fb); i++ {
		numA, numB := isDigits(fa[i]), isDigits(fb[i])
		switch {
		case numA && numB:
			if c := compareDigits(fa[i], fb[i]); c != 0 {
				return c
			}
		case numA:
			return -1
		case numB:
			return 1
		case fa[i] != fb[i]:
			return sign(strings.Compare(fa[i], fb[i]))
		}
	}
	return sign(len(fa) - len(fb))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func compareCore(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = leadingDigits(pa[i])
		}
		if i < len(pb) {
			y = leadingDigits(pb[i])
		}
		if c := compareDigits(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// compareDigits compares unsigned decimal strings of any length (no overflow):
// strip leading zeros, then the longer number is larger, else compare lexically.
func compareDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return sign(len(a) - len(b))
	}
	return sign(strings.Compare(a, b))
}

func isDigits(s string) bool {
	return s != "" && leadingDigits(s) == s
}

// isUninstalled covers every uninstall marker the spec documents.
func isUninstalled(a jaRes) bool {
	switch a.str("status") {
	case "uninstalled", "uninstalled_from_client":
		return true
	}
	return a.str("uninstalled_at") != "" || a.str("agent_state") == "uninstalled"
}

func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

// openLocalMirror opens the synced store read-only. ok=false (with empty
// already written in the command's own result shape) means there is no
// mirror yet. The stderr line starts with "hint:" so the MCP shell-out
// forwards it to agents instead of leaving them a silent empty result.
func openLocalMirror(cmd *cobra.Command, flags *rootFlags, dbPath, syncHint string, empty any) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("dnsfilter-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; this command reads synced data only. Run: dnsfilter-cli sync --full --resources %s --db %s\n", dbPath, syncHint, dbPath)
		if !wantsHumanTable(cmd.OutOrStdout(), flags) {
			return nil, false, printJSONFiltered(cmd.OutOrStdout(), empty, flags)
		}
		return nil, false, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening local mirror: %w", err)
	}
	return db, true, nil
}

// incompleteCollections returns, in the order given, every collection a
// local-mirror answer joins that the mirror cannot vouch for: never synced,
// or its last sync attempt did not finish a full walk
// (sync_state.last_attempt_complete is 1 only after one). A read-only store
// that predates that column proves nothing, so every collection counts as
// incomplete there, the same rule mcpStoreStatus applies.
func incompleteCollections(ctx context.Context, db mirrorReader, collections ...string) ([]string, error) {
	out := make([]string, 0)
	for _, c := range collections {
		var complete int
		err := db.QueryRowContext(ctx, `SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, c).Scan(&complete)
		if err == nil && complete == 1 {
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			if msg := strings.ToLower(err.Error()); !strings.Contains(msg, "no such table") && !strings.Contains(msg, "no such column") {
				return nil, fmt.Errorf("reading sync state for %s: %w", c, err)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// incompleteSyncNote is the note a local-mirror command prints, in place of
// its all-clear, when incompleteCollections reported missing collections.
// The remedy is --full: a plain sync resumes an interrupted or page-capped
// walk from its saved cursor and still records it incomplete.
func incompleteSyncNote(missing []string) string {
	return fmt.Sprintf("Incomplete local mirror: no complete sync of %s, so this is not an all-clear (clients or rows may be missing, counts may be low). Run: dnsfilter-cli sync --full --resources %s", strings.Join(missing, ", "), strings.Join(missing, ","))
}

// incompleteMirrorResult replaces a list command's bare JSON rows when the
// mirror is incomplete. It is the {meta, results} envelope --agent already
// prints, so --agent keeps its results array and gains the fields in meta,
// and plain --json can no longer read as a clean empty list.
type incompleteMirrorResult struct {
	Meta struct {
		IncompleteCollections []string `json:"incomplete_collections"`
		Note                  string   `json:"note"`
		// UnresolvedSchedules (lists where) names scheduled policies that
		// deployments use but the mirror lacks or that name no slot policy.
		UnresolvedSchedules []string `json:"unresolved_schedules,omitempty"`
	} `json:"meta"`
	Results any `json:"results"`
}

func incompleteMirrorJSON(rows any, missing []string) incompleteMirrorResult {
	var r incompleteMirrorResult
	r.Meta.IncompleteCollections, r.Meta.Note, r.Results = missing, incompleteSyncNote(missing), rows
	return r
}

// mirrorReader is what loadLocal and incompleteCollections query: a read
// transaction from mirrorSnapshot (or the store's *sql.DB in tests).
type mirrorReader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// mirrorSnapshot opens one read transaction on the mirror. The store is WAL,
// so every read inside it sees the same committed state: a sync that commits
// rows and marks a collection complete between two reads cannot pair the old
// rows with the new completeness flag. The caller rolls it back when done.
func mirrorSnapshot(ctx context.Context, db *store.Store) (*sql.Tx, error) {
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("opening a read snapshot of the local mirror: %w", err)
	}
	return tx, nil
}

// mirrorRowsReadHook, when set by a test, runs after a reader has loaded its
// rows and before it reads sync_state.
var mirrorRowsReadHook func()

func afterMirrorRowsRead() {
	if mirrorRowsReadHook != nil {
		mirrorRowsReadHook()
	}
}

// loadLocal returns every synced row of one resource type (drain-first).
func loadLocal(ctx context.Context, db mirrorReader, resourceType string) ([]jaRes, error) {
	rows, err := db.QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", resourceType, err)
	}
	raws := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s: %w", resourceType, err)
		}
		raws = append(raws, s)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]jaRes, 0, len(raws))
	for i, s := range raws {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil || m == nil {
			return nil, fmt.Errorf("local mirror: %s row %d is not a JSON object; re-run sync", resourceType, i)
		}
		out = append(out, toJARes(m))
	}
	return out, nil
}

func byID(items []jaRes) map[string]jaRes {
	m := make(map[string]jaRes, len(items))
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- policy resolution shared by why-blocked and allow ----

type orgRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type categoryRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Security bool   `json:"security"`
	Unknown  bool   `json:"unknown,omitempty"` // referenced by the lookup but absent from /v1/categories
}

type deployment struct {
	Kind              string `json:"kind"` // site or roaming_client
	ID                string `json:"id"`
	Name              string `json:"name"`
	PolicyID          string `json:"policy_id"`
	PolicyName        string `json:"policy_name"`
	Via               string `json:"via"`
	ScheduledPolicyID string `json:"scheduled_policy_id,omitempty"`
	NetworkID         string `json:"-"`
}

type blockContext struct {
	FQDN       string
	Org        orgRef
	Categories []categoryRef
	// CategoriesKnown is false when the domain lookup returned no domain
	// record, so "no categories" cannot be trusted.
	CategoriesKnown bool
	Policies        map[string]jaRes
	Deployments     []deployment
	Notes           []string
}

// attributionNote is attached to every why-blocked / allow result: the policy
// API exposes site and roaming-client assignments only.
const attributionNote = "user, collection and LAN-subnet policy assignments (which outrank site and roaming-client assignments) and the org/MSP Universal Allow and Block Lists are not exposed by the policy API and are not evaluated; recent query-log hits show the policy that actually applied"

// resolveBlockContext gathers the client's deployments, the policy each one is
// using right now (scheduled slot applied, roaming clients inheriting their
// site), and the domain's categories.
func resolveBlockContext(ctx context.Context, c *client.Client, fqdn, orgRefStr, host string, maxPages int, now time.Time) (*blockContext, error) {
	org, err := resolveOrg(ctx, c, orgRefStr)
	if err != nil {
		return nil, err
	}
	bc := &blockContext{FQDN: normalizeDomain(fqdn), Org: orgRef{ID: org.ID, Name: org.str("name")}, Policies: map[string]jaRes{}, Notes: []string{attributionNote}}

	policies, _, err := fetchAllPages(ctx, c, "/v1/policies/all", map[string]string{"organization_id": org.ID, "include_global_policies": "true"}, 0)
	if err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}
	for _, p := range policies {
		bc.Policies[p.ID] = p
	}

	networks, truncated, err := fetchAllPages(ctx, c, "/v1/networks/all", nil, maxPages)
	if err != nil {
		return nil, fmt.Errorf("listing sites: %w", err)
	}
	if truncated {
		bc.Notes = append(bc.Notes, fmt.Sprintf("site scan stopped at --max-scan-pages=%d; raise it if a site is missing", maxPages))
	}
	sites := map[string]jaRes{}
	for _, n := range networks {
		if n.relID("organization") == org.ID {
			sites[n.ID] = n
		}
	}

	if host != "" {
		matched, err := resolveHost(ctx, c, org.ID, host, sites, maxPages)
		if err != nil {
			return nil, err
		}
		for _, a := range matched {
			name := a.str("friendly_name")
			if name == "" {
				name = a.str("hostname")
			}
			d := deployment{Kind: "roaming_client", ID: a.ID, Name: name, PolicyID: a.relID("policy"), ScheduledPolicyID: a.relID("scheduled_policy"), NetworkID: a.relID("network"), Via: "roaming client policy"}
			// A client with no own assignment inherits its site's policy or schedule.
			if d.PolicyID == "" && d.ScheduledPolicyID == "" {
				site, ok := sites[d.NetworkID]
				if !ok && d.NetworkID != "" {
					if raw, err := c.GetNoCache(ctx, "/v1/networks/"+url.PathEscape(d.NetworkID), nil); err == nil {
						if list, _ := decodeJA(raw); len(list) > 0 {
							site, ok = list[0], true
						}
					}
				}
				if ok {
					d.PolicyID, d.ScheduledPolicyID = site.relID("policy"), site.relID("scheduled_policy")
					d.Via = fmt.Sprintf("inherited from site %q", site.str("name"))
				}
			}
			bc.Deployments = append(bc.Deployments, d)
		}
		if len(bc.Deployments) == 0 {
			return nil, usageErr(fmt.Errorf("--host %q matches no installed roaming client in %s", host, humanText(bc.Org.Name)))
		}
	} else {
		for _, id := range sortedKeys(sites) {
			n := sites[id]
			bc.Deployments = append(bc.Deployments, deployment{Kind: "site", ID: n.ID, Name: n.str("name"), PolicyID: n.relID("policy"), ScheduledPolicyID: n.relID("scheduled_policy"), NetworkID: n.ID, Via: "site policy"})
		}
		if len(bc.Deployments) == 0 {
			bc.Notes = append(bc.Notes, "no sites found for this organization; pass --host to check a roaming client")
		}
	}

	// A scheduled policy swaps in one of 672 per-slot policies. If the schedule
	// cannot be read or has no policy for the current slot, the effective
	// policy is unknown: never fall back to the plain assignment.
	schedCache := map[string]*jaRes{}
	for i := range bc.Deployments {
		d := &bc.Deployments[i]
		if d.ScheduledPolicyID == "" {
			continue
		}
		sp, seen := schedCache[d.ScheduledPolicyID]
		if !seen {
			schedCache[d.ScheduledPolicyID] = nil
			if raw, err := c.GetNoCache(ctx, "/v1/scheduled_policies/"+url.PathEscape(d.ScheduledPolicyID), nil); err != nil {
				bc.Notes = append(bc.Notes, fmt.Sprintf("scheduled policy %s unreadable: %v", d.ScheduledPolicyID, err))
			} else if list, _ := decodeJA(raw); len(list) > 0 {
				schedCache[d.ScheduledPolicyID] = &list[0]
			}
			sp = schedCache[d.ScheduledPolicyID]
		}
		if id, via, ok := scheduledPolicyNow(sp, now); ok {
			d.PolicyID, d.Via = id, via
		} else {
			d.PolicyID, d.Via = "", fmt.Sprintf("scheduled policy %s: current slot unresolved", d.ScheduledPolicyID)
		}
	}
	for i := range bc.Deployments {
		d := &bc.Deployments[i]
		if d.PolicyID == "" {
			continue
		}
		if p, err := ensurePolicy(ctx, c, bc.Policies, d.PolicyID); err == nil {
			d.PolicyName = p.str("name")
		}
	}

	cats, known, err := lookupCategories(ctx, c, bc.FQDN)
	if err != nil {
		return nil, err
	}
	bc.Categories, bc.CategoriesKnown = cats, known
	if !known {
		bc.Notes = append(bc.Notes, "domain lookup returned no domain record; categorization is unavailable, so category and uncategorized verdicts are unreliable")
	}
	return bc, nil
}

// rejectExtraArgs refuses positional arguments beyond max. A boolean flag
// followed by a separate value (`--confirm false`) parses as the flag plus a
// stray positional; silently ignoring it would turn "false" into true.
func rejectExtraArgs(args []string, max int) error {
	if len(args) <= max {
		return nil
	}
	return usageErr(fmt.Errorf("unexpected argument %q; boolean flags take --flag=false, not --flag false", args[max]))
}

// resolveHost finds the single roaming client --host names in the org. A
// roaming-client id is read directly (the list search does not match ids) and
// must sit on one of the org's sites; a hostname or friendly name is matched
// only after a complete scan, so an unread page cannot hide a duplicate.
func resolveHost(ctx context.Context, c *client.Client, orgID, host string, sites map[string]jaRes, maxPages int) ([]jaRes, error) {
	// siteInOrg checks a client's site against the org, reading the site
	// directly when it is not among the scanned ones. known is false when the
	// site could not be read, which is never treated as "not in the org".
	siteInOrg := func(a jaRes) (in, known bool) {
		netID := a.relID("network")
		if _, ok := sites[netID]; ok {
			return true, true
		}
		if netID == "" {
			return false, true
		}
		nraw, err := c.GetNoCache(ctx, "/v1/networks/"+url.PathEscape(netID), nil)
		if err != nil {
			return false, false
		}
		nets, derr := decodeJA(nraw)
		if derr != nil || len(nets) != 1 || nets[0].ID != netID || nets[0].relID("organization") == "" {
			// No usable organization link: membership is unknown, not "outside".
			return false, false
		}
		if nets[0].relID("organization") == orgID {
			sites[netID] = nets[0]
			return true, true
		}
		return false, true
	}
	id := strings.TrimSpace(host)
	if raw, err := c.GetNoCache(ctx, "/v1/user_agents/"+url.PathEscape(id), nil); err == nil {
		if list, derr := decodeJA(raw); derr == nil && len(list) == 1 && list[0].ID == id && !isUninstalled(list[0]) {
			switch in, known := siteInOrg(list[0]); {
			case !known:
				return nil, fmt.Errorf("--host %q: could not read the site of roaming client %s to confirm it belongs to the organization", host, id)
			case in:
				return list, nil
			default:
				return nil, usageErr(fmt.Errorf("--host %q: roaming client %s is not on a site of this organization", host, id))
			}
		}
	}
	agents, truncated, err := fetchAllPages(ctx, c, "/v1/user_agents/all", map[string]string{"organization_id": orgID, "search": host}, maxPages)
	if err != nil {
		return nil, fmt.Errorf("listing roaming clients: %w", err)
	}
	if truncated {
		return nil, usageErr(fmt.Errorf("--host %q: the roaming-client scan stopped at --max-scan-pages=%d before it could prove the name is unique; raise --max-scan-pages or pass the roaming-client id", host, maxPages))
	}
	// Never trust the list's org scoping for a write: drop clients whose site
	// is not verified to belong to the org before choosing one.
	scoped := make([]jaRes, 0, len(agents))
	unverified := 0
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		switch in, known := siteInOrg(a); {
		case !known:
			unverified++
		case in:
			scoped = append(scoped, a)
		}
	}
	if unverified > 0 {
		// An unverifiable candidate could be the duplicate that makes the
		// name ambiguous, so a name cannot be trusted for a write.
		return nil, fmt.Errorf("--host %q: could not read the site of %d matching roaming client(s); retry or pass the roaming-client id", host, unverified)
	}
	return matchHosts(scoped, host)
}

// matchHosts picks the one roaming client --host names: a roaming-client id,
// else a unique exact hostname or friendly name (case-insensitive), else a
// unique partial match. Several candidates at the deciding tier are refused as
// ambiguous so a write can never fan out to devices the operator did not name.
func matchHosts(agents []jaRes, host string) ([]jaRes, error) {
	raw := strings.TrimSpace(host)
	want := strings.ToLower(raw)
	byIDMatch, exact, partial := make([]jaRes, 0), make([]jaRes, 0), make([]jaRes, 0)
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		h, f := strings.ToLower(a.str("hostname")), strings.ToLower(a.str("friendly_name"))
		switch {
		case a.ID == raw:
			byIDMatch = append(byIDMatch, a)
		case h == want || f == want:
			exact = append(exact, a)
		case strings.Contains(h, want) || strings.Contains(f, want):
			partial = append(partial, a)
		}
	}
	for _, tier := range [][]jaRes{byIDMatch, exact, partial} {
		switch {
		case len(tier) == 1:
			return tier, nil
		case len(tier) > 1:
			names := make([]string, 0, len(tier))
			for i, a := range tier {
				if i == 10 {
					names = append(names, fmt.Sprintf("and %d more", len(tier)-10))
					break
				}
				names = append(names, fmt.Sprintf("%s (id %s)", humanText(a.str("hostname")), humanText(a.ID)))
			}
			return nil, usageErr(fmt.Errorf("--host %q matches several roaming clients (%s); pass one roaming-client id as --host", host, strings.Join(names, ", ")))
		}
	}
	return []jaRes{}, nil
}

// scheduledPolicyNow returns the policy id for now's 15-minute slot; ok is
// false unless the schedule has all 672 slots, a loadable timezone, and a
// non-empty id for this slot.
func scheduledPolicyNow(sp *jaRes, now time.Time) (string, string, bool) {
	if sp == nil {
		return "", "", false
	}
	slots := sp.strs("policy_ids")
	tz := sp.str("timezone")
	if len(slots) != 672 {
		return "", "", false
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "" {
		return "", "", false
	}
	slot := scheduleSlot(now, tz)
	if slots[slot] == "" || slots[slot] == "0" {
		return "", "", false
	}
	return slots[slot], fmt.Sprintf("scheduled policy %q slot %d (%s)", sp.str("name"), slot, tz), true
}

func ensurePolicy(ctx context.Context, c *client.Client, cache map[string]jaRes, id string) (jaRes, error) {
	if p, ok := cache[id]; ok {
		return p, nil
	}
	raw, err := c.GetNoCache(ctx, "/v1/policies/"+url.PathEscape(id), nil)
	if err != nil {
		return jaRes{}, err
	}
	list, err := decodeJA(raw)
	if err != nil || len(list) != 1 || list[0].ID != id {
		// Only the requested policy may authorize a write: never cache a
		// different (or unidentified) resource under this id.
		return jaRes{}, fmt.Errorf("policy %s: response is not that policy", id)
	}
	cache[id] = list[0]
	return list[0], nil
}

// lookupCategories resolves the domain's category ids to names and security
// flags. Decode failures are errors; ids missing from /v1/categories are kept
// as Unknown so callers can refuse to treat them as safe.
func lookupCategories(ctx context.Context, c *client.Client, fqdn string) ([]categoryRef, bool, error) {
	raw, err := c.GetNoCache(ctx, "/v1/domains/user_lookup", map[string]string{"fqdn": fqdn})
	if err != nil {
		return nil, false, fmt.Errorf("domain lookup: %w", err)
	}
	lookups, err := decodeJA(raw)
	if err != nil {
		return nil, false, fmt.Errorf("domain lookup: %w", err)
	}
	known := false
	catIDs := map[string]bool{}
	for _, l := range lookups {
		ok, ids := domainRecordCategories(l)
		if !ok {
			continue
		}
		known = true
		for _, id := range ids {
			catIDs[id] = true
		}
	}
	out := make([]categoryRef, 0, len(catIDs))
	if len(catIDs) == 0 {
		return out, known, nil
	}
	cats, _, err := fetchAllPages(ctx, c, "/v1/categories/all", nil, 0)
	if err != nil {
		return nil, false, fmt.Errorf("listing categories: %w", err)
	}
	byCat := byID(cats)
	for _, id := range sortedKeys(catIDs) {
		cat, ok := byCat[id]
		// Threat status is known only when the catalog row carries a real
		// boolean security flag; anything else fails closed as Unknown.
		security, isBool := cat.Attrs["security"].(bool)
		out = append(out, categoryRef{ID: id, Name: cat.str("name"), Security: security, Unknown: !ok || !isBool})
	}
	return out, known, nil
}

// queryLogScanPages bounds the attribution scan: the API's fqdn filter is a
// substring search, so subdomain hits can fill pages before exact ones.
const queryLogScanPages = 5

// domainRecordCategories validates one user_lookup record: it must name a host
// (attributes host or name) and carry relationships.categories.data as a list
// whose every entry has an id. ok=false means the categorization is unknown.
func domainRecordCategories(l jaRes) (bool, []string) {
	if l.str("host") == "" && l.str("name") == "" {
		return false, nil
	}
	rel, _ := l.Rels["categories"].(map[string]any)
	data, isList := rel["data"].([]any)
	if !isList {
		return false, nil
	}
	ids := l.relIDs("categories")
	if len(ids) != len(data) {
		return false, nil
	}
	return true, ids
}

// fetchBlockedHits returns every recent query-log hit for exactly fqdn within
// the page cap (callers trim for display; attribution uses them all),
// optionally scoped to roaming clients. complete is false when the cap stopped
// the walk before a short page proved the log exhausted.
func fetchBlockedHits(ctx context.Context, c *client.Client, orgID, fqdn string, agentIDs []string, since time.Duration, now time.Time) (hits []queryLogHit, complete bool, err error) {
	hits = make([]queryLogHit, 0)
	for page := 1; page <= queryLogScanPages; page++ {
		params := map[string]string{
			"organization_id": orgID,
			"fqdn":            fqdn,
			"result":          "blocked",
			"from":            now.Add(-since).UTC().Format("2006-01-02T15:04:05Z"),
			"page[number]":    strconv.Itoa(page),
			"page[size]":      strconv.Itoa(dnsfilterPageSize),
		}
		if len(agentIDs) > 0 {
			params["agent_ids"] = strings.Join(agentIDs, ",")
		}
		raw, err := c.GetNoCache(ctx, "/v1/traffic_reports/query_logs", params)
		if err != nil {
			return hits, false, err
		}
		batch, err := parseQueryLogHits(raw)
		if err != nil {
			return hits, false, fmt.Errorf("query log: %w", err)
		}
		for _, h := range batch {
			if normalizeDomain(h.FQDN) == fqdn {
				hits = append(hits, h)
			}
		}
		if len(batch) < dnsfilterPageSize {
			return hits, true, nil
		}
	}
	return hits, false, nil
}

type verdict struct {
	Decision string `json:"decision"` // allowed, blocked, or unknown
	Rule     string `json:"rule"`
	Detail   string `json:"detail"`
}

// evaluatePolicy applies DNSFilter's documented policy-level precedence (Allow
// List, Block List, uncategorized domains, filtering categories; see
// help.dnsfilter.com article 1500008108522) to one effective policy.
// Universal Lists and AppAware application rules are not visible on the
// policy resource and are not evaluated (see attributionNote).
//
// DNSFilter documents exceptions to subdomain coverage, so a parent-domain
// entry cannot be assumed to apply, and allow-list and block-list parents can
// apply independently. The policy is evaluated under every combination of
// "parent allow entries apply" x "parent block entries apply" (exact entries
// always apply); when all four agree the exact-only verdict stands, otherwise
// the verdict is unknown and names the deciding parent entries.
func evaluatePolicy(fqdn string, policy jaRes, cats []categoryRef, categoriesKnown bool) verdict {
	exactOnly := evaluateLists(fqdn, policy, cats, categoriesKnown, false, false)
	outcomes := map[string]bool{exactOnly.Decision: true}
	for _, allowParents := range []bool{false, true} {
		for _, blockParents := range []bool{false, true} {
			outcomes[evaluateLists(fqdn, policy, cats, categoriesKnown, allowParents, blockParents).Decision] = true
		}
	}
	if len(outcomes) == 1 {
		return exactOnly
	}
	parents := make([]string, 0, 2)
	for _, list := range []string{"whitelist_domains", "blacklist_domains"} {
		if e := firstCovering(policy.strs(list), fqdn); e != "" && normalizeDomain(e) != normalizeDomain(fqdn) {
			parents = append(parents, fmt.Sprintf("%q (%s)", e, list))
		}
	}
	return verdict{"unknown", "parent_entry_unverified", fmt.Sprintf("the outcome depends on whether parent entries %s apply (DNSFilter documents subdomain exceptions; check the query log); exact entries alone give %s", strings.Join(parents, ", "), exactOnly.Decision)}
}

func evaluateLists(fqdn string, policy jaRes, cats []categoryRef, categoriesKnown, allowParents, blockParents bool) verdict {
	listed := func(entries []string, parents bool) string {
		if parents {
			return firstCovering(entries, fqdn)
		}
		if hasExactEntry(entries, fqdn) {
			return fqdn
		}
		return ""
	}
	if e := listed(policy.strs("whitelist_domains"), allowParents); e != "" {
		return verdict{"allowed", "allow_list", fmt.Sprintf("%q on policy %q", e, policy.str("name"))}
	}
	if policy.boolean("allow_list_only") {
		return verdict{"blocked", "allow_list_only", fmt.Sprintf("policy %q only allows allow-listed domains", policy.str("name"))}
	}
	if e := listed(policy.strs("blacklist_domains"), blockParents); e != "" {
		return verdict{"blocked", "block_list", fmt.Sprintf("%q on policy %q", e, policy.str("name"))}
	}
	if !categoriesKnown {
		return verdict{"unknown", "categorization_unavailable", fmt.Sprintf("no list entry in policy %q matches and the domain's categories could not be read", policy.str("name"))}
	}
	if len(cats) == 0 && !policy.boolean("allow_unknown_domains") {
		return verdict{"blocked", "uncategorized", fmt.Sprintf("domain has no category and policy %q blocks unknown domains", policy.str("name"))}
	}
	blocked := map[string]bool{}
	for _, id := range policy.strs("blacklist_categories") {
		blocked[id] = true
	}
	for _, cat := range cats {
		if blocked[cat.ID] {
			return verdict{"blocked", "category", fmt.Sprintf("category %q (%s) is blocked by policy %q", cat.Name, cat.ID, policy.str("name"))}
		}
	}
	return verdict{"allowed", "no_matching_rule", fmt.Sprintf("no list entry or blocked category in policy %q matches", policy.str("name"))}
}
