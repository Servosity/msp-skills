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
	"maps"
	"math"
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

// idString reads a resource id: a string, or a whole non-negative number.
// HAND-FIX(novel-strict-ids): a fractional (60.9), negative or non-numeric
// value is unreadable (""), never truncated into another id ("60").
func idString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t < 0 || t != math.Trunc(t) || math.IsInf(t, 0) {
			return ""
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		if s := t.String(); isDigits(s) {
			return s
		}
	case int:
		if t >= 0 {
			return strconv.Itoa(t)
		}
	}
	return ""
}

// str returns a string attribute ("" when absent, null or not a JSON
// string). HAND-FIX(novel-strict-ids): a number is not coerced into text,
// so agent_version 999 or domain 4821 is unreadable, not "999" or "4821".
// Integer-typed ids (organization_id, policy_id) are read with idAttr.
func (r jaRes) str(k string) string {
	s, _ := r.Attrs[k].(string)
	return s
}

// idAttr returns an id-valued attribute (organization_id, policy_id): a
// string or a whole non-negative number, "" otherwise.
func (r jaRes) idAttr(k string) string { return idString(r.Attrs[k]) }

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

// HAND-FIX(novel-presence-aware-attributes): boolean() and strs() turn an
// absent, null or mistyped attribute into false / an empty list. A verdict
// that rules something out needs the attribute to be known, so these
// variants report presence: ok=false is unknown, never false or empty.

// boolKnown returns a boolean attribute; ok is false unless it is a real boolean.
func (r jaRes) boolKnown(k string) (bool, bool) {
	b, ok := r.Attrs[k].(bool)
	return b, ok
}

// strsKnown returns a string-list attribute's readable entries (domain
// lists); ok is false when the attribute is absent, null or not a list, or
// any entry is not a non-empty JSON string (that entry is dropped, so the
// list is incomplete). HAND-FIX(novel-strict-ids): a number in a domain list
// is unreadable, not a domain.
func (r jaRes) strsKnown(k string) ([]string, bool) {
	arr, ok := r.Attrs[k].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if s, isStr := it.(string); isStr && s != "" {
			out = append(out, s)
		} else {
			ok = false
		}
	}
	return out, ok
}

// idsKnown is strsKnown for a list of ids (blacklist_categories): every entry
// must be an id idString reads (a string, or a whole non-negative number).
func (r jaRes) idsKnown(k string) ([]string, bool) {
	arr, ok := r.Attrs[k].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if s := idString(it); s != "" {
			out = append(out, s)
		} else {
			ok = false
		}
	}
	return out, ok
}

// strKnown returns a string attribute; ok is false unless it is a non-empty string.
func (r jaRes) strKnown(k string) (string, bool) {
	s, ok := r.Attrs[k].(string)
	return s, ok && s != ""
}

// joinNotes joins the non-empty notes with a space.
func joinNotes(notes ...string) string {
	kept := make([]string, 0, len(notes))
	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, " ")
}

// relID returns relationships.<name>.data.id ("" when absent, null or
// unreadable). A caller that would fall back on "" reads relRef instead.
func (r jaRes) relID(name string) string {
	id, _ := r.relRef(name)
	return id
}

