// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"jira-service-management-pp-cli/internal/client"
)

// Shared plumbing for the hand-written JSM and Assets commands.

const assetsBase = "https://api.atlassian.com/ex/jira/{cloudId}/jsm/assets/workspace/{workspaceId}/v1"

type jsmGetter interface {
	GetWithHeadersNoCache(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}

// jsmGet is an uncached read for the hand-written commands (they promise
// right-now state). It opts into Atlassian's experimental operations, which
// some of these walks hit (customers, request types); others ignore it.
func jsmGet(ctx context.Context, c jsmGetter, path string, params map[string]string) (json.RawMessage, error) {
	return c.GetWithHeadersNoCache(ctx, path, params, map[string]string{"X-ExperimentalApi": "opt-in"})
}

// flexString accepts a JSON string or number (Atlassian mixes them for ids).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexString(v)
		return nil
	}
	*f = flexString(s)
	return nil
}

type jsmPage struct {
	Limit  int               `json:"limit"`
	Values []json.RawMessage `json:"values"`
}

// jsmWalkCeiling bounds an "unlimited" walk so a server that ignores ?start=
// cannot loop forever (500 pages x 50 rows).
const jsmWalkCeiling = 500

// jsmWalk pages a JSM PagedDTO endpoint (?start=&limit=) until an empty page or
// a page shorter than the effective limit. It never trusts isLastPage, which
// the customers endpoint reports as true on every page (JSDCLOUD-8279), and it
// de-duplicates by key so rows shifting between pages (queues drift as tickets
// arrive) are counted once. It also stops when a page adds nothing new (every
// row already seen, or a byte-identical repeat of the previous page when no
// key func is given): that is a server ignoring ?start=, so the rest is
// unreachable. maxPages <= 0 means up to jsmWalkCeiling, and a caller asking
// for everything gets an error when either the ceiling or a stall stops the
// walk; with a page cap, truncated reports the walk stopped early instead.
func jsmWalk(ctx context.Context, c jsmGetter, path string, params map[string]string, maxPages int, key func(json.RawMessage) string) (items []json.RawMessage, truncated bool, err error) {
	seen := map[string]bool{}
	items = make([]json.RawMessage, 0)
	start := 0
	unlimited := maxPages <= 0
	if unlimited {
		maxPages = jsmWalkCeiling
	}
	var prev []byte
	stalled := func() ([]json.RawMessage, bool, error) {
		if unlimited {
			return items, true, fmt.Errorf("%s: the server repeated a page instead of advancing ?start=; results after %d rows are unreachable", path, len(items))
		}
		return items, true, nil
	}
	for page := 1; ; page++ {
		if page > maxPages {
			if unlimited {
				return items, true, fmt.Errorf("%s: stopped after %d pages (%d rows) to avoid an endless walk; results would be incomplete", path, jsmWalkCeiling, len(items))
			}
			return items, true, nil
		}
		q := map[string]string{}
		for k, v := range params {
			q[k] = v
		}
		q["start"] = strconv.Itoa(start)
		q["limit"] = strconv.Itoa(jsmMaxPageSize)
		raw, err := jsmGet(ctx, c, path, q)
		if err != nil {
			return items, false, err
		}
		var pg jsmPage
		if err := json.Unmarshal(raw, &pg); err != nil {
			return items, false, fmt.Errorf("decoding %s page: %w", path, err)
		}
		if key == nil && page > 1 && bytes.Equal(raw, prev) {
			return stalled()
		}
		prev = raw
		fresh := 0
		for _, v := range pg.Values {
			k := ""
			if key != nil {
				k = key(v)
			}
			if k != "" {
				if seen[k] {
					continue
				}
				seen[k] = true
			}
			fresh++
			items = append(items, v)
		}
		limit := jsmMaxPageSize
		if pg.Limit > 0 && pg.Limit < limit {
			limit = pg.Limit
		}
		// A short page is the end even if every row on it was already seen
		// (a row inserted at the head mid-walk shifts one back); only a full
		// page that adds nothing is a stall.
		if len(pg.Values) == 0 || len(pg.Values) < limit {
			return items, false, nil
		}
		if fresh == 0 {
			return stalled()
		}
		start += len(pg.Values)
	}
}

// jsonKey returns a key func reading one top-level string/number field.
func jsonKey(field string) func(json.RawMessage) string {
	return func(raw json.RawMessage) string {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return ""
		}
		var f flexString
		if json.Unmarshal(m[field], &f) != nil {
			return ""
		}
		return string(f)
	}
}

type jsmDuration struct {
	Millis   int64  `json:"millis"`
	Friendly string `json:"friendly"`
}

type jsmDate struct {
	ISO8601     string `json:"iso8601"`
	EpochMillis int64  `json:"epochMillis"`
}

