// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for `issues watch` and `events list`.
// Pinned by this connector's handfixes.json under the "issues-watch-events"
// entry.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/config"
	"servosity-msp-pp-cli/internal/store"
)

const (
	issueA = "aaaaaaaa-0000-4000-8000-000000000001"
	issueB = "bbbbbbbb-0000-4000-8000-000000000002"
	issueC = "cccccccc-0000-4000-8000-000000000003"
	issueD = "dddddddd-0000-4000-8000-000000000004"
)

// fakeIssuesAPI serves /resellers/7/issues/ two issues per page through a
// cursor, and /issues/{id}/ from detail. Every request is recorded.
type fakeIssuesAPI struct {
	mu       sync.Mutex
	list     []map[string]any
	detail   map[string]map[string]any
	requests []string
	failList string         // when set, the list answers HTTP 400 with this body
	status   map[string]int // per issue id: answer /issues/{id}/ with this status
	badNext  bool           // when set, every page links back to the first
	pause    string         // when set, the first request for this path (and query) waits for unpause
	paused   chan struct{}  // closed when that request arrives
	unpause  chan struct{}
}

// pauseOnce holds the next request for uri (path and query) until the returned release is
// called, and returns once that request has arrived; the API keeps serving
// every other request meanwhile.
func (f *fakeIssuesAPI) pauseOnce(uri string) (arrived <-chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pause, f.paused, f.unpause = uri, make(chan struct{}), make(chan struct{})
	return f.paused, func() { close(f.unpause) }
}

func (f *fakeIssuesAPI) setStatus(status map[string]int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

func (f *fakeIssuesAPI) set(list []map[string]any, detail map[string]map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list, f.detail = list, detail
}

func (f *fakeIssuesAPI) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeIssuesAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if f.pause != "" && r.URL.RequestURI() == f.pause {
		f.pause = ""
		close(f.paused)
		unpause := f.unpause
		f.mu.Unlock()
		<-unpause
		f.mu.Lock()
	}
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/resellers/7/issues/" {
		if f.failList != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(f.failList))
			return
		}
		start := 0
		if cur := r.URL.Query().Get("cursor"); cur != "" {
			_, _ = fmt.Sscanf(cur, "p%d", &start)
		}
		end := min(start+2, len(f.list))
		page := map[string]any{"results": f.list[start:end], "next": nil}
		if f.badNext {
			page["next"] = fmt.Sprintf("http://%s/resellers/7/issues/?cursor=p0", r.Host)
		} else if end < len(f.list) {
			page["next"] = fmt.Sprintf("http://%s/resellers/7/issues/?cursor=p%d", r.Host, end)
		}
		_ = json.NewEncoder(w).Encode(page)
		return
	}
	for id, code := range f.status {
		if r.URL.Path == "/issues/"+id+"/" {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"detail":"You don't have access to this issue."}`))
			return
		}
	}
	for id, d := range f.detail {
		if r.URL.Path == "/issues/"+id+"/" {
			_ = json.NewEncoder(w).Encode(d)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"detail":"Not found."}`))
}

func fixtureIssue(id, state, severity, updated string) map[string]any {
	return map[string]any{
		"id": id, "state": state, "severity": severity, "priority": 3000, "audience": "SUPPORT",
		"company": 4421, "backup": 10, "created_at": "2026-10-01T00:00:00Z", "updated_at": updated,
		"last_seen_at": updated, "description": "FIXTURE-DESCRIPTION free text", "company_name": "Acme",
	}
}

func newWatchFixture(t *testing.T, token string) (*fakeIssuesAPI, string) {
	t.Helper()
	isolateNovelStore(t)
	api := &fakeIssuesAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", token)
	t.Setenv("SERVOSITY_MSP_RESELLER_ID", "7")
	return api, filepath.Join(t.TempDir(), "watch.db")
}

