// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"servosity-msp-pp-cli/internal/store"
)

// The Servosity API pages lists the Django REST Framework way: count, next (a
// full URL or null), previous, results, and 404 {"detail":"Invalid page."}
// past the last page. A page-number list once came back as page 1 only under
// --all, with exit 0 and the count stripped. Every row below is synthetic.
const (
	drfRows    = 27
	drfPerPage = 10
)

type drfMode int

const (
	drfHonest   drfMode = iota
	drfPastEnd          // page 1's next skips to a page that does not exist
	drfCutShort         // page 2's next is null while count still says 27
)

type drfServer struct {
	*httptest.Server
	mode     drfMode
	mu       sync.Mutex
	hits     []string
	pastEnds int
}

func newDRFServer(t *testing.T, mode drfMode) *drfServer {
	t.Helper()
	s := &drfServer{mode: mode}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	t.Setenv("SERVOSITY_MSP_BASE_URL", s.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")
	return s
}

func (s *drfServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hits = append(s.hits, r.URL.RequestURI())
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/latest-success/") {
		_, _ = w.Write([]byte(`{"date":"2026-01-01T00:00:00Z"}`))
		return
	}
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		page, _ = strconv.Atoi(v)
	}
	per := drfPerPage
	if v, err := strconv.Atoi(q.Get("page_size")); err == nil && v > 0 {
		per = v
	}
	pages := (drfRows + per - 1) / per
	if page < 1 || page > pages {
		s.mu.Lock()
		s.pastEnds++
		s.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Invalid page."}`))
		return
	}
	base := "http://" + r.Host
	rows := []map[string]any{}
	for i := (page-1)*per + 1; i <= page*per && i <= drfRows; i++ {
		rows = append(rows, map[string]any{
			"id":          i,
			"url":         fmt.Sprintf("%s%s%d/", base, r.URL.Path, i),
			"company":     base + "/companies/1/",
			"reseller":    base + "/resellers/7/",
			"name":        fmt.Sprintf("synthetic-%d", i),
			"device_name": fmt.Sprintf("host-%d", i),
			"hostname":    fmt.Sprintf("host-%d", i),
			"status":      "open",
			"state":       "active",
		})
	}
	if strings.Contains(r.URL.Path, "/issues/") {
		// Issues name their company by id, not by URL.
		for _, row := range rows {
			row["company"] = 1
		}
	}
	nextPage := page + 1
	switch {
	case s.mode == drfPastEnd && page == 1:
		nextPage = pages + 1
	case s.mode == drfCutShort && page == 2:
		nextPage = 0
	case page >= pages:
		nextPage = 0
	}
	var next any
	if nextPage > 0 {
		nq := r.URL.Query()
		nq.Set("page", strconv.Itoa(nextPage))
		next = base + r.URL.Path + "?" + nq.Encode()
	}
	var previous any
	if page > 1 {
		pq := r.URL.Query()
		pq.Set("page", strconv.Itoa(page-1))
		previous = base + r.URL.Path + "?" + pq.Encode()
	}
	body, _ := json.Marshal(map[string]any{
		"count": drfRows, "num_pages": pages, "next": next, "previous": previous, "results": rows,
	})
	_, _ = w.Write(body)
}

func (s *drfServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hits...)
}

// runDRF executes one command through the root command and returns stdout and
// stderr. Pagination events go to os.Stderr directly, so that is captured too.
func runDRF(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	var piped bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&piped, r); close(done) }()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	runErr := cmd.Execute()
	os.Stderr = saved
	_ = w.Close()
	<-done
	return out.String(), errb.String() + piped.String(), runErr
}