type jsmSLACycle struct {
	Breached      bool        `json:"breached"`
	Paused        bool        `json:"paused"`
	GoalDuration  jsmDuration `json:"goalDuration"`
	ElapsedTime   jsmDuration `json:"elapsedTime"`
	RemainingTime jsmDuration `json:"remainingTime"`
	StopTime      *jsmDate    `json:"stopTime"`
	BreachTime    *jsmDate    `json:"breachTime"`
}

type jsmSLA struct {
	Name            string        `json:"name"`
	OngoingCycle    *jsmSLACycle  `json:"ongoingCycle"`
	CompletedCycles []jsmSLACycle `json:"completedCycles"`
}

// slaRisk is the most urgent ongoing SLA on one ticket.
type slaRisk struct {
	Metric      string `json:"sla"`
	Breached    bool   `json:"breached"`
	Paused      bool   `json:"paused"`
	RemainingMS int64  `json:"remaining_ms"`
	Remaining   string `json:"remaining"`
	BreachTime  string `json:"breach_time,omitempty"`
}

// mostUrgent picks the most urgent ongoing cycle using urgentBefore's order:
// breached first (paused or not), then running before paused, then least
// time left.
func mostUrgent(slas []jsmSLA) (slaRisk, bool) {
	var best slaRisk
	found := false
	for _, s := range slas {
		if r, ok := riskOf(s); ok && (!found || urgentBefore(r, best)) {
			best, found = r, true
		}
	}
	return best, found
}

// riskOf converts an SLA's ongoing cycle (false when none is running).
func riskOf(s jsmSLA) (slaRisk, bool) {
	c := s.OngoingCycle
	if c == nil {
		return slaRisk{}, false
	}
	// One breach rule for every command: flagged by JSM, or out of time.
	r := slaRisk{Metric: s.Name, Breached: c.Breached || c.RemainingTime.Millis < 0, Paused: c.Paused, RemainingMS: c.RemainingTime.Millis, Remaining: c.RemainingTime.Friendly}
	if c.BreachTime != nil {
		r.BreachTime = c.BreachTime.ISO8601
	}
	return r, true
}

// urgentBefore ranks breached cycles first (paused or not: a breach already
// happened), then running before paused, then least time left.
func urgentBefore(a, b slaRisk) bool {
	if a.Breached != b.Breached {
		return a.Breached
	}
	if a.Paused != b.Paused {
		return !a.Paused
	}
	return a.RemainingMS < b.RemainingMS
}

// fetchSLAs reads a request's SLA metrics.
func fetchSLAs(ctx context.Context, c jsmGetter, issueKey string) ([]jsmSLA, error) {
	seg, err := jsmPathSeg(issueKey)
	if err != nil {
		return nil, err
	}
	items, _, err := jsmWalk(ctx, c, "/request/"+seg+"/sla", nil, 0, jsonKey("id"))
	if err != nil {
		return nil, err
	}
	out := make([]jsmSLA, 0, len(items))
	for _, raw := range items {
		var s jsmSLA
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("decoding SLA for %s: %w", issueKey, err)
		}
		out = append(out, s)
	}
	return out, nil
}

type fetchFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
	err   error
}

// allFailed returns the first underlying error when every one of n lookups
// failed, so callers exit non-zero instead of reporting an empty, healthy
// result (an expired token fails every call the same way).
func allFailed(failures []fetchFailure, n int) error {
	if n == 0 || len(failures) < n {
		return nil
	}
	for _, f := range failures {
		if f.err != nil {
			return f.err
		}
	}
	return fmt.Errorf("all %d lookups failed: %s", n, failures[0].Error)
}

// allNotFound reports whether every failure is an HTTP 404.
func allNotFound(failures []fetchFailure) bool {
	for _, f := range failures {
		var apiErr *client.APIError
		if !errors.As(f.err, &apiErr) || apiErr.StatusCode != 404 {
			return false
		}
	}
	return len(failures) > 0
}

// fanOut runs fn over keys with bounded concurrency and keeps per-key errors.
func fanOut[T any](ctx context.Context, keys []string, workers int, fn func(context.Context, string) (T, error)) (map[string]T, []fetchFailure) {
	if workers < 1 {
		workers = 1
	}
	var mu sync.Mutex
	results := make(map[string]T, len(keys))
	failures := make([]fetchFailure, 0)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()
			v, err := fn(ctx, k)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fetchFailure{ID: k, Error: err.Error(), err: err})
				return
			}
			results[k] = v
		}(k)
	}
	wg.Wait()
	sort.Slice(failures, func(i, j int) bool { return failures[i].ID < failures[j].ID })
	return results, failures
}

// ---- Assets (CMDB) ----

type assetsPoster interface {
	PostQueryWithParams(ctx context.Context, path string, params map[string]string, body any) (json.RawMessage, int, error)
}

type assetsAttrValue struct {
	Value        any    `json:"value"`
	DisplayValue string `json:"displayValue"`
}