// runCLI runs one command and returns stdout, stderr and the error.
func runCLI(args ...string) (string, string, error) {
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func watchOnce(t *testing.T, db string) map[string]any {
	t.Helper()
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"issues", "watch", "--once", "--json", "--db", db})
	if err := root.Execute(); err != nil {
		t.Fatalf("issues watch --once: %v\n%s", err, errOut.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got["stale"] != nil {
		t.Errorf("a poll with no other watcher skipped a write as stale: %v", got)
	}
	return got
}

type listedEvents struct {
	Events []struct {
		EventID   string         `json:"eventId"`
		Name      string         `json:"name"`
		Timestamp string         `json:"timestamp"`
		Data      map[string]any `json:"data"`
		Cursor    string         `json:"cursor"`
		Source    string         `json:"source"`
	} `json:"events"`
	Cursor string `json:"cursor"`
	More   bool   `json:"hasMore"`
}

func listEvents(t *testing.T, db string, args ...string) listedEvents {
	t.Helper()
	out, errOut, err := runCLI(append([]string{"events", "list", "--json", "--db", db}, args...)...)
	if err != nil {
		t.Fatalf("events list %v: %v\n%s", args, err, errOut)
	}
	var got listedEvents
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return got
}

func eventNames(l listedEvents) []string {
	var out []string
	for _, e := range l.Events {
		out = append(out, e.Name+" "+e.Data["issue_id"].(string))
	}
	return out
}

// setWatchClock moves the poll clock for the rest of the test.
func setWatchClock(t *testing.T, at time.Time) {
	t.Helper()
	issueWatchNow = func() time.Time { return at }
	t.Cleanup(func() { issueWatchNow = time.Now })
}

// TestIssuesWatchRecordsEachTransition walks a baseline and every event type
// through the real command against a fake API shaped like servosity-api's
// (the reseller list holds ACTIVE issues only, 2 per page here): all pages
// are read, the first run records no events, each later change is one row
// in cursor order, an unchanged poll adds nothing, an ignored issue is
// re-read only after an hour, an archived one never, and only GETs reach
// Servosity.
func TestIssuesWatchRecordsEachTransition(t *testing.T) {
	api, db := newWatchFixture(t, "fixture-token-watch")
	clock := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	setWatchClock(t, clock)
	t1 := "2026-10-27T10:00:00Z"
	api.set([]map[string]any{
		fixtureIssue(issueA, "ACTIVE", "WARNING", t1),
		fixtureIssue(issueB, "ACTIVE", "WARNING", t1),
		fixtureIssue(issueC, "ACTIVE", "WARNING", t1),
	}, nil)

	got := watchOnce(t, db)
	if got["baseline"] != true || got["issues"] != float64(3) || got["events"] != float64(0) || got["complete"] != true {
		t.Fatalf("baseline run: %v", got)
	}
	if l := listEvents(t, db, "--since", "0"); len(l.Events) != 0 || l.Cursor != "0" {
		t.Fatalf("baseline flooded events: %+v", l)
	}

	// A escalates and D is new; B was archived and C ignored, so both left the list.
	t2 := "2026-10-27T11:00:00Z"
	api.set([]map[string]any{
		fixtureIssue(issueA, "ACTIVE", "DANGER", t2),
		fixtureIssue(issueD, "ACTIVE", "WARNING", t2),
	}, map[string]map[string]any{
		issueB: fixtureIssue(issueB, "ARCHIVED", "WARNING", t2),
		issueC: fixtureIssue(issueC, "IGNORED", "WARNING", t2),
	})
	if got := watchOnce(t, db); got["baseline"] != false || got["events"] != float64(4) {
		t.Fatalf("second run: %v", got)
	}

	// C comes back, A is ignored, D is gone (404).
	t3 := "2026-10-27T12:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueC, "ACTIVE", "WARNING", t3)},
		map[string]map[string]any{issueA: fixtureIssue(issueA, "IGNORED", "DANGER", t3)})
	if got := watchOnce(t, db); got["events"] != float64(2) {
		t.Fatalf("third run: %v", got)
	}
	before := len(api.seen())
	if got := watchOnce(t, db); got["events"] != float64(0) {
		t.Fatalf("unchanged run: %v", got)
	}
	for _, r := range api.seen()[before:] {
		if strings.HasPrefix(r, "GET /issues/") {
			t.Errorf("an issue was re-read within the hour: %s", r)
		}
	}

	// Two hours on, the ignored A is re-read and has been archived while
	// out of the list (servosity-api prune_issues); archived B is not re-read.
	setWatchClock(t, clock.Add(2*time.Hour))
	t4 := "2026-10-27T13:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueC, "ACTIVE", "WARNING", t3)},
		map[string]map[string]any{issueA: fixtureIssue(issueA, "ARCHIVED", "DANGER", t4)})
	before = len(api.seen())
	if got := watchOnce(t, db); got["events"] != float64(1) {
		t.Fatalf("hourly re-read: %v", got)
	}
	rereads := strings.Join(api.seen()[before:], "\n")
	if !strings.Contains(rereads, "GET /issues/"+issueA+"/") || strings.Contains(rereads, issueB) || strings.Contains(rereads, issueD) {
		t.Errorf("re-reads after an hour:\n%s", rereads)
	}

	all := listEvents(t, db, "--since", "0")
	want := []string{
		"servosity.issue.updated " + issueA,
		"servosity.issue.created " + issueD,
		"servosity.issue.archived " + issueB,
		"servosity.issue.ignored " + issueC,
		"servosity.issue.reactivated " + issueC,
		"servosity.issue.ignored " + issueA,
		"servosity.issue.archived " + issueA,
	}
	if g := eventNames(all); strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events =\n%s\nwant\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
	for i, e := range all.Events {
		if e.Cursor != fmt.Sprint(i+1) || e.Source != "poll" || e.Timestamp == "" || !strings.HasPrefix(e.EventID, e.Name+":"+e.Data["issue_id"].(string)) {
			t.Errorf("event %d: %+v", i, e)
		}
		for _, k := range []string{"description", "company_name", "checked_at"} {
			if _, ok := e.Data[k]; ok {
				t.Errorf("event %d carries %s", i, k)
			}
		}
	}
	if d := all.Events[0].Data; d["severity"] != "DANGER" || d["company"] != float64(4421) || !strings.Contains(fmt.Sprint(d["changed"]), "severity") {
		t.Errorf("updated data: %v", d)
	}
	if d := all.Events[2].Data; d["state"] != "ARCHIVED" || d["previous_state"] != "ACTIVE" {
		t.Errorf("archived data: %v", d)
	}
	if d := all.Events[6].Data; d["previous_state"] != "IGNORED" {
		t.Errorf("ignored-then-archived data: %v", d)
	}

	// The cursor pages forward, --type narrows, --limit reports hasMore.
	if l := listEvents(t, db, "--since", "4"); len(l.Events) != 3 || l.Events[0].Cursor != "5" || l.Cursor != "7" || l.More {
		t.Errorf("--since 4: %+v", l)
	}
	if l := listEvents(t, db, "--since", "0", "--type", "servosity.issue.ignored"); len(l.Events) != 2 || l.Cursor != "6" {
		t.Errorf("--type: %+v", l)
	}
	if l := listEvents(t, db, "--since", "0", "--limit", "1"); len(l.Events) != 1 || l.Cursor != "1" || !l.More {
		t.Errorf("--limit 1: %+v", l)
	}
	if _, _, err := runCLI("events", "list", "--db", db, "--type", "issue.deleted"); err == nil {
		t.Error("an unknown --type was accepted")
	}

	pages := 0
	for _, r := range api.seen() {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("non-GET request: %s", r)
		}
		if strings.Contains(r, "cursor=p2") {
			pages++
		}
	}
	if pages != 1 {
		t.Errorf("second page read %d times; only the baseline poll has one", pages)
	}
	raw, err := os.ReadFile(db)
	if err != nil || bytes.Contains(raw, []byte("FIXTURE-DESCRIPTION")) || bytes.Contains(raw, []byte("Acme")) {
		t.Errorf("store keeps free text (%v)", err)
	}
}

