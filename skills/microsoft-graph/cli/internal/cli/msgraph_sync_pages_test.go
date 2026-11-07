// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
	"microsoft-graph-pp-cli/internal/store"
)

// microsoft-graph handfix (sync-reads-every-page), class
// incomplete-read-as-complete. The fake pages every collection the way
// Microsoft documents it (https://learn.microsoft.com/en-us/graph/paging): the
// server picks the page size whatever $top asks, a page may hold fewer items
// than were requested, and every page but the last carries an
// @odata.nextLink. numberAssignments has no nextLink: it pages with $skip
// until an empty page. Directory collections refuse $skip, as Graph does.
type pagedCollection struct {
	total     int    // items in the collection
	page      int    // items per page, whatever $top asks
	skipPaged bool   // $skip windows and no nextLink (numberAssignments)
	nextHost  string // origin the nextLink names ("" = this server)
	failAt    int    // the page starting at this item answers 503 (0 = none)
	bareAt    int    // the page starting at this item has no value array (0 = none)
	stuckAt   int    // the page starting at this item links to itself (0 = none)
	bareAt0   int    // the page starting at this item is a bare empty JSON array (0 = none)
}

type syncPagesFake struct {
	mu       sync.Mutex
	colls    map[string]*pagedCollection // by path below /v1.0
	requests []string
}

