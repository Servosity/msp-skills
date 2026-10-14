// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Meraki paginates every list endpoint with an RFC 5988 Link response header:
// rel=next carries the startingAfter token for the following page, and the
// body is a bare JSON array with no cursor in it. The generated sync and --all
// loops only read cursors from JSON bodies, so they stopped after page one.
//
// A client hook wraps the HTTP transport and hands each live response's Link
// header to a per-request sink carried on the request context. Sync and
// paginatedGet open a sink before each page fetch and advance with
// linkAdvance. The fleet commands use merakiGetAll directly.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/platform"
)

const merakiCursorParam = "startingAfter"

type linkSinkKey struct{}

// linkSink records the Link header of the last live response for one request.
// seen stays false when the response came from the local HTTP cache, which
// does not keep headers; callers that need every page must bypass the cache.
type linkSink struct {
	mu         sync.Mutex
	seen       bool
	link       string
	path       string // request path, identifies the walk
	sentCursor string // startingAfter this request sent ("" = first page)
}

// walkCursors remembers every Link cursor each in-flight walk (keyed by
// request path, which includes the org or network ID) has followed, so an
// A->B->A cycle stops instead of looping forever. A request without
// startingAfter starts a new walk and resets that path's set.
var walkCursors = struct {
	sync.Mutex
	byPath map[string]map[string]bool
}{byPath: map[string]map[string]bool{}}

func withLinkSink(ctx context.Context) (context.Context, *linkSink) {
	s := &linkSink{}
	return context.WithValue(ctx, linkSinkKey{}, s), s
}

func (s *linkSink) record(h http.Header) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.seen = true
	s.link = h.Get("Link")
	s.mu.Unlock()
}

func (s *linkSink) recordRequest(u *url.URL) {
	if s == nil || u == nil {
		return
	}
	s.mu.Lock()
	s.path, s.sentCursor = u.Path, u.Query().Get(merakiCursorParam)
	s.mu.Unlock()
}

// cycled reports whether tok was already followed in this walk, recording it
// otherwise, and returns the cursor this page was fetched with.
func (s *linkSink) cycled(tok string) (sent string, repeated bool) {
	s.mu.Lock()
	path, sent := s.path, s.sentCursor
	s.mu.Unlock()
	walkCursors.Lock()
	defer walkCursors.Unlock()
	seen := walkCursors.byPath[path]
	if sent == "" || seen == nil {
		seen = map[string]bool{}
		walkCursors.byPath[path] = seen
	}
	if seen[tok] {
		delete(walkCursors.byPath, path)
		return sent, true
	}
	seen[tok] = true
	return sent, false
}

// endWalk forgets the cursors of a walk that reached its last page.
func (s *linkSink) endWalk() {
	s.mu.Lock()
	path := s.path
	s.mu.Unlock()
	walkCursors.Lock()
	delete(walkCursors.byPath, path)
	walkCursors.Unlock()
}

// next returns the startingAfter token of the rel=next link, or "" at the end.
func (s *linkSink) next() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return linkNextToken(s.link, merakiCursorParam)
}

func (s *linkSink) sawResponse() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen
}

// linkNextToken extracts the cursorParam query value from the rel=next entry
// of a Link header such as
// `<https://api.meraki.com/api/v1/organizations?startingAfter=X>; rel=next`.
func linkNextToken(header, cursorParam string) string {
	for _, part := range strings.Split(header, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		isNext := false
		for _, attr := range segs[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(attr), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "rel") {
				for _, rel := range strings.Fields(strings.Trim(strings.TrimSpace(v), `"`)) {
					if strings.EqualFold(rel, "next") {
						isNext = true
					}
				}
			}
		}
		if !isNext {
			continue
		}
		raw := strings.Trim(strings.TrimSpace(segs[0]), "<>")
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if tok := u.Query().Get(cursorParam); tok != "" {
			return tok
		}
	}
	return ""
}

type linkCaptureTransport struct{ base http.RoundTripper }

func (t linkCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil {
		if s, ok := req.Context().Value(linkSinkKey{}).(*linkSink); ok {
			s.recordRequest(req.URL)
			s.record(resp.Header)
		}
	}
	return resp, err
}

