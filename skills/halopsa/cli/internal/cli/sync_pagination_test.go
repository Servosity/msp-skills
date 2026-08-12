// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the Halo paging contract encoded in
// determinePaginationDefaults. See skills/halopsa/handfixes.json
// (halo-pageinate-page-no-pagination) and docs/reprint-survival.md.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"halopsa-pp-cli/internal/store"
)

// fakeHaloPager models Halo's live paging contract, including the failure mode
// the hand-fix exists to prevent: unless pageinate=true is present Halo ignores
// page_size/page_no entirely and answers with the acting agent's default page
// size, which is how a 1254-ticket instance silently synced 50 rows.
type fakeHaloPager struct {
	total         int
	agentPageSize int
	// fixedPageSize, when non-zero, models the endpoint class (/Asset) that
	// ignores page_size and always answers with this many rows per page while
	// still reporting the full total in record_count.
	fixedPageSize   int
	calls           []map[string]string
	sawEnableParam  bool
	sawCursorParams []string
}

func (f *fakeHaloPager) RateLimit() float64 { return 0 }

func (f *fakeHaloPager) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	captured := make(map[string]string, len(params))
	for k, v := range params {
		captured[k] = v
	}
	f.calls = append(f.calls, captured)

	// Halo honours paging only when pageinate=true AND page_no are both
	// present. pageinate alone is not enough: it returns the agent default
	// page size and reports record_count as that short count, so the page
	// looks complete and a caller stops walking after one request.
	pageNo, hasPageNo := params["page_no"]
	paginated := params["pageinate"] == "true" && hasPageNo && pageNo != ""
	if params["pageinate"] == "true" {
		f.sawEnableParam = true
	}
	f.sawCursorParams = append(f.sawCursorParams, pageNo)

	size := f.agentPageSize
	page := 1
	if paginated {
		if raw, ok := params["page_size"]; ok {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				size = n
			}
		}
		if f.fixedPageSize > 0 {
			size = f.fixedPageSize // endpoint ignores page_size entirely
		}
		if n, err := strconv.Atoi(pageNo); err == nil && n > 0 {
			page = n
		}
	}

	start := (page - 1) * size
	end := start + size
	if start > f.total {
		start = f.total
	}
	if end > f.total {
		end = f.total
	}

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":%d,"summary":"ticket %d"}`, 1000+i, 1000+i))
	}

	// Halo reports record_count as the full result-set total when paginating,
	// and as the returned-row count when it is not.
	recordCount := len(rows)
	if paginated {
		recordCount = f.total
	}
	return json.RawMessage(fmt.Sprintf(
		`{"record_count":%d,"tickets":[%s],"include_children":false}`,
		recordCount, strings.Join(rows, ","),
	)), nil
}

func syncTicketsWithFake(t *testing.T, f *fakeHaloPager) (syncResult, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	res := syncResource(context.Background(), f, db, "tickets", "", true, 0, false, &syncUserParams{}, nil)
	if res.Err != nil {
		t.Fatalf("syncResource: %v", res.Err)
	}
	return res, db
}

// TestSyncPagination_WalksEveryPage is the regression guard: a result set
// several pages deep must land in full, not truncate at the first page.
func TestSyncPagination_WalksEveryPage(t *testing.T) {
	f := &fakeHaloPager{total: 250, agentPageSize: 50}
	res, db := syncTicketsWithFake(t, f)

	if res.Count != 250 {
		t.Fatalf("synced %d rows, want 250 (single-page truncation regressed)", res.Count)
	}
	stored, err := db.Count("tickets")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if stored != 250 {
		t.Fatalf("stored %d rows, want 250", stored)
	}
}

// TestSyncPagination_SendsEnableParam pins pageinate=true on every request.
// Without it Halo silently caps the sync at the agent's default page size.
func TestSyncPagination_SendsEnableParam(t *testing.T) {
	f := &fakeHaloPager{total: 250, agentPageSize: 50}
	syncTicketsWithFake(t, f)

	if !f.sawEnableParam {
		t.Fatal("no request carried pageinate=true; Halo would have returned the agent default page size")
	}
	for i, call := range f.calls {
		if call["pageinate"] != "true" {
			t.Fatalf("request %d missing pageinate=true, got %q", i, call["pageinate"])
		}
		if call["page_size"] != "100" {
			t.Fatalf("request %d page_size = %q, want 100", i, call["page_size"])
		}
	}
}

