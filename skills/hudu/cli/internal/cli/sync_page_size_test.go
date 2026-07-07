// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression tests for the Hudu pagination defects:
//   - #153: the IPAM-family list endpoints reject the `page_size` filter.
//   - #158: those same endpoints are in fact NON-paginated — they reject
//     `page` too — so sync must send neither param and fetch them once; and
//     every other resource must advance by ?page=N (cursorType "page") instead
//     of truncating at the first page.
//   - #159: /matchers requires an integration_id, so flat sync must skip it
//     unless the caller supplies one.
//   - #167: /relations 500s when walked at the global page_size=100, so sync
//     must page it at 25 (resourcePageSize override), not the global default.
//   - #169: sync --exclude drops named resources from the effective set.
// A reprint that drops this file is caught by skills/hudu/handfixes.json — do
// not remove without porting the fixes.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hudu-pp-cli/internal/store"
)

func TestResourceRejectsPageSize(t *testing.T) {
	rejects := []string{"ip-addresses", "networks", "vlans", "vlan-zones", "rack-storage-items"}
	for _, r := range rejects {
		if !resourceRejectsPageSize(r) {
			t.Errorf("resourceRejectsPageSize(%q) = false, want true (non-paginated; 400s on page and page_size)", r)
		}
	}
	for _, r := range []string{"companies", "articles", "assets", "websites", "users"} {
		if resourceRejectsPageSize(r) {
			t.Errorf("resourceRejectsPageSize(%q) = true, want false (endpoint paginates normally)", r)
		}
	}
}

// recordingClient implements the syncResource client interface, records every
// params map it is handed, and serves canned array pages indexed by `page`
// (absent `page` => page 1; index past the end => empty page).
type recordingClient struct {
	seen  []map[string]string
	pages [][]byte
}

func (r *recordingClient) RateLimit() float64 { return 0 }

func (r *recordingClient) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	cp := map[string]string{}
	for k, v := range params {
		cp[k] = v
	}
	r.seen = append(r.seen, cp)
	page := 1
	if p, ok := params["page"]; ok {
		if n, err := strconv.Atoi(p); err == nil && n >= 1 {
			page = n
		}
	}
	if page-1 < len(r.pages) {
		return json.RawMessage(r.pages[page-1]), nil
	}
	return json.RawMessage(`[]`), nil
}