type assetsAttribute struct {
	ObjectTypeAttributeID flexString        `json:"objectTypeAttributeId"`
	ObjectAttributeValues []assetsAttrValue `json:"objectAttributeValues"`
}

type assetsObject struct {
	ID         flexString `json:"id"`
	ObjectKey  string     `json:"objectKey"`
	Label      string     `json:"label"`
	ObjectType struct {
		Name string `json:"name"`
	} `json:"objectType"`
	Attributes []assetsAttribute `json:"attributes"`
}

type assetsAQLPage struct {
	IsLast               bool           `json:"isLast"`
	Values               []assetsObject `json:"values"`
	ObjectTypeAttributes []struct {
		ID   flexString `json:"id"`
		Name string     `json:"name"`
	} `json:"objectTypeAttributes"`
}

// assetsAQL walks POST /object/aql until isLast, maxObjects objects (<= 0 means
// no cap), or a page with no new object ids (a server ignoring startAt).
// It returns the objects and an attribute-id -> name map from the responses.
func assetsAQL(ctx context.Context, c assetsPoster, ql string, maxObjects int) ([]assetsObject, map[string]string, bool, error) {
	const pageSize = 50
	objs := make([]assetsObject, 0)
	names := map[string]string{}
	seen := map[string]bool{}
	for startAt := 0; ; {
		// Always full pages: a smaller final page could hold only shifted,
		// already-seen rows and trip the stall check below.
		params := map[string]string{"startAt": strconv.Itoa(startAt), "maxResults": strconv.Itoa(pageSize), "includeAttributes": "true"}
		raw, _, err := c.PostQueryWithParams(ctx, assetsBase+"/object/aql", params, map[string]any{"qlQuery": ql})
		if err != nil {
			return objs, names, false, err
		}
		var pg assetsAQLPage
		if err := json.Unmarshal(raw, &pg); err != nil {
			return objs, names, false, fmt.Errorf("decoding AQL page: %w", err)
		}
		for _, a := range pg.ObjectTypeAttributes {
			names[string(a.ID)] = a.Name
		}
		fresh := 0
		for _, o := range pg.Values {
			if seen[string(o.ID)] {
				continue
			}
			seen[string(o.ID)] = true
			objs = append(objs, o)
			fresh++
		}
		if maxObjects > 0 && len(objs) >= maxObjects {
			more := !pg.IsLast || len(objs) > maxObjects
			return objs[:maxObjects], names, more, nil
		}
		// The last page ends the walk even if rows shifted back onto it; only
		// a non-last page that adds nothing is a server ignoring startAt.
		if pg.IsLast || len(pg.Values) == 0 {
			return objs, names, false, nil
		}
		if fresh == 0 {
			return objs, names, true, nil
		}
		startAt += len(pg.Values)
	}
}

// flattenObject maps attribute names to display values (multi-values joined),
// plus Key and Label so callers can match on either.
func flattenObject(o assetsObject, names map[string]string) map[string]string {
	out := map[string]string{"Key": o.ObjectKey, "Label": o.Label}
	for _, a := range o.Attributes {
		name := names[string(a.ObjectTypeAttributeID)]
		if name == "" {
			name = "attr:" + string(a.ObjectTypeAttributeID)
		}
		vals := make([]string, 0, len(a.ObjectAttributeValues))
		for _, v := range a.ObjectAttributeValues {
			s := v.DisplayValue
			if s == "" && v.Value != nil {
				s = fmt.Sprint(v.Value)
			}
			vals = append(vals, s)
		}
		out[name] = strings.Join(vals, ", ")
	}
	return out
}

// aqlQuote quotes a value for an AQL string literal.
func aqlQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// issueKeyLike reports whether s looks like a Jira or Assets key (ABC-123).
func issueKeyLike(s string) bool {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 || i == len(s)-1 {
		return false
	}
	if _, err := strconv.Atoi(s[i+1:]); err != nil {
		return false
	}
	for _, r := range s[:i] {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// jsmPathSeg validates and escapes one URL path segment taken from user input
// or a response. "..", "/" and "\\" would let the value walk to another
// endpoint (e.g. a public comment API instead of the internal-note one).
func jsmPathSeg(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\") {
		return "", fmt.Errorf("invalid id or key %q", s)
	}
	return url.PathEscape(s), nil
}

// termSafe drops control characters (ANSI escapes included) and Unicode
// format characters (bidi overrides) from API text before it reaches a human
// terminal; tabs survive.
func termSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || (!unicode.IsControl(r) && !unicode.Is(unicode.Cf, r)) {
			return r
		}
		return -1
	}, s)
}

func truncateText(s string, n int) string {
	s = termSafe(s)
	r := []rune(s)
	if n < 1 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

type jsmUser struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

type jsmOrg struct {
	ID   flexString `json:"id"`
	Name string     `json:"name"`
}