func init() {
	registerClientHook(func(c *client.Client) error {
		if c == nil || c.HTTPClient == nil {
			return nil
		}
		if _, ok := c.HTTPClient.Transport.(linkCaptureTransport); ok {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		c.HTTPClient.Transport = linkCaptureTransport{base: base}
		return nil
	})
}

// merakiLinkPagination turns the generator's perPage-only default (which it
// classifies as offset paging with no cursor) into startingAfter cursor paging
// so the sync loop sends the token linkAdvance hands it.
func merakiLinkPagination(p paginationDefaults) paginationDefaults {
	if p.limitParam == "perPage" && p.cursorParam == "" {
		p.cursorParam = merakiCursorParam
		p.cursorType = "cursor"
	}
	return p
}

// linkAdvance supplies the next cursor from the Link header when the body had
// none. A body cursor, when an endpoint has one, still wins.
func linkAdvance(s *linkSink, p paginationDefaults, nextCursor string, hasMore bool) (string, bool) {
	if nextCursor != "" || p.cursorParam != merakiCursorParam {
		return nextCursor, hasMore
	}
	tok := s.next()
	if tok == "" {
		s.endWalk()
		return nextCursor, hasMore
	}
	if sent, repeated := s.cycled(tok); repeated {
		// Hand back the cursor this page was fetched with: it equals the
		// loop's lastNextCursor, which trips the generated stuck-cursor guard
		// and ends the walk as incomplete (no reconcile).
		return sent, true
	}
	return tok, true
}

// linkProvenEnd reports that a live Meraki page carried no rel=next link, so
// the walk reached the true end even when that last page happened to be full.
// Sync uses it to keep reconcile running instead of marking the partition
// "cursor unavailable".
func linkProvenEnd(s *linkSink, p paginationDefaults) bool {
	return p.cursorParam == merakiCursorParam && s.sawResponse() && s.next() == ""
}

// envelopeArrayField names the array an object page carries its items in
// (clients/search returns {"clientId":..., "records":[...]} and pages records).
func envelopeArrayField(obj map[string]json.RawMessage) string {
	for _, k := range []string{"records", "items", "data"} {
		if v, ok := obj[k]; ok && len(v) > 0 && v[0] == '[' {
			return k
		}
	}
	found := ""
	for k, v := range obj {
		if len(v) > 0 && v[0] == '[' {
			if found != "" {
				return ""
			}
			found = k
		}
	}
	return found
}

// paginatedGetLinkNext is the --all hook: when a page carried a rel=next
// link, it stores the token in params and reports true so the caller fetches
// the next page. seen is the caller's cursor set; a cursor the API already
// sent stops the walk and marks the output truncated rather than complete.
func paginatedGetLinkNext(ctx context.Context, s *linkSink, params map[string]string, seen map[string]struct{}) bool {
	if !s.sawResponse() {
		// The page never reached the transport (a client shape that reads the
		// cache), so its Link header is unknown: say so instead of "complete".
		platform.MarkContextTruncated(ctx, platform.TruncationReason{Kind: "pagination_link_unobserved", Configured: "live_page", Observed: "cached_page", MoreAvailable: true})
		return false
	}
	tok := s.next()
	if tok == "" {
		return false
	}
	if _, dup := seen[tok]; dup || params[merakiCursorParam] == tok {
		platform.MarkContextTruncated(ctx, platform.TruncationReason{Kind: "pagination_cursor_repeated", Configured: "unique_cursor", Observed: "repeated_cursor", MoreAvailable: true})
		if humanFriendly {
			fmt.Fprintf(os.Stderr, "warning: --all received the same pagination cursor twice; returning fetched pages only.\n")
		} else {
			fmt.Fprintf(os.Stderr, `{"event":"truncated","reason":"pagination_cursor_repeated","message":"--all received the same Link cursor twice; returning fetched pages only"}`+"\n")
		}
		return false
	}
	seen[tok] = struct{}{}
	params[merakiCursorParam] = tok
	return true
}

// noCacheGetter adapts a pageGetter to the GetWithHeaders shape so the
// generated response-path wrapper also reads every page past the cache.
type noCacheGetter struct{ pg pageGetter }

func (n noCacheGetter) GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error) {
	return n.pg.GetWithHeadersNoCache(ctx, path, params, headers)
}

func (n noCacheGetter) IsDryRun() bool {
	d, ok := n.pg.(interface{ IsDryRun() bool })
	return ok && d.IsDryRun()
}

// pageGetter is the cache-bypassing read every Link walk needs: a cached
// response has no headers, so it would look like the last page.
type pageGetter interface {
	GetWithHeadersNoCache(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}

// paginatedGetPage fetches one page for --all, bypassing the response cache
// when the client supports it so the Link header is always observed.
func paginatedGetPage(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, path string, params, headers map[string]string) (json.RawMessage, error) {
	if pg, ok := c.(pageGetter); ok {
		return pg.GetWithHeadersNoCache(ctx, path, params, headers)
	}
	if rp, ok := c.(responsePathPaginatedClient); ok {
		if pg, ok := rp.client.(pageGetter); ok {
			return responsePathPaginatedClient{client: noCacheGetter{pg}, responsePath: rp.responsePath}.GetWithHeaders(ctx, path, params, headers)
		}
	}
	return c.GetWithHeaders(ctx, path, params, headers)
}

// pageResult is one fully walked list read with the provenance the official
// Meraki MCP lacks (its issues #2 and #3).
type pageResult struct {
	Items         []json.RawMessage `json:"-"`
	PagesFetched  int               `json:"pages_fetched"`
	ItemsReturned int               `json:"items_returned"`
	Complete      bool              `json:"complete"`
	NextCursor    string            `json:"next_cursor,omitempty"`
	LimitReason   string            `json:"limit_reason,omitempty"`
}

// merakiGetAll walks every Link page of a bare-array list endpoint. maxPages
// <= 0 means unlimited. When the cap stops the walk, Complete is false and
// NextCursor holds the token that resumes it.
func merakiGetAll(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) (pageResult, error) {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v) // merakiGetAllValues drops empty values
	}
	return merakiGetAllValues(ctx, c, path, q, maxPages)
}