// TestSyncPagination_AdvancesByPageNumber pins the cursor parameter name and
// its 1-based page-number semantics. Offset arithmetic (0,100,200) against
// page_no would skip whole pages, since Halo reads the value as a page index.
func TestSyncPagination_AdvancesByPageNumber(t *testing.T) {
	f := &fakeHaloPager{total: 250, agentPageSize: 50}
	syncTicketsWithFake(t, f)

	for i, call := range f.calls {
		if _, wrong := call["page"]; wrong {
			t.Fatalf("request %d sent 'page'; Halo's cursor parameter is 'page_no'", i)
		}
	}
	// page_no must be explicit from the first request onward: Halo does not
	// default it, and omitting it silently caps the walk at one short page.
	want := []string{"1", "2", "3"}
	if len(f.sawCursorParams) != len(want) {
		t.Fatalf("made %d requests (%v), want %d", len(f.sawCursorParams), f.sawCursorParams, len(want))
	}
	for i, w := range want {
		if f.sawCursorParams[i] != w {
			t.Fatalf("request %d page_no = %q, want %q (offset arithmetic would skip pages)", i, f.sawCursorParams[i], w)
		}
	}
}

// TestSyncPagination_EndpointIgnoringPageSize covers the /Asset class: the
// endpoint caps pages at 50 no matter what page_size asks for, so a
// "len(items) >= limit means more pages" heuristic sees 50 < 100 and stops one
// page in. The walk must instead trust the record_count total, which stays
// authoritative regardless of the page size actually served.
func TestSyncPagination_EndpointIgnoringPageSize(t *testing.T) {
	f := &fakeHaloPager{total: 144, agentPageSize: 50, fixedPageSize: 50}
	res, db := syncTicketsWithFake(t, f)

	if res.Count != 144 {
		t.Fatalf("synced %d rows, want 144; a fixed-page-size endpoint truncated the walk", res.Count)
	}
	stored, err := db.Count("tickets")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if stored != 144 {
		t.Fatalf("stored %d rows, want 144", stored)
	}
	if len(f.calls) != 3 {
		t.Fatalf("made %d requests, want 3 (50+50+44)", len(f.calls))
	}
}

// TestSyncPagination_StopsAtReportedTotal guards the other direction: the walk
// must not keep requesting pages once the reported total is reached, even
// though every page it saw was "full".
func TestSyncPagination_StopsAtReportedTotal(t *testing.T) {
	f := &fakeHaloPager{total: 200, agentPageSize: 50}
	res, _ := syncTicketsWithFake(t, f)

	if res.Count != 200 {
		t.Fatalf("synced %d rows, want 200", res.Count)
	}
	// 100 + 100 exactly exhausts the set; one probe past the end is
	// acceptable, more than that means the walk did not honour the total.
	if len(f.calls) > 3 {
		t.Fatalf("made %d requests for an exactly-divisible set, want at most 3", len(f.calls))
	}
}

// TestSyncPagination_DefaultsMatchLiveContract pins the resolved values
// themselves, so a reprint that restores the profiler's defaults fails loudly
// even if the fake above drifts.
func TestSyncPagination_DefaultsMatchLiveContract(t *testing.T) {
	got := determinePaginationDefaults()
	if got.cursorParam != "page_no" {
		t.Errorf("cursorParam = %q, want page_no", got.cursorParam)
	}
	if got.cursorType != "page" {
		t.Errorf("cursorType = %q, want page (offset arithmetic skips pages)", got.cursorType)
	}
	if got.limitParam != "page_size" {
		t.Errorf("limitParam = %q, want page_size", got.limitParam)
	}
	if got.limit != 100 {
		t.Errorf("limit = %d, want 100 (Halo's documented maximum)", got.limit)
	}
	if got.enableParam != "pageinate" {
		t.Errorf("enableParam = %q, want pageinate", got.enableParam)
	}
	if got.startCursor != "1" {
		t.Errorf("startCursor = %q, want 1 (Halo does not default page_no)", got.startCursor)
	}
}

// TestSyncPagination_FirstRequestCarriesCursor is the specific guard for the
// subtlest half of the bug: pageinate=true and page_size were both correct,
// yet the sync still stored one short page because page_no was omitted on the
// first request and Halo reported that short page as the complete result set.
func TestSyncPagination_FirstRequestCarriesCursor(t *testing.T) {
	f := &fakeHaloPager{total: 250, agentPageSize: 50}
	syncTicketsWithFake(t, f)

	if len(f.calls) == 0 {
		t.Fatal("no requests made")
	}
	if got := f.calls[0]["page_no"]; got != "1" {
		t.Fatalf("first request page_no = %q, want 1; without it Halo returns %d rows and reports them as the full set", got, f.agentPageSize)
	}
}