// TestSyncResource_NonPaginatedEndpoint_FetchesOnceNoPageParam pins the #158
// fix: the IPAM-family endpoints are non-paginated, so sync sends neither
// page_size nor page and makes exactly one request even at the unlimited
// (--max-pages 0) production default. The pre-#158 code sent page=1,2,... and
// 400'd on every request after the first.
func TestSyncResource_NonPaginatedEndpoint_FetchesOnceNoPageParam(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	c := &recordingClient{pages: [][]byte{
		[]byte(`[{"id":"a1"},{"id":"a2"},{"id":"a3"}]`),
	}}

	res := syncResource(context.Background(), c, db, "ip-addresses", "", true, 0, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	// Exactly one request — no page walk (would loop/400 against a real tenant).
	if len(c.seen) != 1 {
		t.Fatalf("made %d requests, want exactly 1 (non-paginated endpoint)", len(c.seen))
	}
	// Neither pagination param may be sent — both 400 on these endpoints.
	if v, bad := c.seen[0]["page"]; bad {
		t.Errorf("request carried page=%q to a non-paginated endpoint", v)
	}
	if v, bad := c.seen[0]["page_size"]; bad {
		t.Errorf("request carried page_size=%q to a non-paginated endpoint", v)
	}
	// The single response is the whole collection — all rows must land.
	if n := countRows(t, db, "ip-addresses"); n != 3 {
		t.Errorf("stored %d rows, want 3 (single response is the full collection)", n)
	}
}

// TestSyncResource_PageType_AdvancesUntilShortPage pins the #158 cursorType
// fix: cursorType "page" drives the page-int fallback so full pages advance
// page=2,3,... until a short page ends the walk. Pre-#158 (cursorType "") the
// fallback was dead and every resource truncated at the first 100 rows.
func TestSyncResource_PageType_AdvancesUntilShortPage(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	limit := determinePaginationDefaults().limit // 100
	c := &recordingClient{pages: [][]byte{
		makePage(1, limit),       // page 1: full
		makePage(limit+1, limit), // page 2: full
		makePage(2*limit+1, 5),   // page 3: short -> stop
	}}

	res := syncResource(context.Background(), c, db, "companies", "", true, 0, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	if got := len(c.seen); got != 3 {
		t.Fatalf("made %d requests, want 3 (page-int walk across two full pages)", got)
	}
	// First request omits page (cursor == ""); then page=2, page=3.
	if v, sent := c.seen[0]["page"]; sent {
		t.Errorf("first request sent page=%q, want no page param", v)
	}
	if c.seen[1]["page"] != "2" || c.seen[2]["page"] != "3" {
		t.Errorf("page sequence = %q,%q, want 2,3", c.seen[1]["page"], c.seen[2]["page"])
	}
	if n := countRows(t, db, "companies"); n != 2*limit+5 {
		t.Errorf("stored %d rows, want %d (no truncation at page 1)", n, 2*limit+5)
	}
}

// staticClient ignores `page` and returns the same full page on every call —
// the page-ignoring shape the fingerprint guard must terminate.
type staticClient struct {
	calls int
	body  []byte
}

func (s *staticClient) RateLimit() float64 { return 0 }

func (s *staticClient) Get(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	s.calls++
	return json.RawMessage(s.body), nil
}

// TestSyncResource_PageIgnoringEndpoint_FingerprintTerminates pins the loop
// backstop for the now-active page-int walk (#158): an endpoint that returns a
// full page (>= limit) and ignores `page` must NOT loop forever at the
// unlimited (--max-pages 0) production default — the fingerprint dup-guard
// stops it on the first repeated page.
func TestSyncResource_PageIgnoringEndpoint_FingerprintTerminates(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	limit := determinePaginationDefaults().limit // 100
	c := &staticClient{body: makePage(1, limit)} // full page, same every call
	res := syncResource(context.Background(), c, db, "companies", "", true, 0, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	// Page 1 advances; page 2 is byte-identical -> fingerprint match -> stop.
	// If the guard were missing this would loop to the page ceiling (or forever).
	if c.calls != 2 {
		t.Errorf("made %d requests, want 2 (fingerprint guard must break on the duplicate page)", c.calls)
	}
	if n := countRows(t, db, "companies"); n != limit {
		t.Errorf("stored %d distinct rows, want %d", n, limit)
	}
}

// TestSyncResource_Matchers_SkippedWithoutIntegrationID pins the #159 fix: a
// flat sync must not call /matchers without an integration_id (Hudu 500s), so
// the resource is skipped with a warning and zero API requests.
func TestSyncResource_Matchers_SkippedWithoutIntegrationID(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	c := &recordingClient{pages: [][]byte{[]byte(`[{"id":"m1"}]`)}}
	res := syncResource(context.Background(), c, db, "matchers", "", true, 0, false, nil, nil)

	if len(c.seen) != 0 {
		t.Errorf("matchers made %d API requests, want 0 (must skip without integration_id)", len(c.seen))
	}
	if res.Warn == nil {
		t.Error("expected a skip warning for matchers without integration_id")
	}
	if res.Err != nil {
		t.Errorf("skip must not surface an error: %v", res.Err)
	}
}

// TestSyncResource_Matchers_SyncedWithIntegrationID pins the escape hatch: an
// explicit integration_id (via --resource-param) lets matchers sync, and the
// param is forwarded to the request.
func TestSyncResource_Matchers_SyncedWithIntegrationID(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	up, err := parseSyncUserParams(nil, []string{"matchers:integration_id=42"}, nil)
	if err != nil {
		t.Fatalf("parseSyncUserParams: %v", err)
	}
	c := &recordingClient{pages: [][]byte{[]byte(`[{"id":"m1"}]`)}}
	res := syncResource(context.Background(), c, db, "matchers", "", true, 0, false, up, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	if len(c.seen) == 0 {
		t.Fatal("matchers must be fetched when integration_id is supplied")
	}
	if got := c.seen[0]["integration_id"]; got != "42" {
		t.Errorf("integration_id forwarded = %q, want 42 (params: %v)", got, c.seen[0])
	}
	if n := countRows(t, db, "matchers"); n != 1 {
		t.Errorf("stored %d matcher rows, want 1", n)
	}
}

// TestResourcePageSize_RelationsOverridesGlobalTo25 pins the #167 fix: relations
// pages at 25 (Hudu's default; the size n8n-nodes-hudu uses to retrieve every
// relation), while every other resource keeps the global default (0 => 100).
func TestResourcePageSize_RelationsOverridesGlobalTo25(t *testing.T) {
	if got := resourcePageSize("relations"); got != 25 {
		t.Errorf("resourcePageSize(relations) = %d, want 25 (#167: /relations 500s at the global 100)", got)
	}
	for _, r := range []string{"companies", "assets", "procedure-tasks", "articles", "users"} {
		if got := resourcePageSize(r); got != 0 {
			t.Errorf("resourcePageSize(%q) = %d, want 0 (use the global default)", r, got)
		}
	}
}

// TestSyncResource_Relations_PagesAt25NotGlobal100 pins the #167 behavioral fix:
// sync must send page_size=25 on every /relations request (never the 100 that
// 500s beyond page 1) and still walk the full collection via ?page=N.
func TestSyncResource_Relations_PagesAt25NotGlobal100(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	rel := resourcePageSize("relations") // 25
	c := &recordingClient{pages: [][]byte{
		makePage(1, rel),     // page 1: full at 25
		makePage(rel+1, rel), // page 2: full at 25
		makePage(2*rel+1, 4), // page 3: short -> stop
	}}

	res := syncResource(context.Background(), c, db, "relations", "", true, 0, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	// Every request must carry page_size=25 — the global 100 is what 500s.
	for i, p := range c.seen {
		if p["page_size"] != strconv.Itoa(rel) {
			t.Errorf("request %d sent page_size=%q, want %q (#167)", i, p["page_size"], strconv.Itoa(rel))
		}
	}
	// The walk advances at the 25-row threshold and fetches the whole collection.
	if got := len(c.seen); got != 3 {
		t.Fatalf("made %d requests, want 3 (page-int walk at size 25)", got)
	}
	if c.seen[1]["page"] != "2" || c.seen[2]["page"] != "3" {
		t.Errorf("page sequence = %q,%q, want 2,3", c.seen[1]["page"], c.seen[2]["page"])
	}
	if n := countRows(t, db, "relations"); n != 2*rel+4 {
		t.Errorf("stored %d rows, want %d (full collection, no truncation)", n, 2*rel+4)
	}
}

// TestExcludeResources pins the #169 --exclude flag: valid names are removed
// (order preserved) and an unknown name fails loudly instead of silently
// excluding nothing.
func TestExcludeResources(t *testing.T) {
	got := filterExcludedResources(
		[]string{"companies", "public-photos", "procedures", "procedure-tasks", "assets"},
		[]string{"public-photos", "procedures", "procedure-tasks"},
	)
	if want := "companies,assets"; strings.Join(got, ",") != want {
		t.Errorf("filterExcludedResources = %v, want %q", got, want)
	}

	known := knownSyncResourceNames()
	if err := validateExcludeNames([]string{"companies", "notathing"}, known); err == nil {
		t.Error("validateExcludeNames accepted an unknown resource; want an error")
	}
	if err := validateExcludeNames([]string{"public-photos", "procedures"}, known); err != nil {
		t.Errorf("validateExcludeNames rejected known resources: %v", err)
	}
}

func openSyncTestDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return db
}

func countRows(t *testing.T, db *store.Store, resource string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, resource,
	).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// makePage builds a JSON array of n objects with sequential integer ids
// starting at start — used to synthesize full/short pages for the page walk.
func makePage(start, n int) []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d}`, start+i)
	}
	b.WriteByte(']')
	return []byte(b.String())
}
