// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"autotask-pp-cli/internal/cliutil/testenv"
)

// pagedQueryClient answers POST /Tickets/query from tickets 1..last the way Autotask does: the
// filter's id gte/gt conditions select, maxRecords caps the page. GET answers a single ticket.
type pagedQueryClient struct {
	last    int
	bodies  []string
	gets    []string
	updated map[int]string // id -> a later title and modification time
}

func (f *pagedQueryClient) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	f.gets = append(f.gets, path)
	return json.RawMessage(`{"item":{"id":7,"title":"T7"}}`), nil
}

func (f *pagedQueryClient) PostWithParams(ctx context.Context, path string, q map[string]string, body any) (json.RawMessage, int, error) {
	return f.PostQueryWithParams(ctx, path, q, body)
}

func (f *pagedQueryClient) PostQueryWithParams(_ context.Context, path string, _ map[string]string, body any) (json.RawMessage, int, error) {
	raw, _ := json.Marshal(body)
	f.bodies = append(f.bodies, path+" "+string(raw))
	var q struct {
		Filter []struct {
			Op    string  `json:"op"`
			Value float64 `json:"value"`
		} `json:"filter"`
		MaxRecords int `json:"maxRecords"`
	}
	_ = json.Unmarshal(raw, &q)
	from := 1
	for _, c := range q.Filter {
		if c.Op == "gte" && int(c.Value) > from {
			from = int(c.Value)
		}
		if c.Op == "gt" && int(c.Value)+1 > from {
			from = int(c.Value) + 1
		}
	}
	var items []string
	for id := from; id <= f.last && len(items) < q.MaxRecords; id++ {
		title, modified := fmt.Sprintf("T%d", id), "2026-10-28T00:00:00Z"
		if t, ok := f.updated[id]; ok {
			title, modified = t, "2026-10-28T09:30:00.25Z"
		}
		items = append(items, fmt.Sprintf(`{"id":%d,"title":"%s","lastTrackedModificationDateTime":"%s"}`, id, title, modified))
	}
	return json.RawMessage(`{"items":[` + strings.Join(items, ",") + `],"pageDetails":{"count":` + fmt.Sprint(len(items)) + `}}`), 200, nil
}

// Handfix export-tail-read-records: every readable resource reads its records through the
// id-walked query, and the generated pager follows the walk to the last page.
func TestExportReadsRecordsAcrossIDWalkPages(t *testing.T) {
	for r, p := range resourceReadPaths {
		if !strings.HasSuffix(p, "/query") {
			t.Errorf("%s reads %s, not its record query", r, p)
		}
	}
	path, err := resourceReadPath("tickets")
	if err != nil {
		t.Fatal(err)
	}
	config := resourceReadConfigFor("tickets")
	c := &pagedQueryClient{last: 101}
	count, cursor := 0, ""
	for page := 0; page < 5; page++ {
		data, err := readResourcePage(context.Background(), c, path, resourcePageParams(config, cursor, page, 0))
		if err != nil {
			t.Fatal(err)
		}
		items, next, hasMore := extractResourcePage(data, config)
		count += len(items)
		var stop bool
		if cursor, stop, err = resourceNextCursor(config, cursor, next, hasMore); err != nil || stop {
			break
		}
	}
	if count != 101 || len(c.bodies) != 2 {
		t.Fatalf("want 101 tickets over 2 queries, got %d over %v", count, c.bodies)
	}
	if !strings.Contains(c.bodies[0], `"op":"gte"`) || !strings.Contains(c.bodies[1], `"op":"gt","value":100`) || strings.Contains(c.bodies[1], resourceIDCursorParam) {
		t.Errorf("want a seeded first query and an id-after-100 second query with no cursor key, got %v", c.bodies)
	}
}

// Codex r1 class incomplete-paging-read-as-complete: tail emits every record on its first poll
// (tickets 1..101 span two pages), then only the records created since; a single-record export
// path is read with GET.
func TestTailEmitsEveryPageThenOnlyNewRecords(t *testing.T) {
	freshTailStates(t)
	path, err := resourceReadPath("tickets")
	if err != nil {
		t.Fatal(err)
	}
	config := resourceReadConfigFor("tickets")
	c := &pagedQueryClient{last: 101}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	count := func() int { n := strings.Count(out.String(), `"event":"data"`); out.Reset(); return n }
	if err := fetchAndEmit(context.Background(), c, path, config, enc); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 101 {
		t.Fatalf("first poll emitted %d of 101 tickets (queries %v)", n, c.bodies)
	}
	c.last = 103
	if err := fetchAndEmit(context.Background(), c, path, config, enc); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Fatalf("second poll emitted %d records, want the 2 created since", n)
	}
	if _, err := readResourcePage(context.Background(), c, "/Tickets/7", nil); err != nil || len(c.gets) != 1 {
		t.Fatalf("a single-record path was not read with GET (err %v, gets %v)", err, c.gets)
	}
}