// relRef returns relationships.<name>.data.id. HAND-FIX(novel-relationship-linkage-unknown):
// relID read {"data":{"id":null}} as "no assignment", so a site kept its
// direct policy under an unreadable schedule and a roaming client fell back
// to its site. known is false when the relationship is present but its
// linkage cannot be read (not an object, no data member, data not an object,
// or an id that is null or not an id); an absent relationship, a null one or
// data null is a known absence ("", true).
func (r jaRes) relRef(name string) (id string, known bool) {
	raw, present := r.Rels[name]
	if !present || raw == nil {
		return "", true
	}
	rel, isObj := raw.(map[string]any)
	if !isObj {
		return "", false
	}
	data, hasData := rel["data"]
	if !hasData {
		return "", false
	}
	if data == nil {
		return "", true
	}
	m, isObj := data.(map[string]any)
	if !isObj {
		return "", false
	}
	id = idString(m["id"])
	return id, id != ""
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

// decodeJAList decodes one page of a list endpoint. HAND-FIX(fetch-pages-null-collection-unknown):
// the collection must be a JSON array, bare or under "data"; null, a missing
// data key, or an object is an unreadable page, never an empty (exhausted) one.
func decodeJAList(raw json.RawMessage) ([]jaRes, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if m, isObj := v.(map[string]any); isObj {
		v = m["data"]
	}
	if _, isList := v.([]any); !isList {
		return nil, fmt.Errorf("decoding response: the page carries no list (got %T where a list was expected), so its rows are unknown", v)
	}
	return decodeJA(raw)
}

// novelWalkPageLimit bounds a walk that has no --max-scan-pages cap.
const novelWalkPageLimit = 10000

// fetchAllPages walks a V1 `page[number]` list until a page with no
// continuation that is short, or maxPages. truncated reports that maxPages
// stopped the walk with more data likely. HAND-FIX(novel-walk-follows-continuation):
// a short page ended the walk even when it named a next page, so later sites,
// policies or organizations were never read and verdicts were certified.
func fetchAllPages(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) (items []jaRes, truncated bool, err error) {
	items = make([]jaRes, 0)
	page := 1
	for fetched := 0; ; fetched++ {
		if maxPages > 0 && fetched >= maxPages {
			return items, true, nil
		}
		if fetched >= novelWalkPageLimit {
			return items, false, fmt.Errorf("decoding response: %s still names more rows after %d pages; the walk stopped without reaching the end", path, novelWalkPageLimit)
		}
		p := map[string]string{"page[number]": strconv.Itoa(page), "page[size]": strconv.Itoa(dnsfilterPageSize)}
		for k, v := range params {
			p[k] = v
		}
		raw, err := c.GetNoCache(ctx, path, p)
		if err != nil {
			return items, false, err
		}
		batch, err := decodeJAList(raw)
		if err != nil {
			return items, false, err
		}
		items = append(items, batch...)
		next, more, err := listPageContinuation(raw, page)
		if err != nil {
			return items, false, fmt.Errorf("decoding response: %s page %d: %w", path, page, err)
		}
		switch {
		case more && len(batch) == 0:
			return items, false, fmt.Errorf("decoding response: %s page %d lists no rows but says more follow, so the rest of the list is unknown", path, page)
		case !more && len(batch) < dnsfilterPageSize:
			return items, false, nil
		}
		page = next
	}
}

// listPageContinuation reads the explicit continuation one list page
// carries, the signals sync follows (syncPageEndsWalk): a next link or cursor
// (links.next, next_cursor, ...) or has_more true, plus meta.current_page
// below meta.total_pages. more reports any of them; next is the page the walk
// reads next (page+1). A continuation a page-number walk cannot follow (an
// opaque cursor, a link that names no page, or names a page other than the
// next one) is an error: the rest of the list is unknown, never empty.
func listPageContinuation(raw json.RawMessage, page int) (next int, more bool, err error) {
	next = page + 1
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil {
		return next, false, nil // a bare list carries no continuation
	}
	cursor, hasMore := extractPaginationFromEnvelope(env, "page[number]", "")
	if cursor == "" && linkNextPresent(env) {
		if cursor = linkNextPage(env); cursor == "" {
			return next, true, fmt.Errorf("the page names a next link this walk cannot follow, so the rest of the list is unknown")
		}
	}
	if cursor != "" {
		n, convErr := strconv.Atoi(cursor)
		switch {
		case convErr != nil:
			return next, true, fmt.Errorf("the page names continuation %q, which is not a page number, so the rest of the list is unknown", cursor)
		case n != next:
			return next, true, fmt.Errorf("the page names page %d after page %d, so the walk cannot prove it read every page", n, page)
		}
		return next, true, nil
	}
	return next, hasMore || metaPagesRemain(env), nil
}

// linkNextPresent reports a links.next (or _links.next) that is not null or "".
func linkNextPresent(env map[string]json.RawMessage) bool {
	for _, key := range []string{"links", "_links"} {
		var links map[string]json.RawMessage
		if json.Unmarshal(env[key], &links) != nil {
			continue
		}
		if raw, ok := links["next"]; ok {
			if s := strings.TrimSpace(string(raw)); s != "null" && s != `""` {
				return true
			}
		}
	}
	return false
}

// linkNextPage returns the page number a links.next URL carries as a plain
// page parameter ("?page=2"), "" when it carries none.
func linkNextPage(env map[string]json.RawMessage) string {
	for _, key := range []string{"links", "_links"} {
		var links map[string]json.RawMessage
		if json.Unmarshal(env[key], &links) != nil {
			continue
		}
		if u, err := url.Parse(paginationLinkURL(links["next"])); err == nil {
			if n := u.Query().Get("page"); isDigits(n) {
				return n
			}
		}
	}
	return ""
}

// metaPagesRemain reports meta.current_page below meta.total_pages (the
// unblock-request list's pagination metadata).
func metaPagesRemain(env map[string]json.RawMessage) bool {
	var meta struct {
		Current *float64 `json:"current_page"`
		Total   *float64 `json:"total_pages"`
	}
	if json.Unmarshal(env["meta"], &meta) != nil || meta.Current == nil || meta.Total == nil {
		return false
	}
	return *meta.Current < *meta.Total
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
		// HAND-FIX(resolve-org-identity): only the requested organization may
		// scope a verdict or a write; a response that names another (or no)
		// organization is not that organization.
		if len(orgs) != 1 || !sameID(orgs[0].ID, ref) {
			return jaRes{}, fmt.Errorf("organization %s: the response does not identify that organization (got id %q)", ref, humanText(orgs[0].ID))
		}
		return orgs[0], nil
	}
	orgs, _, err := fetchAllPages(ctx, c, "/v1/organizations/all", nil, 0)
	if err != nil {
		return jaRes{}, err
	}
	return matchOrg(orgs, ref)
}