// drfListRows counts the rows a list command printed and returns the count it
// kept, or -1 when the output carries none.
func drfListRows(t *testing.T, stdout string) (int, int) {
	t.Helper()
	var arr []json.RawMessage
	if json.Unmarshal([]byte(stdout), &arr) == nil {
		return len(arr), -1
	}
	var env struct {
		Results []json.RawMessage `json:"results"`
		Meta    struct {
			TotalCount *int `json:"total_count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	if env.Meta.TotalCount == nil {
		return len(env.Results), -1
	}
	return len(env.Results), *env.Meta.TotalCount
}

// Every generated page-number list command in this tree, by its own name.
var drfListCommands = [][]string{
	{"backups", "list"},
	{"backups", "mfa-codes"},
	{"backup-search"},
	{"companies", "list"},
	{"companies", "fully-managed"},
	{"companies", "summary"},
	{"companies", "summary-ng"},
	{"company-notes", "list"},
	{"contracts", "list"},
	{"contracts", "get-by-token", "--token", "synthetic-token"},
	{"contracts", "signatures"},
	{"credentials", "list"},
	{"dr-backups", "list"},
	{"restic-backups", "list"},
}

func TestDRFAllFollowsEveryPage(t *testing.T) {
	for _, base := range drfListCommands {
		for _, shape := range [][]string{{"--json"}, {"--agent"}} {
			name := strings.Join(append(append([]string{}, base...), shape...), " ")
			t.Run(name, func(t *testing.T) {
				isolateNovelStore(t)
				srv := newDRFServer(t, drfHonest)
				args := append(append(append([]string{}, base...), "--all"), shape...)
				stdout, stderr, err := runDRF(t, args...)
				if err != nil {
					t.Fatalf("%s: err = %v\nstderr:\n%s", name, err, stderr)
				}
				got, count := drfListRows(t, stdout)
				if got != drfRows {
					t.Fatalf("%s --all returned %d rows, want %d\nrequests: %v\nstderr:\n%s", name, got, drfRows, srv.requests(), stderr)
				}
				if count != drfRows {
					t.Fatalf("%s --all dropped the count (meta.total_count = %d, want %d)\n%s", name, count, drfRows, stdout)
				}
				if srv.pastEnds != 0 {
					t.Fatalf("%s --all asked for a page past the last one: %v", name, srv.requests())
				}
				if strings.Contains(stderr, `"event":"truncated"`) {
					t.Fatalf("%s --all reported truncation on a complete walk:\n%s", name, stderr)
				}
			})
		}
	}
}

// The next link carries every original parameter; only the page number may
// change between requests.
func TestDRFAllKeepsFiltersAcrossPages(t *testing.T) {
	isolateNovelStore(t)
	srv := newDRFServer(t, drfHonest)
	if _, stderr, err := runDRF(t, "backups", "list", "--all", "--json", "--company", "1", "--search", "synthetic"); err != nil {
		t.Fatalf("err = %v\n%s", err, stderr)
	}
	pages := 0
	for _, hit := range srv.requests() {
		if !strings.HasPrefix(hit, "/backups/") {
			continue
		}
		pages++
		if !strings.Contains(hit, "company=1") || !strings.Contains(hit, "search=synthetic") {
			t.Fatalf("request %q lost a filter", hit)
		}
	}
	if pages != 3 {
		t.Fatalf("fetched %d pages, want 3: %v", pages, srv.requests())
	}
}

func TestDRFAllNeverTruncatesSilently(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode drfMode
		want int
	}{
		{"next points past the end", drfPastEnd, drfPerPage},
		{"run cut short", drfCutShort, 2 * drfPerPage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateNovelStore(t)
			newDRFServer(t, tc.mode)
			_, stderr, err := runDRF(t, "backups", "list", "--all", "--json")
			if err == nil {
				t.Fatalf("--all returned exit 0 on a partial walk\nstderr:\n%s", stderr)
			}
			want := fmt.Sprintf(`{"event":"truncated","fetched":%d,"count":%d}`, tc.want, drfRows)
			if !strings.Contains(stderr, want) {
				t.Fatalf("stderr lacks %s:\n%s", want, stderr)
			}
		})
	}
}

// sync has its own page loop; the same server must land every row, and the
// store-backed commands read what sync landed.
func TestDRFSyncAndStoreCommandsSeeEveryRow(t *testing.T) {
	isolateNovelStore(t)
	newDRFServer(t, drfHonest)
	if _, stderr, err := runDRF(t, "sync", "--resources", "backups,restic-backups,dr-backups,companies", "--full", "--strict", "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	db := openDRFStore(t)
	for _, resource := range []string{"backups", "restic-backups", "dr-backups", "companies"} {
		n, err := db.Count(resource)
		if err != nil {
			t.Fatal(err)
		}
		if n != drfRows {
			t.Fatalf("sync stored %d %s rows, want %d", n, resource, drfRows)
		}
	}
	db.Close()

	stdout, stderr, err := runDRF(t, "backup-facts", "--json")
	if err != nil {
		t.Fatalf("backup-facts: %v\n%s", err, stderr)
	}
	if got := countJSONRows(t, stdout); got != 3*drfRows {
		t.Fatalf("backup-facts returned %d rows, want %d (27 per engine)\n%s", got, 3*drfRows, stderr)
	}

	stdout, stderr, err = runDRF(t, "stale-backups", "--refresh", "--json")
	if err != nil {
		t.Fatalf("stale-backups: %v\n%s", err, stderr)
	}
	if got := countJSONRows(t, stdout); got != 2*drfRows {
		t.Fatalf("stale-backups returned %d rows, want %d (every restic and DR backup)\n%s", got, 2*drfRows, stderr)
	}
}

// attention --refresh walks /resellers/{id}/issues/ itself.
func TestDRFAttentionCountsEveryIssue(t *testing.T) {
	isolateNovelStore(t)
	srv := newDRFServer(t, drfHonest)
	stdout, stderr, err := runDRF(t, "attention", "--refresh", "--json")
	if err != nil {
		t.Fatalf("attention: %v\n%s", err, stderr)
	}
	var res struct {
		Totals map[string]int `json:"totals"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("attention output: %v\n%s", err, stdout)
	}
	if res.Totals["issues"] != drfRows {
		t.Fatalf("attention counted %d open issues, want %d\nrequests: %v", res.Totals["issues"], drfRows, srv.requests())
	}
}

func countJSONRows(t *testing.T, stdout string) int {
	t.Helper()
	var arr []json.RawMessage
	if json.Unmarshal([]byte(stdout), &arr) == nil {
		return len(arr)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	for _, key := range []string{"results", "data", "rows", "entries", "facts", "backups"} {
		if json.Unmarshal(env[key], &arr) == nil && arr != nil {
			return len(arr)
		}
	}
	t.Fatalf("no row array in output:\n%s", stdout)
	return 0
}

// openDRFStore opens the store the commands just wrote; the scope the run
// derived from the credential stays set process-wide.
func openDRFStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(defaultDBPath("servosity-cli"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}