// Codex r2 class documented updates omitted: a later poll asked only for ids
// above the highest emitted, so a ticket completed after the first poll was
// never emitted again. Each later poll emits every record whose content
// changed: tickets are asked for by their documented modification time
// (TicketModel.lastTrackedModificationDateTime, plus any higher id), and a
// record that did not change is not emitted twice.
func TestTailEmitsUpdatedRecords(t *testing.T) {
	freshTailStates(t)
	path, err := resourceReadPath("tickets")
	if err != nil {
		t.Fatal(err)
	}
	config := resourceReadConfigFor("tickets")
	c := &pagedQueryClient{last: 101}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	if err := fetchAndEmit(context.Background(), c, path, config, enc); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	c.bodies = nil
	c.updated = map[int]string{1: "T1 completed"}
	if err := fetchAndEmit(context.Background(), c, path, config, enc); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), `"event":"data"`); n != 1 || !strings.Contains(out.String(), "T1 completed") {
		t.Fatalf("second poll emitted %d records, want only the updated ticket 1:\n%s", n, out.String())
	}
	if !strings.Contains(strings.Join(c.bodies, "\n"), `"field":"lastTrackedModificationDateTime","op":"gte","value":"2026-10-28T00:00:00Z"`) {
		t.Errorf("second poll did not ask for records modified since the first: %v", c.bodies)
	}
	out.Reset()
	if err := fetchAndEmit(context.Background(), c, path, config, enc); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), `"event":"data"`); n != 0 {
		t.Fatalf("third poll re-emitted %d unchanged records", n)
	}
}