// TestIssueObservationsDropStaleReads: a read older than the recorded one
// (an earlier updated_at, or the same updated_at and an earlier
// last_seen_at) changes nothing, the case two watchers racing produce.
func TestIssueObservationsDropStaleReads(t *testing.T) {
	isolateNovelStore(t)
	db, err := store.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := func(state, updated, seen string) watchIssue {
		return watchIssue{ID: issueA, State: state, Severity: "WARNING", UpdatedAt: updated, LastSeenAt: seen}
	}
	now := time.Now()
	steps := []struct {
		obs  watchIssue
		want int
	}{
		{at("ACTIVE", "2026-10-27T10:00:00Z", "2026-10-27T10:00:00Z"), 0}, // baseline
		{at("IGNORED", "2026-10-27T11:00:00Z", "2026-10-27T10:00:00Z"), 1},
		{at("ACTIVE", "2026-10-27T12:00:00Z", "2026-10-27T10:00:00Z"), 1},
		{at("IGNORED", "2026-10-27T11:00:00Z", "2026-10-27T10:00:00Z"), 0},      // the paused watcher's read
		{at("ACTIVE", "2026-10-27T12:00:00Z", "2026-10-27T12:30:00.5Z"), 1},     // recurrence: last_seen_at only
		{at("ACTIVE", "2026-10-27T12:00:00.000Z", "2026-10-27T12:30:00.4Z"), 0}, // older recurrence
	}
	for i, st := range steps {
		based, err := db.IssueWatchState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		res, err := recordIssueObservations(ctx, db, "poll", based, []watchIssue{st.obs}, nil, true, now)
		if err != nil || res.Events != st.want {
			t.Fatalf("step %d: %+v %v, want %d events", i, res, err, st.want)
		}
	}
	state, err := db.IssueWatchState(ctx)
	if err != nil || !strings.Contains(state.Records[issueA], `"state":"ACTIVE"`) || !strings.Contains(state.Records[issueA], "12:30:00.5Z") {
		t.Errorf("record after a stale read: %v %v", state.Records[issueA], err)
	}
}

