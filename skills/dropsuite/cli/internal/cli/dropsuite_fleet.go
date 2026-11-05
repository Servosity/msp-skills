// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared helpers for the Dropsuite fleet commands (health,
// coverage, billing, evidence, hygiene, locate).

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
)

// orgPageSize is the vendor's fixed page size on organization-scoped lists
// (S1: "Each page consists of max 25 ...").
const orgPageSize = 25

// decodeList accepts a bare JSON array or an envelope ({data:[...]} on
// /users/{id}/* routes, {result_set:[...]} on connection_failures). next is
// the envelope's pagination.next_page (0 when absent).
// decodeList decodes an inventory: a bare array or a list envelope. A lone
// object is an error here (an inventory route that answers {} is malformed);
// routes documented to return one object use singleObject or the
// pagedObjectOrList mode.
func decodeList(data json.RawMessage) (items []map[string]any, next int, err error) {
	return decodeListOpt(data, false)
}

func decodeListOpt(data json.RawMessage, allowSingle bool) (items []map[string]any, next int, err error) {
	trimmed := bytes.TrimSpace(data)
	items = make([]map[string]any, 0)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		// An empty list is [] (or an envelope with []); an empty or null
		// body is not evidence that nothing exists.
		return nil, 0, fmt.Errorf("empty response body")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, 0, err
	}
	switch t := v.(type) {
	case []any:
		items, err := mapsOf(t)
		return items, 0, err
	case map[string]any:
		for _, key := range []string{"data", "result_set", "results", "users"} {
			if arr, ok := t[key].([]any); ok {
				if items, err = mapsOf(arr); err != nil {
					return nil, 0, err
				}
				if p, ok := t["pagination"].(map[string]any); ok {
					next = int(num(p, "next_page"))
				}
				return items, next, nil
			}
		}
		// A recognized envelope without its array is malformed, not a record.
		_, paged := t["pagination"]
		for _, key := range []string{"data", "result_set", "results"} {
			if _, ok := t[key]; ok {
				paged = true
			}
		}
		if paged {
			return nil, 0, fmt.Errorf("list envelope has no array of items")
		}
		if !allowSingle {
			return nil, 0, fmt.Errorf("expected a list, got a single object")
		}
		return []map[string]any{t}, 0, nil
	default:
		return nil, 0, fmt.Errorf("unexpected JSON %T", v)
	}
}

// singleObject decodes a response that must hold exactly one object (an
// organization detail, a sign-in link).
func singleObject(data json.RawMessage) (map[string]any, error) {
	items, _, err := decodeListOpt(data, true)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("expected one object, got %d", len(items))
	}
	return items[0], nil
}

// ssoLinkFrom extracts a non-empty redirect_url from a sign-in link response.
func ssoLinkFrom(data json.RawMessage) (string, error) {
	m, err := singleObject(data)
	if err != nil {
		return "", err
	}
	link := str(m, "redirect_url")
	if link == "" {
		return "", fmt.Errorf("response has no redirect_url")
	}
	return link, nil
}

// mapsOf returns the array's objects; any other element (null, a number)
// makes the read an error rather than a silently shorter list.
func mapsOf(arr []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("list item %d is %T, not an object", i, item)
		}
		out = append(out, m)
	}
	return out, nil
}

// listMode picks the paging contract of a route.
type listMode int

const (
	pagedSingle       listMode = iota // one request, no paging params
	pagedObjectOrList                 // one request; a lone object is one item (S1: /accounts/invites)
	pagedByPage                       // page=N, 25 per page, stop on a short page
	pagedEnvelope                     // page=N&per_page=100, follow pagination.next_page
)

