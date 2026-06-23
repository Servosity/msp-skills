// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression test for issue #153: several Hudu IPAM-family list
// endpoints reject the `page_size` filter with HTTP 400 ("page_size is not a
// valid filter parameter."). The connector must page them by `page` alone and
// drain every page. Guards resourceRejectsPageSize, pageFingerprint, and the
// page-until-empty sync path. A reprint that drops this file is caught by
// skills/hudu/handfixes.json — do not remove without porting the fix.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"

	"hudu-pp-cli/internal/store"
)

func TestResourceRejectsPageSize(t *testing.T) {
	rejects := []string{"ip-addresses", "networks", "vlans", "vlan-zones", "rack-storage-items"}
	for _, r := range rejects {
		if !resourceRejectsPageSize(r) {
			t.Errorf("resourceRejectsPageSize(%q) = false, want true (endpoint 400s on page_size)", r)
		}
	}
	for _, r := range []string{"companies", "articles", "assets", "websites", "users"} {
		if resourceRejectsPageSize(r) {
			t.Errorf("resourceRejectsPageSize(%q) = true, want false (endpoint accepts page_size)", r)
		}
	}
}

func TestPageFingerprint(t *testing.T) {
	empty := []json.RawMessage{}
	if got := pageFingerprint(empty); got != "" {
		t.Errorf("pageFingerprint(empty) = %q, want \"\"", got)
	}
	a := []json.RawMessage{json.RawMessage(`{"id":1}`), json.RawMessage(`{"id":2}`)}
	b := []json.RawMessage{json.RawMessage(`{"id":3}`), json.RawMessage(`{"id":4}`)}
	if pageFingerprint(a) == pageFingerprint(b) {
		t.Error("pageFingerprint: distinct pages must not collide")
	}
	if pageFingerprint(a) != pageFingerprint(a) {
		t.Error("pageFingerprint: identical pages must match (the page-ignoring dup guard)")
	}
}

// recordingClient implements the syncResource client interface, records every
// params map it is handed, and serves canned array pages indexed by `page`.
type recordingClient struct {
	seen  []map[string]string
	pages [][]byte // pages[i] is the body for page i+1; absent index => empty page
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

func TestSyncResource_PageSizeRejectingEndpoint_PagesByPageOnly(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	c := &recordingClient{pages: [][]byte{
		[]byte(`[{"id":"a1"},{"id":"a2"}]`),
		[]byte(`[{"id":"a3"},{"id":"a4"}]`),
		// page 3+ => empty (recordingClient default) => end of data
	}}

	res := syncResource(context.Background(), c, db, "ip-addresses", "", true, 10, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}

	// (1) page_size must NEVER be sent to a rejecting endpoint — that is the bug.
	for i, p := range c.seen {
		if _, bad := p["page_size"]; bad {
			t.Errorf("request %d carried page_size=%q to a page_size-rejecting endpoint", i, p["page_size"])
		}
	}
	// (2) the first request must page by `page=1` (not an empty/absent page).
	if len(c.seen) == 0 || c.seen[0]["page"] != "1" {
		t.Errorf("first request page = %q, want \"1\"", firstPage(c.seen))
	}
	// (3) every page must be drained — all 4 rows land, no silent truncation.
	var n int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, "ip-addresses",
	).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 4 {
		t.Errorf("stored %d rows, want 4 (pages 1+2 fully drained)", n)
	}
}

// staticClient ignores the `page` param and returns the same body every call —
// the pathological "endpoint ignores page" shape the fingerprint guard defends.
type staticClient struct {
	calls int
	body  []byte
}

func (s *staticClient) RateLimit() float64 { return 0 }
func (s *staticClient) Get(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	s.calls++
	return json.RawMessage(s.body), nil
}

func TestSyncResource_PageIgnoringEndpoint_FingerprintTerminatesAtUnlimitedBudget(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	c := &staticClient{body: []byte(`[{"id":"n1"},{"id":"n2"},{"id":"n3"}]`)}
	// maxPages=0 is the PRODUCTION default (unlimited): the fingerprint guard,
	// not the page cap, must stop the walk. If it doesn't, this test hangs.
	res := syncResource(context.Background(), c, db, "networks", "", true, 0, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("syncResource returned error: %v", res.Err)
	}
	// Page 1 stores the set; page 2 returns the identical set → fingerprint
	// match → break. So exactly 2 requests, and only 3 distinct rows persist.
	if c.calls != 2 {
		t.Errorf("made %d requests, want 2 (fingerprint guard must break on the duplicate page)", c.calls)
	}
	var n int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, "networks",
	).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 3 {
		t.Errorf("stored %d distinct rows, want 3", n)
	}
}

func firstPage(seen []map[string]string) string {
	if len(seen) == 0 {
		return "<no requests>"
	}
	return seen[0]["page"]
}
