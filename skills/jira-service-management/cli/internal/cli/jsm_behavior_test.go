// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"jira-service-management-pp-cli/internal/store"
)

// A paused SLA that already breached outranks a running one with time left,
// and stays in the list even without --include-paused.
func TestPausedBreachCounts(t *testing.T) {
	slas := []jsmSLA{
		{Name: "TTR", OngoingCycle: cyc(18_000_000, false, false)},
		{Name: "TTFR", OngoingCycle: cyc(-1, true, true)},
	}
	r, ok := mostUrgent(slas)
	if !ok || r.Metric != "TTFR" || !r.Breached {
		t.Fatalf("mostUrgent = %+v, want the paused breached TTFR", r)
	}
	rows := map[string]*breachRow{"A-1": {Key: "A-1"}}
	got := rankBreaches([]string{"A-1"}, rows, map[string][]jsmSLA{"A-1": slas}, map[string][]int{"A-1": {0}}, []queueSummary{{}}, 2*time.Hour, false, 10)
	if len(got) != 1 {
		t.Fatalf("paused-but-breached ticket dropped: %+v", got)
	}
}

func TestRankBreaches(t *testing.T) {
	rows := map[string]*breachRow{"A-1": {Key: "A-1"}, "A-2": {Key: "A-2"}, "A-3": {Key: "A-3"}, "A-4": {Key: "A-4"}, "A-5": {Key: "A-5"}}
	slas := map[string][]jsmSLA{
		"A-1": {{Name: "TTR", OngoingCycle: cyc(600_000, false, false)}},          // 10m
		"A-2": {{Name: "TTR", OngoingCycle: cyc(-5, false, false)}},               // out of time, not flagged yet
		"A-3": {{Name: "TTR", OngoingCycle: cyc(36_000_000, false, false)}},       // 10h
		"A-4": {{Name: "TTR", OngoingCycle: cyc(60_000, false, true)}},            // paused only
		"A-5": {{Name: "Done", CompletedCycles: []jsmSLACycle{{Breached: true}}}}, // no running SLA
	}
	queues := []queueSummary{{Queue: "q1"}, {Queue: "q2"}}
	queueOfKey := map[string][]int{"A-1": {0, 1}, "A-2": {0}, "A-3": {1}, "A-4": {0}}
	order := []string{"A-1", "A-2", "A-3", "A-4", "A-5", "A-404"}

	got := rankBreaches(order, rows, slas, queueOfKey, queues, time.Hour, false, 10)
	if len(got) != 2 || got[0].Key != "A-2" || !got[0].Breached || got[1].Key != "A-1" {
		t.Fatalf("ranked = %+v, want A-2 (breached by time) then A-1; A-3 outside window, A-4 paused, A-5 not running", got)
	}
	if queues[0].Breached != 1 || queues[0].Running != 1 || queues[1].Running != 1 || queues[1].Breached != 0 {
		t.Fatalf("queue counts = %+v", queues)
	}

	queues2 := []queueSummary{{}, {}}
	all := rankBreaches(order, rows, slas, queueOfKey, queues2, 0, true, 2)
	if len(all) != 2 || all[0].Key != "A-2" {
		t.Fatalf("limit/paused: %+v", all)
	}
}

// fakeGetHeaders records the query each page was requested with.
type fakeGetHeaders struct {
	total int
	cap   int
	seen  []map[string]string
}

func (f *fakeGetHeaders) GetWithHeaders(_ context.Context, _ string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	cp := map[string]string{}
	for k, v := range params {
		cp[k] = v
	}
	f.seen = append(f.seen, cp)
	start, _ := strconv.Atoi(params["start"])
	limit, _ := strconv.Atoi(params["limit"])
	if limit > f.cap {
		limit = f.cap
	}
	vals := []map[string]any{}
	for i := start; i < start+limit && i < f.total; i++ {
		vals = append(vals, map[string]any{"id": strconv.Itoa(i)})
	}
	return json.Marshal(map[string]any{"start": start, "limit": limit, "size": len(vals), "isLastPage": true, "values": vals})
}