// fetchList walks a list route. It stops early when a page repeats rows it
// already returned, so a server that ignores ?page cannot loop forever.
func fetchList(ctx context.Context, c *client.Client, path string, params map[string]string, mode listMode, maxPages int) ([]map[string]any, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	if cliutil.IsDogfoodEnv() {
		maxPages = 1
	}
	all := make([]map[string]any, 0)
	seen := map[string]bool{}
	page := 1
	for i := 0; i < maxPages; i++ {
		q := map[string]string{}
		for k, v := range params {
			q[k] = v
		}
		switch mode {
		case pagedByPage:
			q["page"] = strconv.Itoa(page)
		case pagedEnvelope:
			q["page"] = strconv.Itoa(page)
			q["per_page"] = "100"
		}
		data, err := c.Get(ctx, path, q)
		if err != nil {
			return all, err
		}
		items, next, err := decodeListOpt(data, mode == pagedObjectOrList)
		if err != nil {
			return all, fmt.Errorf("parsing %s: %w", path, err)
		}
		added := 0
		for _, m := range items {
			b, _ := json.Marshal(m)
			key := string(b)
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, m)
			added++
		}
		if mode == pagedSingle || mode == pagedObjectOrList {
			return all, nil
		}
		if i > 0 && len(items) > 0 && added == 0 {
			// A page with nothing new means the API ignored the page number
			// (or repeated next_page); what was read cannot be shown to be
			// the whole list.
			return all, &truncatedError{Path: path, Pages: i + 1, Repeated: true}
		}
		switch mode {
		case pagedByPage:
			if len(items) < orgPageSize {
				return all, nil
			}
		case pagedEnvelope:
			if next <= 0 {
				return all, nil // no next page
			}
			if next <= page {
				// A pointer back to a page already read is a cycle.
				return all, &truncatedError{Path: path, Pages: i + 1, Repeated: true}
			}
			// A forward pointer is followed even past an empty page.
		}
		if mode == pagedEnvelope {
			page = next
		} else {
			page++
		}
	}
	return all, &truncatedError{Path: path, Pages: maxPages}
}

// truncatedError reports a list read that could not reach its end: pages
// left when --max-pages ran out, or the API repeating a page. Callers keep
// the partial items but must not present them as complete.
type truncatedError struct {
	Path     string
	Pages    int
	Repeated bool
}

func (e *truncatedError) Error() string {
	if e.Repeated {
		return fmt.Sprintf("%s: page %d repeated an earlier page, so the list may be incomplete", e.Path, e.Pages)
	}
	return fmt.Sprintf("%s: stopped after %d page(s) with more remaining; raise --max-pages", e.Path, e.Pages)
}

// ---- field accessors (tolerant of number-or-string JSON) -----------------

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func num(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return int64(f)
		}
	case float64:
		return int64(v)
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return i
		}
	case bool:
		if v {
			return 1
		}
	}
	return 0
}

func boolv(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}

// timeField parses a vendor timestamp: RFC3339 (2023-05-11T06:43:27.040Z) or
// the "2023/05/22 07:43" shape used by invites.
func timeField(m map[string]any, key string) (time.Time, bool) {
	s := strings.TrimSpace(str(m, key))
	if s == "" || s == "null" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006/01/02 15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// hasErrors reports a non-empty vendor "errors" value ({} and "" are empty).
func hasErrors(m map[string]any) (bool, string) {
	switch v := m["errors"].(type) {
	case nil:
		return false, ""
	case string:
		return strings.TrimSpace(v) != "", v
	case map[string]any:
		if len(v) == 0 {
			return false, ""
		}
		b, _ := json.Marshal(v)
		return true, string(b)
	case []any:
		if len(v) == 0 {
			return false, ""
		}
		b, _ := json.Marshal(v)
		return true, string(b)
	}
	return false, ""
}

func parseDurationFlag(name, value, def string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		value = def
	}
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil || d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a positive duration (examples: 24h, 48h, 7d)", name, value))
	}
	return d, nil
}

func ageHours(t time.Time, now time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	h := now.Sub(t).Hours()
	return float64(int64(h*10)) / 10
}

// errText is an error message without the trailing newline some API errors carry.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// fmtDuration renders whole days as "90d" and anything else as Go does.
func fmtDuration(d time.Duration) string {
	if d > 0 && d%(24*time.Hour) == 0 {
		return strconv.FormatInt(int64(d/(24*time.Hour)), 10) + "d"
	}
	return d.String()
}

// defaultSweepTimeout bounds a whole fleet run. Each request still uses
// --timeout; a fleet of organizations needs far more than one request's budget.
const defaultSweepTimeout = 15 * time.Minute

func sweepCtx(cmd *cobra.Command, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(cmd.Context())
	}
	return context.WithTimeout(cmd.Context(), d)
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