// sameID reports two ids naming the same resource: equal text, or the same
// decimal number ("7" and "07"). An empty id matches nothing.
func sameID(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || (isDigits(a) && isDigits(b) && compareDigits(a, b) == 0)
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

// isComparableVersion reports whether compareSemver can order v: an optional
// leading "v", dot-separated numeric core parts, then an optional
// "-prerelease" and "+build" of dot-separated [0-9A-Za-z-] identifiers.
// HAND-FIX(agents-drift-incomparable-version-unknown): compareCore reads only
// leading digits, so "999unknown" would compare as 999 and read as current.
func isComparableVersion(v string) bool {
	v, build, hasBuild := strings.Cut(strings.TrimPrefix(v, "v"), "+")
	core, pre, hasPre := strings.Cut(v, "-")
	if (hasBuild && !validVersionIdents(build)) || (hasPre && !validVersionIdents(pre)) {
		return false
	}
	for _, part := range strings.Split(core, ".") {
		if !isDigits(part) {
			return false
		}
	}
	return true
}

func validVersionIdents(s string) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for _, r := range id {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
				return false
			}
		}
	}
	return true
}

// isUninstalled covers every uninstall marker the spec documents; it is true
// only for a readable one (uninstallState).
func isUninstalled(a jaRes) bool {
	uninstalled, _ := uninstallState(a)
	return uninstalled
}

// uninstallState reads a roaming client's uninstall markers.
// HAND-FIX(uninstalled-at-must-parse): any non-empty uninstalled_at
// ("not-a-date", 0) excluded a client from agents stale, agents drift and
// lists where. uninstalled is true only for status uninstalled /
// uninstalled_from_client, agent_state uninstalled, or an uninstalled_at
// that parses as a timestamp. known is false when no marker proves it and
// uninstalled_at is present but unreadable: such a client is kept (treated
// as installed) and flagged, never silently excluded.
func uninstallState(a jaRes) (uninstalled, known bool) {
	switch a.str("status") {
	case "uninstalled", "uninstalled_from_client":
		return true, true
	}
	if a.str("agent_state") == "uninstalled" {
		return true, true
	}
	switch v := a.Attrs["uninstalled_at"].(type) {
	case nil:
		return false, true
	case string:
		if v == "" {
			return false, true
		}
		if _, ok := parseSyncTime(v); ok {
			return true, true
		}
	}
	return false, false
}

// orgMembership applies --org (an org id, or a case-insensitive substring of
// its name) to a roaming client through its site. HAND-FIX(agents-org-membership-unknown):
// known is false when the client names no site, its site is missing from the
// mirror or names no organization, or the org's name cannot be read and its
// id does not match. Such a client can be neither matched nor ruled out, so
// callers report it instead of filtering it away. With no filter every client
// matches.
func orgMembership(a jaRes, netOrg map[string]string, orgByID map[string]jaRes, filter string) (orgID string, match, known bool) {
	// An unreadable site link is unknown membership, never "no site".
	netID, netKnown := a.relRef("network")
	linked := false
	if netID != "" {
		orgID, linked = netOrg[netID]
	}
	if filter == "" {
		return orgID, true, true
	}
	if !netKnown || !linked || orgID == "" {
		return orgID, false, false
	}
	if orgID == filter {
		return orgID, true, true
	}
	org, synced := orgByID[orgID]
	name, named := org.strKnown("name")
	if !synced || !named {
		return orgID, false, false
	}
	return orgID, strings.Contains(strings.ToLower(name), strings.ToLower(filter)), true
}