// The generated pager must walk JSM on the wire key `start` with pages of at
// most 50 (hook: jsmWireParam + jsmClampAllLimit in paginatedGet).
func TestPaginatedGetJSMHook(t *testing.T) {
	f := &fakeGetHeaders{total: 120, cap: 50}
	data, err := paginatedGet(context.Background(), f, "/servicedesk/1/customer", map[string]string{"limit": "100"}, nil, true, "offset", "offset", "limit", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if f.seen[0]["limit"] != "50" {
		t.Fatalf("first page limit = %q, want 50 (clamp)", f.seen[0]["limit"])
	}
	for _, q := range f.seen {
		if _, bad := q["offset"]; bad {
			t.Fatalf("pager sent public name ?offset=: %v", q)
		}
	}
	if len(f.seen) < 3 || f.seen[1]["start"] != "50" || f.seen[2]["start"] != "100" {
		t.Fatalf("pages = %v, want start=0,50,100", f.seen)
	}
	var items []json.RawMessage
	if json.Unmarshal(data, &items) != nil {
		var env struct {
			Values []json.RawMessage `json:"values"`
		}
		_ = json.Unmarshal(data, &env)
		items = env.Values
	}
	if len(items) != 120 {
		t.Fatalf("collected %d items, want 120", len(items))
	}
}

type fakePoster struct {
	total      int
	ignoreFrom bool
	calls      int
}

func (f *fakePoster) PostQueryWithParams(_ context.Context, _ string, params map[string]string, _ any) (json.RawMessage, int, error) {
	f.calls++
	start, _ := strconv.Atoi(params["startAt"])
	if f.ignoreFrom {
		start = 0
	}
	size, _ := strconv.Atoi(params["maxResults"])
	vals := []map[string]any{}
	for i := start; i < start+size && i < f.total; i++ {
		vals = append(vals, map[string]any{"id": strconv.Itoa(i), "objectKey": fmt.Sprintf("IT-%d", i)})
	}
	body, _ := json.Marshal(map[string]any{"isLast": start+size >= f.total, "values": vals, "objectTypeAttributes": []map[string]any{{"id": "1", "name": "Name"}}})
	return body, 200, nil
}

func TestAssetsAQL(t *testing.T) {
	f := &fakePoster{total: 120}
	objs, names, truncated, err := assetsAQL(context.Background(), f, `objectType = "Laptop"`, 0)
	if err != nil || truncated || len(objs) != 120 || names["1"] != "Name" || f.calls != 3 {
		t.Fatalf("objs=%d truncated=%v calls=%d err=%v", len(objs), truncated, f.calls, err)
	}
	objs, _, truncated, _ = assetsAQL(context.Background(), &fakePoster{total: 120}, "x", 60)
	if len(objs) != 60 || !truncated {
		t.Fatalf("capped walk: objs=%d truncated=%v", len(objs), truncated)
	}
	objs, _, truncated, err = assetsAQL(context.Background(), &fakePoster{total: 500, ignoreFrom: true}, "x", 0)
	if err != nil || len(objs) != 50 || !truncated {
		t.Fatalf("server ignoring startAt must stop after one page of new ids, reported incomplete: objs=%d truncated=%v err=%v", len(objs), truncated, err)
	}
}

// shiftedLastPage returns ids 0..49, then a last page holding only id 49
// (a row shifted back mid-walk): that is the end, not a stall.
type shiftedLastPage struct{}

func (shiftedLastPage) PostQueryWithParams(_ context.Context, _ string, params map[string]string, _ any) (json.RawMessage, int, error) {
	vals := []map[string]any{{"id": "49"}}
	last := true
	if params["startAt"] == "0" {
		vals, last = nil, false
		for i := 0; i < 50; i++ {
			vals = append(vals, map[string]any{"id": strconv.Itoa(i)})
		}
	}
	body, _ := json.Marshal(map[string]any{"isLast": last, "values": vals})
	return body, 200, nil
}

func TestAssetsAQLShiftedLastPageIsComplete(t *testing.T) {
	objs, _, truncated, err := assetsAQL(context.Background(), shiftedLastPage{}, "x", 0)
	if err != nil || truncated || len(objs) != 50 {
		t.Fatalf("objs=%d truncated=%v err=%v; a last page of already-seen rows is the end", len(objs), truncated, err)
	}
}

// A server that ignores ?start= and no key func: the repeat guard stops it.
type stuckPager struct{ calls int }

func (s *stuckPager) GetWithHeadersNoCache(_ context.Context, _ string, _ map[string]string, _ map[string]string) (json.RawMessage, error) {
	s.calls++
	vals := make([]map[string]any, 50)
	for i := range vals {
		vals[i] = map[string]any{"status": "Open"}
	}
	return json.Marshal(map[string]any{"limit": 50, "values": vals})
}

func TestJSMWalkStopsOnRepeatedPage(t *testing.T) {
	s := &stuckPager{}
	items, _, err := jsmWalk(context.Background(), s, "/request/A-1/status", nil, 0, nil)
	if err == nil || s.calls != 2 || len(items) != 50 {
		t.Fatalf("calls=%d items=%d err=%v; an unlimited walk must stop on the first repeated page with an error", s.calls, len(items), err)
	}
	s = &stuckPager{}
	_, truncated, err := jsmWalk(context.Background(), s, "/request/A-1/status", nil, 5, nil)
	if err != nil || !truncated || s.calls != 2 {
		t.Fatalf("calls=%d truncated=%v err=%v; a capped walk reports the stall as truncated", s.calls, truncated, err)
	}
}

func TestJSMPathSegAndTermSafe(t *testing.T) {
	for _, bad := range []string{"", "..", ".", "SD-1/../x", `a\b`} {
		if _, err := jsmPathSeg(bad); err == nil {
			t.Errorf("jsmPathSeg(%q) must fail", bad)
		}
	}
	if got, err := jsmPathSeg(" SD-123 "); err != nil || got != "SD-123" {
		t.Fatalf("jsmPathSeg = %q %v", got, err)
	}
	if got, _ := jsmPathSeg("a b?c"); got != "a%20b%3Fc" {
		t.Fatalf("escape = %q", got)
	}
	if got := termSafe("ok\x1b[31mred\x07\tx"); got != "ok[31mred\tx" {
		t.Fatalf("termSafe = %q", got)
	}
	if truncateText("abc", 0) != "" || truncateText("abcdef", 4) != "abc…" {
		t.Fatal("truncateText bounds")
	}
}

// sla-report and client read generated store names; fail loudly if a
// generator change renames them.
func TestLocalStoreNamesExist(t *testing.T) {
	for _, r := range []string{"request", "servicedesk", "requesttype", "organization"} {
		if _, err := syncResourcePath(r); err != nil {
			t.Errorf("sync resource %q missing: %v", r, err)
		}
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM pragma_table_info('user') WHERE name='organization_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("store table user.organization_id missing (n=%d err=%v); client's org membership join would silently return zero members", n, err)
	}
}

func TestNoteKeyOK(t *testing.T) {
	for _, ok := range []string{"SD-123", "MY_PROJ-7", "10001"} {
		if !noteKeyOK(ok) {
			t.Errorf("%q must be accepted", ok)
		}
	}
	for _, bad := range []string{"", "SD-1/../../x", "sd 1", "SD-", "-1", "SD-1?x"} {
		if noteKeyOK(bad) {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

// export and tail page with JSM's wire keys and its 50-row cap.
func TestResourceReadConfigWireNames(t *testing.T) {
	req := resourceReadConfigFor("request")
	if req.cursorParam != "start" || req.limitParam != "limit" || req.pageSize != 50 {
		t.Fatalf("request read config = %+v, want start/limit/50", req)
	}
	if got := resourcePageParams(req, "", 2, 0); got["start"] != "100" || got["limit"] != "50" {
		t.Fatalf("page 3 params = %v, want start=100 limit=50", got)
	}
	schema := resourceReadConfigFor("objectschema")
	if schema.cursorParam != "startAt" || schema.limitParam != "maxResults" {
		t.Fatalf("objectschema read config = %+v, want startAt/maxResults", schema)
	}
}

type slaPager struct{ paths []string }

func (s *slaPager) GetWithHeadersNoCache(_ context.Context, path string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	s.paths = append(s.paths, path)
	if params["start"] != "" && params["start"] != "0" {
		return json.RawMessage(`{"values":[]}`), nil
	}
	return json.RawMessage(`{"isLastPage":true,"values":[{"id":"1","name":"TTR"},{"id":"2","name":"TTFR"}]}`), nil
}

// A request whose embedded SLA page is incomplete gets its full SLA from
// /request/{key}/sla before it is stored; complete pages are left alone, and
// the reports count any request still incomplete.
func TestSyncCompletesRequestSLA(t *testing.T) {
	g := &slaPager{}
	items := []json.RawMessage{
		json.RawMessage(`{"issueKey":"SD-1","sla":{"isLastPage":false,"values":[{"id":"1","name":"TTR"}]}}`),
		json.RawMessage(`{"issueKey":"SD-2","sla":{"isLastPage":true,"values":[{"id":"1","name":"TTR"}]}}`),
	}
	got := jsmCompleteRequestSLA(context.Background(), g, "request", items)
	var r1, r2 localRequest
	_ = json.Unmarshal(got[0], &r1)
	_ = json.Unmarshal(got[1], &r2)
	if len(r1.SLA.Values) != 2 || r1.slaIncomplete() {
		t.Fatalf("SD-1 SLA not completed: %s", got[0])
	}
	if len(r2.SLA.Values) != 1 || len(g.paths) == 0 || g.paths[0] != "/request/SD-1/sla" {
		t.Fatalf("complete page touched or wrong path: %s %v", got[1], g.paths)
	}
	if out := jsmCompleteRequestSLA(context.Background(), g, "organization", []json.RawMessage{items[0]}); string(out[0]) != string(items[0]) {
		t.Fatal("non-request resource rewritten")
	}
	var partial localRequest
	_ = json.Unmarshal([]byte(`{"issueKey":"SD-3","reporter":{"accountId":"a"},"sla":{"isLastPage":false,"values":[]}}`), &partial)
	if p := clientRollup("1", "Acme", map[string]bool{"a": true}, []localRequest{partial}, nil, 0); p.SLAIncomplete != 1 {
		t.Fatalf("client rollup did not count the incomplete SLA: %+v", p)
	}
}

// A single-item export fills the item placeholder, not Assets' {cloudId}.
func TestResourceDetailPathFillsItemID(t *testing.T) {
	got, err := resourceDetailPath("objectschema", "123")
	if err != nil || got != "https://api.atlassian.com/ex/jira/{cloudId}/jsm/assets/workspace/{workspaceId}/v1/objectschema/123" {
		t.Fatalf("objectschema item path = %q, %v", got, err)
	}
	if got, _ := resourceDetailPath("request", "SD-1"); got != "/request/SD-1" {
		t.Fatalf("request item path = %q", got)
	}
}