// TestIssuesWatchTwoWatchersNeverClobber: two watchers on one store file,
// the first paused mid-poll while the second commits. Each of the paused
// watcher's writes was based on a record the second watcher has since
// replaced, so none applies: its forget (a 403 re-read) does not delete the
// newer record, and its keep (a list read from before the second watcher
// forgot the issue) does not bring the issue back with a false created event,
// and its insert of an issue it had not seen does not overwrite the newer
// record the second watcher stored first.
func TestIssuesWatchTwoWatchersNeverClobber(t *testing.T) {
	api, dbPath := newWatchFixture(t, "fixture-token-watch")
	c := client.New(&config.Config{BaseURL: os.Getenv("SERVOSITY_MSP_BASE_URL"), ServosityMspToken: "fixture-token-watch"}, 5*time.Second, 0)
	ctx := context.Background()
	const list = "/resellers/7/issues/"
	open := func() *store.Store {
		s, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	w1, w2 := open(), open()
	poll := func(db *store.Store) issueWatchResult {
		t.Helper()
		res, err := pollIssues(ctx, c, db, list)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	// paused runs watcher 1 until its request for path arrives, runs between
	// while it waits, then lets it finish.
	paused := func(path string, between func()) issueWatchResult {
		t.Helper()
		arrived, release := api.pauseOnce(path)
		done := make(chan issueWatchResult, 1)
		fail := make(chan error, 1)
		go func() {
			res, err := pollIssues(ctx, c, w1, list)
			if err != nil {
				fail <- err
				return
			}
			done <- res
		}()
		<-arrived
		between()
		release()
		select {
		case res := <-done:
			return res
		case err := <-fail:
			t.Fatal(err)
		}
		return issueWatchResult{}
	}
	recorded := func() map[string]string {
		t.Helper()
		rec, err := w2.IssueWatchState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rec.Records
	}
	t1, t2, t3 := "2026-10-27T10:00:00Z", "2026-10-27T11:00:00Z", "2026-10-27T12:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "WARNING", t1), fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
	poll(w2) // baseline

	// A leaves the list and watcher 1's re-read of it answers 403 (servosity-api
	// IssueViewSet.retrieve, ownership moved). Before that answer lands,
	// ownership comes back and watcher 2 records A escalated.
	api.set([]map[string]any{fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
	api.setStatus(map[string]int{issueA: http.StatusForbidden})
	res := paused("/issues/"+issueA+"/", func() {
		api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "DANGER", t2), fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
		if res := poll(w2); res.Events != 1 {
			t.Fatalf("watcher 2: %+v", res)
		}
	})
	if res.Stale == 0 || res.Events != 0 {
		t.Errorf("watcher 1 after the pause: %+v", res)
	}
	if !strings.Contains(recorded()[issueA], "DANGER") {
		t.Fatalf("watcher 1's stale forget deleted the newer record: %v", recorded())
	}
	if res := poll(w2); res.Events != 0 {
		t.Fatalf("the poll after the stale forget: %+v", res)
	}

	// C leaves the list and watcher 1 pauses on its re-read, holding a list
	// read that still has A. Meanwhile A's re-read answers 403 and C was
	// archived, and watcher 2 records both.
	api.setStatus(nil)
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "DANGER", t2)},
		map[string]map[string]any{issueC: fixtureIssue(issueC, "ARCHIVED", "WARNING", t3)})
	res = paused("/issues/"+issueC+"/", func() {
		api.set(nil, map[string]map[string]any{issueC: fixtureIssue(issueC, "ARCHIVED", "WARNING", t3)})
		api.setStatus(map[string]int{issueA: http.StatusForbidden})
		if res := poll(w2); res.Events != 1 {
			t.Fatalf("watcher 2: %+v", res)
		}
	})
	if res.Stale == 0 || res.Events != 0 {
		t.Errorf("watcher 1 after the pause: %+v", res)
	}
	if rec := recorded(); rec[issueA] != "" || !strings.Contains(rec[issueC], "ARCHIVED") {
		t.Fatalf("watcher 1's stale keeps overwrote the newer records: %v", rec)
	}

	// A is back and B and D are new. Watcher 1 pauses after the first page of
	// the list (B, D) while watcher 2 reads B escalated and records all three.
	t4 := "2026-10-27T13:00:00Z"
	api.setStatus(nil)
	api.set([]map[string]any{fixtureIssue(issueB, "ACTIVE", "WARNING", t3), fixtureIssue(issueD, "ACTIVE", "WARNING", t3), fixtureIssue(issueA, "ACTIVE", "DANGER", t2)}, nil)
	res = paused(list+"?cursor=p2", func() {
		api.set([]map[string]any{fixtureIssue(issueB, "ACTIVE", "DANGER", t4), fixtureIssue(issueD, "ACTIVE", "WARNING", t3), fixtureIssue(issueA, "ACTIVE", "DANGER", t2)}, nil)
		if res := poll(w2); res.Events != 3 {
			t.Fatalf("watcher 2: %+v", res)
		}
	})
	if res.Stale == 0 || res.Events != 0 {
		t.Errorf("watcher 1 after the pause: %+v", res)
	}
	if !strings.Contains(recorded()[issueB], "DANGER") {
		t.Fatalf("watcher 1's stale insert overwrote the newer record: %v", recorded()[issueB])
	}
	if res := poll(w2); res.Events != 0 {
		t.Fatalf("the poll after the stale insert: %+v", res)
	}

	want := []string{"servosity.issue.updated " + issueA, "servosity.issue.archived " + issueC,
		"servosity.issue.created " + issueB, "servosity.issue.created " + issueD, "servosity.issue.created " + issueA}
	if g := eventNames(listEvents(t, dbPath, "--since", "0")); strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events =\n%s\nwant\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
}