// TestTailChangeFieldIsDocumented: every tailChangeField entry is a date-time
// property of the model the resource's query returns, and every tailed
// resource whose model documents a modification time is in the table.
func TestTailChangeFieldIsDocumented(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if os.IsNotExist(err) {
		// The spec source is not public (press/publication.json), so the
		// monorepo carries no spec.json; the press tree that has it runs this.
		t.Skip("spec.json is not vendored into this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"responses"`
		} `json:"paths"`
		Definitions map[string]struct {
			Properties map[string]struct {
				Format string `json:"format"`
			} `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	modified := regexp.MustCompile(`^(lastTrackedModifi|lastModified|updateDateTime)`)
	for _, resource := range tailKnownResources() {
		path, err := syncResourcePath(resource)
		if err != nil {
			continue
		}
		m := regexp.MustCompile(`\[(\w+),`).FindStringSubmatch(spec.Paths[path]["post"].Responses["200"].Schema.Ref)
		if m == nil {
			continue
		}
		props := spec.Definitions[m[1]].Properties
		if f, ok := tailChangeField[resource]; ok {
			if props[f].Format != "date-time" || !modified.MatchString(f) {
				t.Errorf("tailChangeField[%q] = %q is not a modification date-time of %s", resource, f, m[1])
			}
			continue
		}
		for p, d := range props {
			if modified.MatchString(p) && d.Format == "date-time" {
				t.Errorf("%s documents modification time %s; add it to tailChangeField", resource, p)
			}
		}
	}
}

// tailFake is Autotask's POST /Tickets/query (spec.json /Tickets/query,
// QueryActionResult[TicketModel,Ticket]) for tail: every filter condition
// selects (id as a number, lastTrackedModificationDateTime as the RFC 3339
// text Autotask returns, which at one precision orders as the times do; a
// ticket with none matches no condition on it), maxRecords caps the page, and
// tickets come in id order. A ticket's title carries its modification time,
// so an update changes the record. deny answers a query HTTP 401 while it
// returns true; before runs as each query arrives.
type tailFake struct {
	mu      sync.Mutex
	mod     map[int]string // ticket id -> lastTrackedModificationDateTime ("" = none)
	deny    func(conds []map[string]any) bool
	before  func(conds []map[string]any)
	queries []string
}

func (f *tailFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		Filter     []map[string]any `json:"filter"`
		MaxRecords int              `json:"maxRecords"`
	}
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, string(raw))
	if f.before != nil {
		f.before(body.Filter)
	}
	if f.deny != nil && f.deny(body.Filter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":["The request is not authorized."]}`))
		return
	}
	ids := make([]int, 0, len(f.mod))
	for id := range f.mod {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	items := []string{}
	for _, id := range ids {
		if len(items) == body.MaxRecords {
			break
		}
		if tailFakeMatches(id, f.mod[id], body.Filter) {
			items = append(items, tailFakeTicket(id, f.mod[id]))
		}
	}
	_, _ = fmt.Fprintf(w, `{"items":[%s],"pageDetails":{"count":%d,"requestCount":%d,"prevPageUrl":null,"nextPageUrl":null}}`, strings.Join(items, ","), len(items), body.MaxRecords)
}

func tailFakeTicket(id int, mod string) string {
	at, title := "null", fmt.Sprintf("T%d as of none", id)
	if mod != "" {
		at, title = `"`+mod+`"`, fmt.Sprintf("T%d as of %s", id, mod)
	}
	return fmt.Sprintf(`{"id":%d,"title":%q,"lastTrackedModificationDateTime":%s}`, id, title, at)
}

func tailFakeMatches(id int, mod string, conds []map[string]any) bool {
	for _, c := range conds {
		var cmp int
		switch v := c["value"].(type) {
		case float64:
			if !strings.EqualFold(fmt.Sprint(c["field"]), "id") {
				return false
			}
			if n := float64(id); n > v {
				cmp = 1
			} else if n < v {
				cmp = -1
			}
		case string:
			if !strings.EqualFold(fmt.Sprint(c["field"]), "lastTrackedModificationDateTime") || mod == "" {
				return false
			}
			cmp = strings.Compare(mod, v)
		default:
			return false
		}
		switch c["op"] {
		case "gt":
			if cmp <= 0 {
				return false
			}
		case "gte":
			if cmp < 0 {
				return false
			}
		case "eq":
			if cmp != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// tailFakeAfter is the id a query's page starts after (its "gt" on id), and
// tailFakeModified whether it asks for records modified since a time.
func tailFakeAfter(conds []map[string]any) float64 {
	for _, c := range conds {
		if v, ok := c["value"].(float64); ok && c["op"] == "gt" && strings.EqualFold(fmt.Sprint(c["field"]), "id") {
			return v
		}
	}
	return -1
}

func tailFakeModified(conds []map[string]any) bool {
	for _, c := range conds {
		if strings.EqualFold(fmt.Sprint(c["field"]), "lastTrackedModificationDateTime") {
			return true
		}
	}
	return false
}

// denyOnce is true for the first query pred matches, then false.
func denyOnce(pred func(conds []map[string]any) bool) func(conds []map[string]any) bool {
	done := false
	return func(conds []map[string]any) bool {
		if done || !pred(conds) {
			return false
		}
		done = true
		return true
	}
}

// freshTailStates starts a new tail process for the test: tailStates lives as
// long as the process, and every test here tails the same path.
func freshTailStates(t *testing.T) {
	t.Helper()
	saved := tailStates
	tailStates = map[string]*tailState{}
	t.Cleanup(func() { tailStates = saved })
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

// tailProcess is one `tail tickets` process against a fresh fake through the
// real client: each poll is what every --interval tick runs (fetchAndEmit),
// and returns the titles it emitted and its error. failEmit makes the poll's
// writes fail.
func tailProcess(t *testing.T) (*tailFake, func(failEmit bool) ([]string, error)) {
	t.Helper()
	f := &tailFake{mod: map[int]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("AUTOTASK_BASE_URL", srv.URL+"/atservicesrest/V1.0")
	t.Setenv("AUTOTASK_API_INTEGRATION_CODE", "tailCode-7d1e3a5c")
	t.Setenv("AUTOTASK_PSA_USER_NAME", "tail-user@example.com")
	t.Setenv("AUTOTASK_PSA_SECRET", "tailSecret-4b6d8f0a")
	c, err := (&rootFlags{timeout: 10 * time.Second}).newClient()
	if err != nil {
		t.Fatal(err)
	}
	c.NoCache = true // as tail sets it
	freshTailStates(t)
	path, err := resourceReadPath("tickets")
	if err != nil {
		t.Fatal(err)
	}
	config := resourceReadConfigFor("tickets")
	return f, func(failEmit bool) ([]string, error) {
		var out bytes.Buffer
		var w io.Writer = &out
		if failEmit {
			w = failingWriter{}
		}
		err := fetchAndEmit(context.Background(), c, path, config, json.NewEncoder(w))
		var titles []string
		for dec := json.NewDecoder(&out); ; {
			var ev struct {
				Data struct {
					Title string `json:"title"`
				} `json:"data"`
			}
			if dec.Decode(&ev) != nil {
				return titles, err
			}
			titles = append(titles, ev.Data.Title)
		}
	}
}

// Codex r1 class incomplete-read-advances-cursor (handfix
// r1k-tail-cursor-advances-only-on-a-complete-poll): tail moved its cursors
// (the newest modification time, the highest id, the emitted-record hashes)
// page by page, so a poll that stopped part-way left them past records it
// never read, and every later poll skipped those. The cursors move only when
// a poll reads every page of every pass; a poll that fails reads the same
// window again next time, and a record it already emitted is not emitted
// twice.
func TestTailCursorsMoveOnlyOnACompletePoll(t *testing.T) {
	const base = "2026-11-04T00:00:00Z"
	at := func(hhmm string) string { return "2026-11-04T" + hhmm + ":00Z" }
	firstPoll := func(t *testing.T, f *tailFake, poll func(bool) ([]string, error)) {
		t.Helper()
		f.mu.Lock()
		for id := 1; id <= 101; id++ {
			f.mod[id] = base
		}
		f.mu.Unlock()
		if got, err := poll(false); err != nil || len(got) != 101 {
			t.Fatalf("first poll: emitted %d of 101 tickets, err %v", len(got), err)
		}
	}
	want := func(t *testing.T, name string, got []string, err error, wantErr bool, titles ...string) {
		t.Helper()
		if (err != nil) != wantErr {
			t.Fatalf("%s: err %v, want an error: %v", name, err, wantErr)
		}
		if strings.Join(got, "\n") != strings.Join(titles, "\n") {
			t.Fatalf("%s emitted\n  %q\nwant\n  %q", name, got, titles)
		}
	}

	// The Codex input: ticket 1 is modified at 00:02 and ticket 101 at 00:01;
	// the update pass reads tickets 1 to 100 and its second page answers 401.
	// Page one held the 00:02 watermark, so every later poll asked only for
	// records modified since 00:02 and ticket 101's update was never emitted.
	t.Run("update pass cut off by a 401", func(t *testing.T) {
		f, poll := tailProcess(t)
		firstPoll(t, f, poll)
		f.mu.Lock()
		f.mod[1], f.mod[101] = at("00:02"), at("00:01")
		f.deny = denyOnce(func(conds []map[string]any) bool { return tailFakeModified(conds) && tailFakeAfter(conds) >= 100 })
		f.mu.Unlock()
		got, err := poll(false)
		want(t, "the cut-off poll", got, err, true, "T1 as of "+at("00:02"))
		got, err = poll(false)
		want(t, "the next poll", got, err, false, "T101 as of "+at("00:01"))
		f.mu.Lock()
		f.queries = nil
		f.mu.Unlock()
		got, err = poll(false)
		want(t, "a poll after a complete one", got, err, false)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !strings.Contains(strings.Join(f.queries, "\n"), `"value":"`+at("00:02")+`"`) {
			t.Errorf("a complete poll did not move the watermark to 00:02: %v", f.queries)
		}
	})

	// The new-records pass reads tickets 102 to 201 (modified at 00:10) and
	// its second page answers 401, before the modified-since pass runs. Page
	// one held the 00:10 watermark, so ticket 50's 00:05 update was skipped.
	t.Run("new-records pass cut off by a 401", func(t *testing.T) {
		f, poll := tailProcess(t)
		firstPoll(t, f, poll)
		f.mu.Lock()
		f.mod[50] = at("00:05")
		for id := 102; id <= 202; id++ {
			f.mod[id] = at("00:10")
		}
		f.deny = denyOnce(func(conds []map[string]any) bool { return !tailFakeModified(conds) && tailFakeAfter(conds) >= 201 })
		f.mu.Unlock()
		got, err := poll(false)
		if err == nil || len(got) != 100 {
			t.Fatalf("the cut-off poll: emitted %d, err %v; want tickets 102 to 201 and the 401", len(got), err)
		}
		got, err = poll(false)
		want(t, "the next poll", got, err, false, "T202 as of "+at("00:10"), "T50 as of "+at("00:05"))
	})

	// A ticket created between the two passes with no modification time yet
	// (102) is in neither; the modified-since pass reads one created beside
	// it (103). Its id is not where an id walk stopped, so the highest id
	// moves only on the walk by id, and the next poll reads 102.
	t.Run("the highest id moves only on the walk by id", func(t *testing.T) {
		f, poll := tailProcess(t)
		firstPoll(t, f, poll)
		f.mu.Lock()
		f.before = func(conds []map[string]any) {
			if tailFakeModified(conds) {
				if _, ok := f.mod[102]; !ok {
					f.mod[102], f.mod[103] = "", at("00:03")
				}
			}
		}
		f.mu.Unlock()
		got, err := poll(false)
		want(t, "the poll the tickets were created in", got, err, false, "T103 as of "+at("00:03"))
		got, err = poll(false)
		want(t, "the next poll", got, err, false, "T102 as of none")
	})

	// A poll whose writes fail emitted nothing; the next poll emits it.
	t.Run("a poll whose emit fails", func(t *testing.T) {
		f, poll := tailProcess(t)
		firstPoll(t, f, poll)
		f.mu.Lock()
		f.mod[1] = at("00:02")
		f.mu.Unlock()
		if _, err := poll(true); err == nil {
			t.Fatal("a poll whose writes fail returned no error")
		}
		got, err := poll(false)
		want(t, "the next poll", got, err, false, "T1 as of "+at("00:02"))
	})
}