func (f *syncPagesFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	p := strings.TrimPrefix(r.URL.Path, "/v1.0")
	coll, ok := f.colls[p]
	if !ok {
		_, _ = w.Write([]byte(`{"value":[]}`))
		return
	}
	q := r.URL.Query()
	start := 0
	if coll.skipPaged {
		start, _ = strconv.Atoi(q.Get("$skip"))
	} else {
		if q.Has("$skip") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"Request_UnsupportedQuery","message":"Unsupported Query."}}`))
			return
		}
		start, _ = strconv.Atoi(q.Get("$skiptoken"))
	}
	switch {
	case coll.failAt > 0 && start == coll.failAt:
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"serviceNotAvailable","message":"try later"}}`))
		return
	case coll.bareAt > 0 && start == coll.bareAt:
		_, _ = w.Write([]byte(`{"@odata.context":"https://graph.example/v1.0/$metadata#x"}`))
		return
	case coll.bareAt0 > 0 && start == coll.bareAt0:
		_, _ = w.Write([]byte(`[]`))
		return
	}
	kind := strings.ReplaceAll(strings.Trim(p, "/"), "/", "-")
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	items := []map[string]any{}
	for i := start; i < coll.total && i < start+coll.page; i++ {
		items = append(items, map[string]any{
			"id": fmt.Sprintf("%s-%04d", kind, i), "displayName": fmt.Sprintf("%s %d", kind, i),
			"userPrincipalName": fmt.Sprintf("u%d@contoso.test", i), "accountEnabled": true, "userType": "Member",
			"title": fmt.Sprintf("alert %d", i), "status": "new", "severity": "high", "createdDateTime": recent,
		})
	}
	body := map[string]any{"@odata.context": "https://graph.example/v1.0/$metadata#x", "value": items}
	if !coll.skipPaged && start+coll.page < coll.total {
		next := start + coll.page
		if coll.stuckAt > 0 && start == coll.stuckAt {
			next = start
		}
		host := coll.nextHost
		if host == "" {
			host = "https://" + r.Host
		}
		nq := url.Values{}
		if top := q.Get("$top"); top != "" {
			nq.Set("$top", top)
		}
		nq.Set("$skiptoken", strconv.Itoa(next))
		body["@odata.nextLink"] = host + r.URL.Path + "?" + nq.Encode()
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (f *syncPagesFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// syncPagesServer starts f over TLS (the credential kit sends a token over
// https only) and points the CLI at it with a bearer token.
func syncPagesServer(t *testing.T, f http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MICROSOFT_GRAPH_CA_CERT", ca)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "syncPagesToken-5e1d9a")
	return srv
}

func storedCount(t *testing.T, db, resource string) int {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open %s: %v", db, err)
	}
	defer st.Close()
	n, err := st.Count(resource)
	if err != nil {
		t.Fatalf("count %s: %v", resource, err)
	}
	return n
}

func syncCompleteEvent(resource string, total int) string {
	return fmt.Sprintf(`{"event":"sync_complete","resource":"%s","total":%d,`, resource, total)
}

// Every resource sync reads is read to the end of its collection: 120 alerts
// in Graph's pages of 50 (the adversarial review's input: one request,
// sync_complete total 50), 6 users in pages of 2, and every other resource
// in pages of 2, by nextLink or, for numberAssignments, by $skip window. The
// same walk backs `workflow archive` (and the MCP sync and workflow_archive
// tools, which run these commands).
func TestSyncReadsEveryPage(t *testing.T) {
	testenv.Isolate(t)
	f := &syncPagesFake{colls: map[string]*pagedCollection{}}
	want := map[string]int{}
	for _, resource := range defaultSyncResources() {
		path, err := syncResourcePath(resource)
		if err != nil {
			t.Fatal(err)
		}
		want[resource] = 5
		f.colls[path] = &pagedCollection{total: 5, page: 2, skipPaged: skipPagedCollections[path]}
	}
	f.colls["/security/alerts_v2"] = &pagedCollection{total: 120, page: 50}
	f.colls["/users"] = &pagedCollection{total: 6, page: 2}
	want["security"], want["users"] = 120, 6
	syncPagesServer(t, f)

	db := filepath.Join(t.TempDir(), "sync.db")
	out, err := runTranscripts(t, "sync", "--json", "--db", db)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for resource, n := range want {
		if got := storedCount(t, db, resource); got != n {
			t.Errorf("sync stored %d %s, want %d (the whole collection)", got, resource, n)
		}
		if !strings.Contains(out, syncCompleteEvent(resource, n)) {
			t.Errorf("sync did not report %s complete with %d records", resource, n)
		}
	}
	if !strings.Contains(out, fmt.Sprintf(`"success":%d,"warned":0,"errored":0,`, len(want))) {
		t.Errorf("sync summary reports a failure:\n%s", out)
	}
	if t.Failed() {
		t.Logf("sync output:\n%s\nrequests: %v", out, f.requests)
	}

	archive := filepath.Join(t.TempDir(), "archive.db")
	out, err = runTranscripts(t, "workflow", "archive", "--json", "--db", archive)
	if err != nil {
		t.Fatalf("workflow archive: %v\n%s", err, out)
	}
	for resource, n := range want {
		if got := storedCount(t, archive, resource); got != n {
			t.Errorf("workflow archive stored %d %s, want %d", got, resource, n)
		}
	}
}

// A read that cannot be proven complete is an error, never sync_complete:
// a nextLink naming another host (never followed), a page that fails, a
// page with no value array, a nextLink that names its own page. Nothing of
// the unfinished read is stored as the collection.
func TestSyncIncompleteReadIsNeverComplete(t *testing.T) {
	other := &syncPagesFake{colls: map[string]*pagedCollection{}}
	otherSrv := httptest.NewTLSServer(other)
	t.Cleanup(otherSrv.Close)

	for name, coll := range map[string]*pagedCollection{
		"nextLink to another host":    {total: 120, page: 50, nextHost: otherSrv.URL},
		"503 on page 2":               {total: 120, page: 50, failAt: 50},
		"page 2 has no value array":   {total: 120, page: 50, bareAt: 50},
		"nextLink names its own page": {total: 120, page: 50, stuckAt: 50},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			f := &syncPagesFake{colls: map[string]*pagedCollection{"/security/alerts_v2": coll}}
			syncPagesServer(t, f)
			db := filepath.Join(t.TempDir(), "sync.db")
			out, err := runTranscripts(t, "sync", "--json", "--resources", "security", "--db", db)
			if err == nil {
				t.Errorf("sync exited 0")
			}
			if strings.Contains(out, `"event":"sync_complete","resource":"security"`) {
				t.Errorf("sync reported security complete")
			}
			if !strings.Contains(out, `"event":"sync_error","resource":"security"`) || !strings.Contains(out, `"success":0,"warned":0,"errored":1,`) {
				t.Errorf("sync did not report security errored")
			}
			if got := storedCount(t, db, "security"); got != 0 {
				t.Errorf("sync stored %d alerts of an unfinished read", got)
			}
			if f.count() > 10 {
				t.Errorf("sync sent %d requests", f.count())
			}
			if t.Failed() {
				t.Logf("output:\n%s\nrequests: %v", out, f.requests)
			}
		})
	}
	if n := other.count(); n != 0 {
		t.Errorf("the other host was sent %d request(s)", n)
	}
}