// siteOrgs maps each mirrored site id to its organization id ("" when the
// site names none); a site missing from the mirror has no entry.
func siteOrgs(networks []jaRes) map[string]string {
	netOrg := make(map[string]string, len(networks))
	for _, n := range networks {
		netOrg[n.ID] = n.relID("organization")
	}
	return netOrg
}

// membershipUnknownName labels the group of clients orgMembership could not place.
const membershipUnknownName = "(org membership unknown)"

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
		// deployments use but the mirror lacks, or that do not name a usable
		// policy in every one of their 672 slots.
		UnresolvedSchedules []string `json:"unresolved_schedules,omitempty"`
		// UnreadablePolicies (lists where) names policies whose allow or
		// block list is absent, null or holds an unreadable entry, so the
		// domain cannot be ruled out of them.
		UnreadablePolicies []string `json:"unreadable_policies,omitempty"`
		// UnplacedRoamingClients (lists where) counts installed roaming
		// clients with no assignment of their own whose site is missing from
		// the mirror: their policy, and so the impact counts, are unknown.
		UnplacedRoamingClients int `json:"unplaced_roaming_clients,omitempty"`
		// MissingPolicies (lists where) names policies a site, roaming
		// client or schedule slot uses that the mirror's policies lack.
		MissingPolicies []string `json:"missing_policies,omitempty"`
		// UnreadableAssignments (lists where) counts sites and roaming
		// clients whose policy, scheduled-policy or site link is unreadable.
		UnreadableAssignments int `json:"unreadable_assignments,omitempty"`
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
	// Kind is site, roaming_client (--host), or roaming_clients: the org's
	// roaming clients that share one assignment of their own (no --host).
	Kind              string `json:"kind"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	PolicyID          string `json:"policy_id"`
	PolicyName        string `json:"policy_name"`
	Via               string `json:"via"`
	ScheduledPolicyID string `json:"scheduled_policy_id,omitempty"`
	// Clients counts the roaming clients a roaming_clients row stands for.
	Clients   int    `json:"clients,omitempty"`
	NetworkID string `json:"-"`
}

type blockContext struct {
	FQDN       string
	Org        orgRef
	Categories []categoryRef
	// CategoriesKnown is false when the domain lookup returned no usable
	// record for this exact domain (or any unusable record), so "no
	// categories" cannot be trusted.
	CategoriesKnown bool
	Policies        map[string]jaRes
	// Listed holds the ids the org-scoped policy list returned (Policies
	// also caches policies read one by one, e.g. from a query-log hit).
	Listed      map[string]bool
	Deployments []deployment
	Notes       []string
	// Gaps name reasons the deployment set may be missing sites (a capped
	// site scan, sites with no organization link); each is also a note, and
	// any gap rules out an all-clear.
	Gaps []string
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
	bc := &blockContext{FQDN: normalizeDomain(fqdn), Org: orgRef{ID: org.ID, Name: org.str("name")}, Policies: map[string]jaRes{}, Listed: map[string]bool{}, Notes: []string{attributionNote}}

	policies, _, err := fetchAllPages(ctx, c, "/v1/policies/all", map[string]string{"organization_id": org.ID, "include_global_policies": "true"}, 0)
	if err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}
	for _, p := range policies {
		bc.Policies[p.ID] = p
		if p.ID != "" {
			bc.Listed[p.ID] = true
		}
	}

	networks, truncated, err := fetchAllPages(ctx, c, "/v1/networks/all", nil, maxPages)
	if err != nil {
		return nil, fmt.Errorf("listing sites: %w", err)
	}
	if truncated {
		bc.Notes = append(bc.Notes, fmt.Sprintf("site scan stopped at --max-scan-pages=%d; raise it if a site is missing", maxPages))
		if host == "" {
			bc.Gaps = append(bc.Gaps, "site scan capped")
		}
	}
	sites := map[string]jaRes{}
	unlinked := 0
	for _, n := range networks {
		switch n.relID("organization") {
		case org.ID:
			sites[n.ID] = n
		case "":
			unlinked++
		}
	}
	// HAND-FIX(why-blocked-aggregate-unknown): a site with no organization
	// link may belong to this org; it is not evaluated, so it is a gap.
	if unlinked > 0 && host == "" {
		bc.Notes = append(bc.Notes, fmt.Sprintf("%d sites name no organization, so they may belong to this org and were not evaluated", unlinked))
		bc.Gaps = append(bc.Gaps, "sites without an organization link")
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
			d := deployment{Kind: "roaming_client", ID: a.ID, Name: name, NetworkID: a.relID("network"), Via: "roaming client policy"}
			own := assignmentOf(a, &d, "roaming client's")
			// A client with no own assignment inherits its site's policy or
			// schedule; an unreadable own assignment never falls back.
			if own == assignmentNone {
				site, ok := sites[d.NetworkID]
				if !ok && d.NetworkID != "" {
					if raw, err := c.GetNoCache(ctx, "/v1/networks/"+url.PathEscape(d.NetworkID), nil); err == nil {
						// Only the client's own site may supply its policy.
						if list, _ := decodeJA(raw); len(list) == 1 && list[0].ID == d.NetworkID {
							site, ok = list[0], true
						}
					}
				}
				if ok {
					d.Via = fmt.Sprintf("inherited from site %q", site.str("name"))
					if assignmentOf(site, &d, "") == assignmentUnknown {
						d.Via += ", whose policy assignment is unreadable"
					}
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
			d := deployment{Kind: "site", ID: n.ID, Name: n.str("name"), NetworkID: n.ID, Via: "site policy"}
			assignmentOf(n, &d, "site's")
			bc.Deployments = append(bc.Deployments, d)
		}
		addOrgRoamingClients(ctx, c, bc, org.ID, sites, maxPages)
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
			} else if list, _ := decodeJA(raw); len(list) == 1 && (list[0].ID == "" || list[0].ID == d.ScheduledPolicyID) {
				// The show response carries no id per the spec; one that names
				// another schedule is not this schedule.
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
		bc.Notes = append(bc.Notes, "domain lookup returned no usable domain record for this exact domain (or an unusable record alongside it); categorization is unavailable, so category and uncategorized verdicts are unreliable")
	}
	return bc, nil
}

type assignmentKind int

const (
	assignmentNone    assignmentKind = iota // no policy or scheduled policy of its own
	assignmentSet                           // a readable policy and/or scheduled policy
	assignmentUnknown                       // a relationship that is present but unreadable
)

// assignmentOf copies r's policy and scheduled-policy relationships into d.
// HAND-FIX(novel-relationship-linkage-unknown): an unreadable one leaves d
// with no policy, so its verdict is unknown, and (when whose is set) says so
// in Via; it never reads as "no assignment", which would fall back to a
// direct policy or the site's.
func assignmentOf(r jaRes, d *deployment, whose string) assignmentKind {
	pol, polKnown := r.relRef("policy")
	sched, schedKnown := r.relRef("scheduled_policy")
	if !polKnown || !schedKnown {
		d.PolicyID, d.ScheduledPolicyID = "", ""
		if whose != "" {
			d.Via = whose + " policy assignment is unreadable"
		}
		return assignmentUnknown
	}
	d.PolicyID, d.ScheduledPolicyID = pol, sched
	if pol == "" && sched == "" {
		return assignmentNone
	}
	return assignmentSet
}

// addOrgRoamingClients adds, without --host, the org's roaming clients that
// carry an assignment of their own. HAND-FIX(why-blocked-org-roaming-clients):
// only sites were evaluated, so a roaming client whose own policy or
// scheduled policy (which outranks its site's) blocks the domain was never
// examined and not_blocked was certified. The org's roaming clients are
// listed (bounded by --max-scan-pages); those on one of the org's sites with
// an assignment of their own become one roaming_clients deployment per
// distinct assignment (an unreadable assignment is its own row with no
// policy, so its verdict is unknown). A client inheriting its site's
// assignment is covered by that site's row. A failed or capped scan, and a
// client whose site cannot be read, are gaps.
func addOrgRoamingClients(ctx context.Context, c *client.Client, bc *blockContext, orgID string, sites map[string]jaRes, maxPages int) {
	agents, truncated, err := fetchAllPages(ctx, c, "/v1/user_agents/all", map[string]string{"organization_id": orgID}, maxPages)
	if err != nil {
		bc.Notes = append(bc.Notes, fmt.Sprintf("roaming clients could not be listed (%v), so no roaming client's own policy assignment was evaluated; pass --host to check one", err))
		bc.Gaps = append(bc.Gaps, "roaming clients not listed")
		return
	}
	if truncated {
		bc.Notes = append(bc.Notes, fmt.Sprintf("roaming-client scan stopped at --max-scan-pages=%d; a roaming client beyond it may carry its own policy assignment; raise it or pass --host", maxPages))
		bc.Gaps = append(bc.Gaps, "roaming-client scan capped")
	}
	type group struct {
		d     deployment
		names []string
	}
	groups := map[string]*group{}
	unplaced := 0
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		netID, netKnown := a.relRef("network")
		if !netKnown || netID == "" {
			unplaced++
			continue
		}
		if _, onSite := sites[netID]; !onSite {
			// Not on a site this org's scan verified (a capped scan or an
			// unlinked site is already a gap).
			continue
		}
		d := deployment{Kind: "roaming_clients", Via: "roaming clients' own policy"}
		kind := assignmentOf(a, &d, "roaming clients'")
		if kind == assignmentNone {
			continue
		}
		key := d.PolicyID + "|" + d.ScheduledPolicyID
		if kind == assignmentUnknown {
			key = "?"
		}
		g := groups[key]
		if g == nil {
			g = &group{d: d}
			groups[key] = g
		}
		name := a.str("friendly_name")
		if name == "" {
			name = a.str("hostname")
		}
		if name == "" {
			name = a.ID
		}
		g.d.Clients++
		g.names = append(g.names, name)
	}
	for _, key := range sortedKeys(groups) {
		g := groups[key]
		shown := g.names[:min(3, len(g.names))]
		g.d.Name = strings.Join(shown, ", ")
		if len(g.names) > len(shown) {
			g.d.Name += fmt.Sprintf(" and %d more", len(g.names)-len(shown))
		}
		bc.Deployments = append(bc.Deployments, g.d)
	}
	if unplaced > 0 {
		bc.Notes = append(bc.Notes, fmt.Sprintf("%d roaming clients name no readable site, so their policy was not evaluated; pass --host to check one", unplaced))
		bc.Gaps = append(bc.Gaps, "roaming clients without a readable site")
	}
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
		netID, netKnown := a.relRef("network")
		if !netKnown {
			return false, false // an unreadable site link is unknown, not "no site"
		}
		if _, ok := sites[netID]; ok && netID != "" {
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

// scheduleSlotIDs validates a scheduled policy's raw policy_ids before any
// filtering. HAND-FIX(schedule-slots-validated-raw): strs() drops unusable
// entries, which shifts every later slot, so a 673-entry list with one null
// passed as 672. ok is true only for a list of exactly 672 entries that are
// each an id (a non-empty string or a non-negative whole number); entry i is
// slot i, and "0" names no policy.
func scheduleSlotIDs(sp jaRes) ([]string, bool) {
	raw, isList := sp.Attrs["policy_ids"].([]any)
	if !isList || len(raw) != 672 {
		return nil, false
	}
	slots := make([]string, len(raw))
	for i, v := range raw {
		switch t := v.(type) {
		case string:
			slots[i] = t
		case float64:
			if t < 0 || t != math.Trunc(t) {
				return nil, false
			}
			slots[i] = idString(t)
		default:
			return nil, false
		}
		if slots[i] == "" {
			return nil, false
		}
	}
	return slots, true
}

// scheduledPolicyNow returns the policy id for now's 15-minute slot; ok is
// false unless the schedule's raw list is 672 usable ids (scheduleSlotIDs),
// its timezone loads, and this slot names a policy (not "0").
func scheduledPolicyNow(sp *jaRes, now time.Time) (string, string, bool) {
	if sp == nil {
		return "", "", false
	}
	slots, valid := scheduleSlotIDs(*sp)
	tz := sp.str("timezone")
	if !valid {
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
	// HAND-FIX(lookup-categories-own-record-required): a usable record for
	// another host (a parent with categories []) does not certify this one.
	// Categories are known only when the requested domain has a usable record
	// of its own and no returned record is unusable (an unusable record's
	// categories are missing from the union).
	ownRecord, allUsable := false, true
	catIDs := map[string]bool{}
	for _, l := range lookups {
		ok, ids := domainRecordCategories(l)
		if !ok {
			allUsable = false
			continue
		}
		if lookupRecordHost(l) == fqdn {
			ownRecord = true
		}
		for _, id := range ids {
			catIDs[id] = true
		}
	}
	known := ownRecord && allUsable
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

// lookupRecordHost is the normalized host a lookup record describes: its
// host attribute, else its name ("" when it names neither).
func lookupRecordHost(l jaRes) string {
	if host := normalizeDomain(l.str("host")); host != "" {
		return host
	}
	return normalizeDomain(l.str("name"))
}

// fetchBlockedHits returns every recent query-log hit for exactly fqdn within
// the page cap (callers trim for display; attribution uses them all),
// optionally scoped to roaming clients. gap is "" only when the scan proved
// the log exhausted; otherwise it says why the attribution is incomplete (the
// cap stopped the walk, or entries carry no readable fqdn).
// HAND-FIX(novel-walk-follows-continuation): a short page ended the scan even
// when data.page named a next page or cursor. HAND-FIX(query-log-unreadable-fqdn):
// an entry with a null, missing or mistyped fqdn was dropped as a non-match,
// and the log was certified complete.
func fetchBlockedHits(ctx context.Context, c *client.Client, orgID, fqdn string, agentIDs []string, since time.Duration, now time.Time) (hits []queryLogHit, gap string, err error) {
	hits = make([]queryLogHit, 0)
	pageParams := map[string]string{"page[number]": "1"}
	unreadable := 0
	unreadableGap := func() string {
		if unreadable == 0 {
			return ""
		}
		return fmt.Sprintf("%d query-log entries carry no readable fqdn, so they cannot be ruled out as blocks of %s", unreadable, fqdn)
	}
	for page := 1; page <= queryLogScanPages; page++ {
		params := map[string]string{
			"organization_id": orgID,
			"fqdn":            fqdn,
			"result":          "blocked",
			"from":            now.Add(-since).UTC().Format("2006-01-02T15:04:05Z"),
			"page[size]":      strconv.Itoa(dnsfilterPageSize),
		}
		for k, v := range pageParams {
			params[k] = v
		}
		if len(agentIDs) > 0 {
			params["agent_ids"] = strings.Join(agentIDs, ",")
		}
		raw, err := c.GetNoCache(ctx, "/v1/traffic_reports/query_logs", params)
		if err != nil {
			return hits, "", err
		}
		batch, err := parseQueryLogHits(raw)
		if err != nil {
			return hits, "", fmt.Errorf("query log: %w", err)
		}
		for _, h := range batch {
			switch {
			case h.fqdnUnreadable:
				unreadable++
			case normalizeDomain(h.FQDN) == fqdn:
				hits = append(hits, h)
			}
		}
		next, more, err := queryLogContinuation(raw, page)
		if err != nil {
			return hits, "", fmt.Errorf("query log: page %d: %w", page, err)
		}
		if !more {
			if len(batch) < dnsfilterPageSize {
				return hits, unreadableGap(), nil
			}
			next = map[string]string{"page[number]": strconv.Itoa(page + 1)}
		}
		if maps.Equal(next, pageParams) {
			return hits, "", fmt.Errorf("query log: page %d names itself as the next page, so the scan cannot advance", page)
		}
		pageParams = next
	}
	gap = fmt.Sprintf("query-log scan stopped after %d pages; older hits may name other blocking policies", queryLogScanPages)
	if u := unreadableGap(); u != "" {
		gap += "; " + u
	}
	return hits, gap, nil
}

// queryLogContinuation reads where the next query-log page starts: the
// spec's data.page (next page number, or a cursor and cursor_offset), or the
// list continuations listPageContinuation reads. A continuation the scan
// cannot follow is an error.
func queryLogContinuation(raw json.RawMessage, page int) (next map[string]string, more bool, err error) {
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return nil, false, fmt.Errorf("response has no data object")
	}
	if rawPage, ok := env.Data["page"]; ok && !rawJSONNull(rawPage) {
		var pg map[string]any
		if json.Unmarshal(rawPage, &pg) != nil {
			return nil, true, fmt.Errorf("data.page is not an object, so the continuation is unknown")
		}
		if v, ok := pg["cursor"]; ok && v != nil {
			cursor, isStr := v.(string)
			if !isStr {
				return nil, true, fmt.Errorf("data.page.cursor is not a string, so the continuation is unknown")
			}
			if cursor != "" {
				next = map[string]string{"page[cursor]": cursor}
				if off, ok := pg["cursor_offset"]; ok && off != nil {
					s := idString(off)
					if s == "" {
						return nil, true, fmt.Errorf("data.page.cursor_offset is not a whole number, so the continuation is unknown")
					}
					next["page[cursor_offset]"] = s
				}
				return next, true, nil
			}
		}
		if v, ok := pg["next"]; ok && v != nil {
			n, convErr := strconv.Atoi(idString(v))
			switch {
			case convErr != nil:
				return nil, true, fmt.Errorf("data.page.next is not a page number, so the continuation is unknown")
			case n != page+1:
				return nil, true, fmt.Errorf("data.page.next names page %d after page %d", n, page)
			}
			return map[string]string{"page[number]": strconv.Itoa(n)}, true, nil
		}
	}
	n, more, err := listPageContinuation(raw, page)
	if err != nil || !more {
		return nil, more, err
	}
	return map[string]string{"page[number]": strconv.Itoa(n)}, true, nil
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
	// HAND-FIX(evaluate-policy-unknown-attributes): an attribute the policy
	// does not carry as a readable value (absent, null, mistyped, or a list
	// with an unreadable entry) is unknown, never false or empty. A match in
	// the readable part of a list still decides; otherwise, once the rules
	// reach an unknown attribute, the verdict is unknown.
	unreadable := func(attr string) verdict {
		return verdict{"unknown", "policy_attribute_unavailable", fmt.Sprintf("policy %q does not carry a readable %s, so the rules from it on cannot be evaluated", policy.str("name"), attr)}
	}
	// HAND-FIX(evaluate-policy-unknown-attribute-agreement): the verdict was
	// unknown at the first unreadable attribute even when every value of it
	// gives the same one (a block-list entry blocks whether or not
	// allow_list_only is set). either evaluates the rest of the chain, the
	// outcome when the attribute does not decide; when that agrees with
	// decided, the outcome when it does, the verdict stands.
	either := func(attr string, decided verdict, rest func() verdict) verdict {
		v := rest()
		if v.Decision == decided.Decision && v.Decision != "unknown" {
			v.Detail += fmt.Sprintf(" (%s is unreadable, but every value of it gives this verdict)", attr)
			return v
		}
		return unreadable(attr)
	}
	categoryRules := func() verdict {
		blockedCats, catsKnown := policy.idsKnown("blacklist_categories")
		blocked := map[string]bool{}
		for _, id := range blockedCats {
			blocked[id] = true
		}
		for _, cat := range cats {
			if blocked[cat.ID] {
				return verdict{"blocked", "category", fmt.Sprintf("category %q (%s) is blocked by policy %q", cat.Name, cat.ID, policy.str("name"))}
			}
		}
		if !catsKnown && len(cats) > 0 {
			return unreadable("blacklist_categories")
		}
		return verdict{"allowed", "no_matching_rule", fmt.Sprintf("no list entry or blocked category in policy %q matches", policy.str("name"))}
	}
	uncategorizedRules := func() verdict {
		if !categoriesKnown {
			return verdict{"unknown", "categorization_unavailable", fmt.Sprintf("no list entry in policy %q matches and the domain's categories could not be read", policy.str("name"))}
		}
		if len(cats) == 0 {
			blocksUnknown := verdict{"blocked", "uncategorized", fmt.Sprintf("domain has no category and policy %q blocks unknown domains", policy.str("name"))}
			allowUnknown, known := policy.boolKnown("allow_unknown_domains")
			if !known {
				return either("allow_unknown_domains", blocksUnknown, categoryRules)
			}
			if !allowUnknown {
				return blocksUnknown
			}
		}
		return categoryRules()
	}
	blockListRules := func() verdict {
		blockList, blockKnown := policy.strsKnown("blacklist_domains")
		if e := listed(blockList, blockParents); e != "" {
			return verdict{"blocked", "block_list", fmt.Sprintf("%q on policy %q", e, policy.str("name"))}
		}
		if !blockKnown {
			return either("blacklist_domains", verdict{Decision: "blocked"}, uncategorizedRules)
		}
		return uncategorizedRules()
	}
	allowListOnlyRules := func() verdict {
		onlyAllowList := verdict{"blocked", "allow_list_only", fmt.Sprintf("policy %q only allows allow-listed domains", policy.str("name"))}
		listOnly, listOnlyKnown := policy.boolKnown("allow_list_only")
		if !listOnlyKnown {
			return either("allow_list_only", onlyAllowList, blockListRules)
		}
		if listOnly {
			return onlyAllowList
		}
		return blockListRules()
	}
	allowList, allowKnown := policy.strsKnown("whitelist_domains")
	if e := listed(allowList, allowParents); e != "" {
		return verdict{"allowed", "allow_list", fmt.Sprintf("%q on policy %q", e, policy.str("name"))}
	}
	if !allowKnown {
		return either("whitelist_domains", verdict{Decision: "allowed"}, allowListOnlyRules)
	}
	return allowListOnlyRules()
}