// merakiGetAllValues is merakiGetAll for multi-value query params
// (Meraki arrays repeat the bracketed key: statuses[]=a&statuses[]=b).
func merakiGetAllValues(ctx context.Context, c *client.Client, path string, params url.Values, maxPages int) (pageResult, error) {
	q := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			if v != "" {
				q.Add(k, v)
			}
		}
	}
	res := pageResult{Items: make([]json.RawMessage, 0)}
	seen := map[string]bool{}
	var envelope map[string]json.RawMessage
	var envelopeField string
	merged := make([]json.RawMessage, 0)
	// done builds a paged envelope once, from every page's array, on exit.
	done := func(err error) (pageResult, error) {
		if envelope != nil {
			b, _ := json.Marshal(merged)
			envelope[envelopeField] = b
			full, _ := json.Marshal(envelope)
			res.Items = []json.RawMessage{full}
			res.ItemsReturned = len(merged)
		}
		return res, err
	}
	for {
		pageCtx, sink := withLinkSink(ctx)
		data, err := c.GetWithHeadersNoCacheValues(pageCtx, path, q, nil)
		if err != nil {
			return done(err)
		}
		res.PagesFetched++
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			if c.DryRun {
				res.Complete = true
				return res, nil
			}
			var obj map[string]json.RawMessage
			field := ""
			if json.Unmarshal(data, &obj) == nil {
				field = envelopeArrayField(obj)
			}
			if field == "" {
				// An overview object without a paged array is one record.
				res, _ = done(nil) // flush envelope pages read before it, if any
				res.Items = append(res.Items, data)
				res.ItemsReturned = len(res.Items)
				res.Complete = sink.next() == ""
				return res, nil
			}
			// Paged envelope: merge every page's array into the first envelope.
			var page []json.RawMessage
			_ = json.Unmarshal(obj[field], &page)
			if envelope == nil {
				envelope, envelopeField = obj, field
			}
			merged = append(merged, page...)
		} else {
			res.Items = append(res.Items, items...)
			res.ItemsReturned = len(res.Items)
		}
		tok := sink.next()
		if tok == "" {
			res.Complete = true
			return done(nil)
		}
		if seen[tok] {
			res.LimitReason = "repeated_cursor"
			res.NextCursor = tok
			return done(fmt.Errorf("GET %s: the API repeated pagination cursor %q after %d pages; refusing to return a partial list", path, tok, res.PagesFetched))
		}
		seen[tok] = true
		if maxPages > 0 && res.PagesFetched >= maxPages {
			res.LimitReason = fmt.Sprintf("max_pages=%d", maxPages)
			res.NextCursor = tok
			return done(nil)
		}
		q.Set(merakiCursorParam, tok)
	}
}

// WithMerakiLinkNext is hand-wired (handfixes.json: meraki-mcp-execute-link-next)
// for the MCP execute tool, which sends one GET outside this package. It
// returns the context to send that GET with and a func reporting the rel=next
// startingAfter token the response's Link header named ("" = last page).
func WithMerakiLinkNext(ctx context.Context) (context.Context, func() string) {
	linkCtx, s := withLinkSink(ctx)
	return linkCtx, s.next
}

// AnnotateMerakiNextPage marks an MCP execute result as one page of several
// when the Link header named a next page, so an agent sees has_more and the
// cursor to resume with instead of a count that silently stops at page one.
func AnnotateMerakiNextPage(result, next string) string {
	if next == "" {
		return result
	}
	var obj map[string]any
	if json.Unmarshal([]byte(result), &obj) != nil || obj == nil {
		obj = map[string]any{"result": result}
	}
	obj["has_more"] = true
	obj["next_starting_after"] = next
	obj["pagination_note"] = "This is one page; Meraki has more (Link rel=next). Call again with params.startingAfter set to next_starting_after to read the next page, and raise params.perPage within the endpoint's range to read more per call."
	b, err := json.Marshal(obj)
	if err != nil {
		return result
	}
	return string(b)
}