// --max-pages and --latest-only keep their contract: a cap that stops the
// read before the collection's end reports max_pages_cap_hit; a cap reached
// on the last page is a complete read.
func TestSyncMaxPagesCapOnGraphPages(t *testing.T) {
	testenv.Isolate(t)
	f := &syncPagesFake{colls: map[string]*pagedCollection{"/security/alerts_v2": {total: 120, page: 50}}}
	syncPagesServer(t, f)
	const capHit = `"reason":"max_pages_cap_hit"`

	db := filepath.Join(t.TempDir(), "one.db")
	out, err := runTranscripts(t, "sync", "--json", "--resources", "security", "--max-pages", "1", "--db", db)
	if err != nil || !strings.Contains(out, capHit) || storedCount(t, db, "security") != 50 {
		t.Errorf("--max-pages 1: %v, want the cap reported and 50 stored\n%s", err, out)
	}

	db = filepath.Join(t.TempDir(), "three.db")
	out, err = runTranscripts(t, "sync", "--json", "--resources", "security", "--max-pages", "3", "--db", db)
	if err != nil || strings.Contains(out, capHit) || !strings.Contains(out, syncCompleteEvent("security", 120)) {
		t.Errorf("--max-pages 3 (the last page): %v, want complete with 120 and no cap\n%s", err, out)
	}

	db = filepath.Join(t.TempDir(), "latest.db")
	out, err = runTranscripts(t, "sync", "--json", "--resources", "security", "--latest-only", "--db", db)
	if err != nil || strings.Contains(out, capHit) || storedCount(t, db, "security") != 50 {
		t.Errorf("--latest-only: %v, want the head page (50) and no cap warning\n%s", err, out)
	}
}

// A page after the first that is not a Graph collection fails the read in
// every reader: a bare JSON array is no page of a Graph collection, so a
// second page (or a second $skip window) answered with [] is not the end.
// GetCollection used to read such a page as the last one and report the
// first page as the whole collection (users list, phone-numbers list and the
// MCP list tools read through it; pull through the same walk).
func TestContinuationPageMustBeAGraphCollection(t *testing.T) {
	testenv.Isolate(t)
	f := &syncPagesFake{colls: map[string]*pagedCollection{
		"/users": {total: 6, page: 2, bareAt0: 2},
		"/admin/teams/telephoneNumberManagement/numberAssignments": {total: 6, page: 2, skipPaged: true, bareAt0: 2},
	}}
	syncPagesServer(t, f)
	for _, args := range [][]string{
		{"users", "list", "--json", "--data-source", "live"},
		{"phone-numbers", "list", "--json", "--data-source", "live"},
		{"pull", "--json", "--only", "users,phone-numbers", "--db", filepath.Join(t.TempDir(), "pull.db")},
		{"sync", "--json", "--resources", "users,phone-numbers", "--db", filepath.Join(t.TempDir(), "sync.db")},
	} {
		out, err := runTranscripts(t, args...)
		if args[0] == "pull" {
			// pull reports a per-resource error and keeps the previous snapshot.
			if strings.Count(out, `"error":`) != 2 {
				t.Errorf("%s: want both resources failed\n%s", strings.Join(args, " "), out)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s exited 0 over a page that is not a Graph collection\n%s", strings.Join(args, " "), out)
		}
	}
}