// TestIssuesWatchStaleReadNeverResurrectsAForgottenIssue: watcher 1 reads the
// empty store and the first list page (A, B), then pauses. Watcher 2 records
// the baseline and then forgets A (its re-read answers 403, servosity-api
// IssueViewSet.retrieve). The stored row for A is absent again, as it was when
// watcher 1 read the store, so a content compare-and-swap would let watcher 1
// insert A with a false created event. Nothing of watcher 1's poll applies.
func TestIssuesWatchStaleReadNeverResurrectsAForgottenIssue(t *testing.T) {
	api, dbPath := newWatchFixture(t, "fixture-token-watch")
	c := client.New(&config.Config{BaseURL: os.Getenv("SERVOSITY_MSP_BASE_URL"), ServosityMspToken: "fixture-token-watch"}, 5*time.Second, 0)
	ctx := context.Background()
	const list = "/resellers/7/issues/"
	open := func() *store.Store {
		s, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	w1, w2 := open(), open()
	t1 := "2026-10-27T10:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "WARNING", t1), fixtureIssue(issueB, "ACTIVE", "WARNING", t1), fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
	arrived, release := api.pauseOnce(list + "?cursor=p2")
	done := make(chan issueWatchResult, 1)
	fail := make(chan error, 1)
	go func() {
		res, err := pollIssues(ctx, c, w1, list)
		if err != nil {
			fail <- err
			return
		}
		done <- res
	}()
	<-arrived
	if res, err := pollIssues(ctx, c, w2, list); err != nil || !res.Baseline {
		t.Fatalf("watcher 2 baseline: %+v %v", res, err)
	}
	api.set([]map[string]any{fixtureIssue(issueB, "ACTIVE", "WARNING", t1), fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
	api.setStatus(map[string]int{issueA: http.StatusForbidden})
	if _, err := pollIssues(ctx, c, w2, list); err != nil {
		t.Fatal(err)
	}
	release()
	var res issueWatchResult
	select {
	case res = <-done:
	case err := <-fail:
		t.Fatal(err)
	}
	if res.Events != 0 || res.Stale == 0 {
		t.Errorf("watcher 1 after the pause: %+v", res)
	}
	rec, err := w2.IssueWatchState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.Records[issueA]; ok {
		t.Errorf("watcher 1's stale insert brought back the forgotten issue: %v", rec.Records[issueA])
	}
	if g := eventNames(listEvents(t, dbPath, "--since", "0")); len(g) != 0 {
		t.Errorf("events = %v, want none", g)
	}
}

// TestIssuesWatchBaselineNeedsTheWholeList: a first poll whose pages loop
// records nothing, so a later complete poll is the baseline, not a flood of
// created events.
func TestIssuesWatchBaselineNeedsTheWholeList(t *testing.T) {
	api, db := newWatchFixture(t, "fixture-token-watch")
	t1 := "2026-10-27T10:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "WARNING", t1), fixtureIssue(issueB, "ACTIVE", "WARNING", t1), fixtureIssue(issueC, "ACTIVE", "WARNING", t1)}, nil)
	api.badNext = true
	if _, _, err := runCLI("issues", "watch", "--once", "--json", "--db", db); err == nil || !strings.Contains(err.Error(), "no baseline was recorded") {
		t.Fatalf("an incomplete first poll: %v", err)
	}
	api.badNext = false
	if got := watchOnce(t, db); got["baseline"] != true || got["events"] != float64(0) {
		t.Fatalf("complete poll after it: %v", got)
	}
}

// TestIssuesWatchRereadFailures: a re-read that answers 403 or 404 forgets
// the issue (gone, or no longer the token's), and any other failure fails
// the poll after the list's changes are recorded.
func TestIssuesWatchRereadFailures(t *testing.T) {
	api, db := newWatchFixture(t, "fixture-token-watch")
	t1, t2 := "2026-10-27T10:00:00Z", "2026-10-27T11:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "WARNING", t1), fixtureIssue(issueB, "ACTIVE", "WARNING", t1)}, nil)
	watchOnce(t, db)
	api.set([]map[string]any{fixtureIssue(issueD, "ACTIVE", "WARNING", t2)}, nil)
	api.status = map[string]int{issueA: http.StatusForbidden, issueB: http.StatusBadRequest}
	if _, _, err := runCLI("issues", "watch", "--once", "--json", "--db", db); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("a failed re-read did not fail the poll: %v", err)
	}
	if l := listEvents(t, db, "--since", "0"); len(l.Events) != 1 || l.Events[0].Name != issueEventCreated {
		t.Errorf("the list's changes were not kept: %+v", l)
	}
	state, _ := store.OpenReadOnly(db)
	defer state.Close()
	rec, err := state.IssueWatchState(context.Background())
	if _, ok := rec.Records[issueA]; ok || err != nil {
		t.Errorf("a 403 issue is still recorded (%v)", err)
	}
	if _, ok := rec.Records[issueB]; !ok {
		t.Error("a 400 issue was forgotten")
	}
}

// TestIssuesWatchKeepsOnlyAllowlistedFields: only the triage shape is kept, so
// text an API put in any other field reaches no output, event or store file.
// That a kept field repeating a held credential is withheld is proven on the
// built binaries (internal/credwire), where the credential kit runs.
func TestIssuesWatchKeepsOnlyAllowlistedFields(t *testing.T) {
	const marker = "watch-sentinel-free-text-7f3a9c2e"
	api, db := newWatchFixture(t, "fixture-token-watch")
	t1, t2 := "2026-10-27T10:00:00Z", "2026-10-27T11:00:00Z"
	api.set([]map[string]any{fixtureIssue(issueA, "ACTIVE", "WARNING", t1)}, nil)
	var shown []string
	run := func(args ...string) {
		out, errOut, err := runCLI(args...)
		shown = append(shown, out, errOut)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	run("issues", "watch", "--once", "--json", "--db", db)
	changed := fixtureIssue(issueA, "RESOLVED", "WARNING", t2)
	changed["audience"], changed["description"] = marker, marker
	api.set([]map[string]any{changed}, nil)
	run("issues", "watch", "--once", "--json", "--db", db)
	run("events", "list", "--since", "0", "--json", "--db", db)
	for i, out := range shown {
		if strings.Contains(out, marker) {
			t.Errorf("output %d shows the free text: %s", i, out)
		}
	}
	if l := listEvents(t, db, "--since", "0"); len(l.Events) != 1 {
		t.Errorf("want the one change, got %+v", l)
	}
	raw, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(marker)) {
		t.Error("the store file holds the free text")
	}
}

// TestIssueTransitions pins the classification of every change.
func TestIssueTransitions(t *testing.T) {
	base := watchIssue{ID: issueA, State: "ACTIVE", Severity: "WARNING", Priority: 3000, UpdatedAt: "t1", LastSeenAt: "t1"}
	with := func(f func(*watchIssue)) watchIssue { w := base; f(&w); return w }
	for _, tc := range []struct {
		name      string
		prev, cur watchIssue
		want      string
	}{
		{"archived", base, with(func(w *watchIssue) { w.State = "ARCHIVED"; w.UpdatedAt = "t2" }), issueEventArchived},
		{"ignored", base, with(func(w *watchIssue) { w.State = "IGNORED" }), issueEventIgnored},
		{"reactivated from archived", with(func(w *watchIssue) { w.State = "ARCHIVED" }), base, issueEventReactivated},
		{"reactivated from ignored", with(func(w *watchIssue) { w.State = "IGNORED" }), base, issueEventReactivated},
		{"severity", base, with(func(w *watchIssue) { w.Severity = "DANGER" }), issueEventUpdated},
		{"priority", base, with(func(w *watchIssue) { w.Priority = 1000 }), issueEventUpdated},
		{"updated_at", base, with(func(w *watchIssue) { w.UpdatedAt = "t2" }), issueEventUpdated},
		{"last_seen_at", base, with(func(w *watchIssue) { w.LastSeenAt = "t2" }), issueEventUpdated},
		{"archived to ignored", with(func(w *watchIssue) { w.State = "ARCHIVED" }), with(func(w *watchIssue) { w.State = "IGNORED" }), issueEventIgnored},
		{"no change", base, base, ""},
		{"company only", base, with(func(w *watchIssue) { w.Company = 9 }), ""},
	} {
		got, _ := issueTransition(tc.prev, tc.cur)
		if got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestIssueWatchLoopBacksOff: a failing poll waits longer each time up to a
// cap, success returns to the interval, and an auth failure stops the watch.
func TestIssueWatchLoopBacksOff(t *testing.T) {
	results := []error{errors.New("boom"), errors.New("boom"), nil, nil,
		&client.APIError{Method: "GET", Path: "/resellers/7/issues/", StatusCode: 403}}
	calls := 0
	var waits []time.Duration
	var errOut bytes.Buffer
	err := runIssueWatchLoop(context.Background(), time.Minute, &errOut,
		func(context.Context) (issueWatchResult, error) {
			err := results[calls]
			calls++
			return issueWatchResult{Issues: 1}, err
		},
		func(_ context.Context, d time.Duration) bool { waits = append(waits, d); return true })
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || calls != len(results) {
		t.Fatalf("loop ended with %v after %d polls", err, calls)
	}
	want := []time.Duration{2 * time.Minute, 4 * time.Minute, time.Minute, time.Minute}
	if fmt.Sprint(waits) != fmt.Sprint(want) {
		t.Errorf("waits %v, want %v", waits, want)
	}
	if !strings.Contains(errOut.String(), "retrying in 2m0s") {
		t.Errorf("status lines: %s", errOut.String())
	}
	if d := issueWatchBackoff(5*time.Minute, 30); d != time.Hour {
		t.Errorf("backoff cap: %v", d)
	}
}

// TestIssuesWatchRefusesUnsafeRuns: --dry-run sends nothing, a short
// --interval is refused, and --data-source local is refused.
func TestIssuesWatchRefusesUnsafeRuns(t *testing.T) {
	api, db := newWatchFixture(t, "fixture-token-watch")
	if _, _, err := runCLI("issues", "watch", "--once", "--dry-run", "--db", db); err != nil {
		t.Fatalf("--dry-run: %v", err)
	}
	if _, _, err := runCLI("issues", "watch", "--interval", "5s", "--db", db); err == nil {
		t.Error("a 5s interval was accepted")
	}
	if _, _, err := runCLI("issues", "watch", "--once", "--data-source", "local", "--db", db); err == nil {
		t.Error("--data-source local was accepted")
	}
	if n := len(api.seen()); n != 0 {
		t.Errorf("%d requests reached the API", n)
	}
}
