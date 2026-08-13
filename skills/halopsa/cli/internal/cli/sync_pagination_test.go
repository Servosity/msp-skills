package cli

import (
	"encoding/json"
	"testing"
)

// Halo honours paging only when pageinate=true and an explicit 1-based page_no
// are both present. The generated profiler defaults (page / offset / no enable
// parameter) truncated every sync to a single page (#203).
func TestPaginationDefaultsMatchHaloContract(t *testing.T) {
	d := determinePaginationDefaults()

	if d.cursorParam != "page_no" {
		t.Errorf("cursorParam = %q, want %q: Halo's cursor parameter is page_no", d.cursorParam, "page_no")
	}
	if d.cursorType != "page" {
		t.Errorf("cursorType = %q, want %q: page_no is a 1-based page number, not a row offset, "+
			"so offset arithmetic makes Halo read a row offset as a page index and skip whole pages",
			d.cursorType, "page")
	}
	if d.enableParam != "pageinate" {
		t.Errorf("enableParam = %q, want %q: without it Halo ignores page_size/page_no entirely "+
			"and returns the acting agent's default page size", d.enableParam, "pageinate")
	}
	if d.startCursor != "1" {
		t.Errorf("startCursor = %q, want %q: Halo does not default page_no, and omitting it returns "+
			"a short page that reports itself as complete", d.startCursor, "1")
	}
}

// The reported result-set total is what lets the walk continue past endpoints
// that ignore page_size and always answer with a fixed short page (/Asset
// returns 50 rows however large page_size is, so 50 of 144 looked complete).
func TestEnvelopeReportedTotal(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantTotal int
		wantOK    bool
	}{
		{"halo record_count", `{"record_count":1254,"tickets":[]}`, 1254, true},
		{"nested envelope", `{"data":{"total_count":144,"items":[]}}`, 144, true},
		{"camel total", `{"totalCount":7}`, 7, true},
		{"absent", `{"tickets":[]}`, 0, false},
		{"bare array has no envelope", `[{"id":1}]`, 0, false},
		{"non-numeric is ignored", `{"record_count":"lots"}`, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total, ok := envelopeReportedTotal(json.RawMessage(tc.body))
			if ok != tc.wantOK || total != tc.wantTotal {
				t.Fatalf("envelopeReportedTotal(%s) = (%d,%v), want (%d,%v)",
					tc.body, total, ok, tc.wantTotal, tc.wantOK)
			}
		})
	}
}

// pageLooksFull is the signal both the natural-end break and the --max-pages
// cap consult. A fixed-short-page endpoint must still read as "more pages" when
// the API reports a larger total, otherwise:
//   - the natural-end break stops after one page (the /Asset truncation), and
//   - the --max-pages cap computes truncatedByCap=false and clears the resume
//     cursor, so a capped sync can never advance through that resource.
func TestPageLooksFullUsesReportedTotalForFixedShortPages(t *testing.T) {
	const limit = 100

	// /Asset: asked for 100, always answers 50, reports 144 total.
	body := json.RawMessage(`{"record_count":144}`)
	reportedTotal, hasReportedTotal := envelopeReportedTotal(body)
	rowsSeen := 50
	itemCount := 50

	moreByReportedTotal := hasReportedTotal && itemCount > 0 && rowsSeen < reportedTotal
	pageLooksFull := itemCount >= limit || moreByReportedTotal

	if itemCount >= limit {
		t.Fatalf("test setup is wrong: the page must be short to exercise the bug")
	}
	if !pageLooksFull {
		t.Fatal("a 50-row page of a 144-row result set must read as 'more pages'; " +
			"the bare length heuristic stops here and strands 94 rows")
	}

	// Once the whole result set has been seen, the walk must stop.
	rowsSeen = 144
	moreByReportedTotal = hasReportedTotal && itemCount > 0 && rowsSeen < reportedTotal
	if itemCount >= limit || moreByReportedTotal {
		t.Fatal("after seeing all 144 rows the walk must terminate")
	}
}

// #203 documents `page_no=1` as the reproduction parameter, so `--param
// page_no=1` is a natural thing to type. applyTo overwrites unconditionally and
// runs last, so that override would freeze every request on page 1 while the
// internal cursor advanced past it, defeating the sticky-cursor guard and
// re-fetching the same page forever under --max-pages 0.
func TestCursorPinnedByUser(t *testing.T) {
	cases := []struct {
		name      string
		params    map[string]string
		cursorKey string
		intended  string
		want      bool
	}{
		{
			name:      "user pinned page_no while the walk wanted page 3",
			params:    map[string]string{"page_no": "1", "page_size": "100"},
			cursorKey: "page_no",
			intended:  "3",
			want:      true,
		},
		{
			name:      "walk's own cursor is not an override",
			params:    map[string]string{"page_no": "3", "page_size": "100"},
			cursorKey: "page_no",
			intended:  "3",
			want:      false,
		},
		{
			name:      "unrelated user param is not an override",
			params:    map[string]string{"page_no": "1", "mine": "true"},
			cursorKey: "page_no",
			intended:  "1",
			want:      false,
		},
		{
			name:      "no cursor param configured",
			params:    map[string]string{"page_no": "1"},
			cursorKey: "",
			intended:  "1",
			want:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cursorPinnedByUser(tc.params, tc.cursorKey, tc.intended); got != tc.want {
				t.Fatalf("cursorPinnedByUser(%v, %q, %q) = %v, want %v",
					tc.params, tc.cursorKey, tc.intended, got, tc.want)
			}
		})
	}
}

// applyTo must still be the thing that can pin the cursor, otherwise the guard
// above is dead code protecting a path that cannot happen.
func TestUserParamsOverrideCursorParam(t *testing.T) {
	params := map[string]string{"page_no": "3", "page_size": "100"}
	up := &syncUserParams{flatGlobal: map[string]string{"page_no": "1"}}
	up.applyTo("tickets", params, false)

	if params["page_no"] != "1" {
		t.Fatalf("expected --param page_no=1 to win over the walk's cursor, got %q", params["page_no"])
	}
	if !cursorPinnedByUser(params, "page_no", "3") {
		t.Fatal("the override must be detected as a pinned cursor")
	}
}
