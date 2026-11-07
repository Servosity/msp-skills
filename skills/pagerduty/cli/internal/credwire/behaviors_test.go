// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). The non-credential behaviors the retired
// black-box suite (internal/pagerdutyblackbox) covered, driven through the
// built CLI against an httptest TLS server with the key held by the kit:
// pagerduty handfixes export-offset-envelope, export-file-closed-on-error,
// cursor-all-walks-every-page and dry-run-sends-nothing.

package credwire

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// requestLog records "METHOD /path" for every request a fake answered.
type requestLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *requestLog) add(r *http.Request) {
	l.mu.Lock()
	l.seen = append(l.seen, r.Method+" "+r.URL.Path)
	l.mu.Unlock()
}

func (l *requestLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

// offsetPage is PagerDuty's documented offset envelope over items.
func offsetPage(key string, q url.Values, items []string) string {
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	start := min(max(offset, 0), len(items))
	end := min(start+limit, len(items))
	return fmt.Sprintf(`{%q:[%s],"limit":%d,"offset":%d,"more":%t,"total":null}`, key, strings.Join(items[start:end], ","), limit, offset, end < len(items))
}

// TestExportWalksEveryPage (export-offset-envelope): export reads the plural
// out of the documented offset envelope and walks every page, to a file and
// to stdout; users list --all walks the same pages.
func TestExportWalksEveryPage(t *testing.T) {
	const total = 250
	var users []string
	for i := 1; i <= total; i++ {
		users = append(users, fmt.Sprintf(`{"id":"PU%04d","type":"user","summary":"Paged user %d","name":"Paged user %d","email":"paged%d@hq-record.example.com","role":"user"}`, i, i, i, i))
	}
	srv, ca, authed := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users" {
			fmt.Fprint(w, offsetPage("users", r.URL.Query(), users))
			return
		}
		fmt.Fprint(w, offsetPage(filepath.Base(r.URL.Path), r.URL.Query(), nil))
	})
	home := testenv.Isolate(t)
	distinct := func(s string) int {
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`paged([0-9]+)@hq-record`).FindAllStringSubmatch(s, -1) {
			seen[m[1]] = true
		}
		return len(seen)
	}
	file := filepath.Join(home, "users.jsonl")
	out, code := kitRun(t, srv, ca, home, "export", "users", "--format", "jsonl", "--output", file)
	b, err := os.ReadFile(file)
	if code != 0 || err != nil {
		t.Fatalf("export --output: exit %d, read %v (%d bytes: %.300q):\n%q", code, err, len(b), b, out)
	}
	if n := distinct(string(b)); n != total || strings.Count(string(b), "@hq-record") != total {
		t.Errorf("export --output wrote %d distinct of %d users:\n%.600s", n, total, out)
	}
	if out, _ := kitRun(t, srv, ca, home, "export", "users", "--format", "json"); distinct(out) != total {
		t.Errorf("export --format json printed %d distinct of %d users:\n%.600s", distinct(out), total, out)
	}
	if out, _ := kitRun(t, srv, ca, home, "users", "list", "--all", "--json"); distinct(out) != total {
		t.Errorf("users list --all printed %d distinct of %d users:\n%.600s", distinct(out), total, out)
	}
	if *authed == 0 {
		t.Fatal("the CLI never sent the key to the fake, so nothing was tested")
	}
}

// TestExportOutputReturnsAfterAnAPIError (export-file-closed-on-error): an API
// error after export opened its output file fails the run and returns.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	var log requestLog
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Unauthorized","code":2006}}`)
	})
	home := testenv.Isolate(t)
	out, code := kitRun(t, srv, ca, home, "export", "rulesets", "--output", filepath.Join(home, "rulesets.jsonl"))
	if code == 0 {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	if !strings.Contains(strings.Join(log.all(), "\n"), "GET /rulesets") {
		t.Fatalf("export never asked the API for the rulesets; the test did not run (requests %v):\n%s", log.all(), out)
	}
}

// TestCursorListWalksEveryPage (cursor-all-walks-every-page): a cursor-paged
// list with --all follows next_cursor to the last page.
func TestCursorListWalksEveryPage(t *testing.T) {
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/audit/records" {
			http.NotFound(w, r)
			return
		}
		rec := func(id string) string {
			return fmt.Sprintf(`{"id":%q,"execution_time":"2026-10-01T00:00:00Z","action":"update","root_resource":{"id":"PSVC001","type":"service_reference"}}`, id)
		}
		if r.URL.Query().Get("cursor") == "PAGE2" {
			fmt.Fprintf(w, `{"records":[%s],"next_cursor":null,"limit":100}`, rec("PAUD002"))
			return
		}
		fmt.Fprintf(w, `{"records":[%s],"next_cursor":"PAGE2","limit":100}`, rec("PAUD001"))
	})
	out := runKitCLI(t, srv, ca, "audit", "--all", "--json")
	if !strings.Contains(out, "PAUD001") || !strings.Contains(out, "PAUD002") {
		t.Errorf("audit --all did not print both pages:\n%.2000s", out)
	}
}

// TestDryRunSendsNothing (dry-run-sends-nothing): a --dry-run never reaches
// the API, writes and reads alike; without it the same calls do.
func TestDryRunSendsNothing(t *testing.T) {
	var log requestLog
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/users/PUSR001":
			fmt.Fprint(w, `{"user":{"id":"PUSR001","type":"user","name":"Pat Clicker"}}`)
		default:
			fmt.Fprint(w, offsetPage(filepath.Base(r.URL.Path), r.URL.Query(), nil))
		}
	})
	home := testenv.Isolate(t)
	for _, args := range [][]string{
		{"services", "delete", "PSVC001", "--yes", "--dry-run"},
		{"incidents", "create", "--from", "pat.clicker@hq-record.example.com", "--incident-type", "incident", "--incident-service-type", "service_reference", "--incident-title", "Dry", "--yes", "--json", "--dry-run"},
		{"users", "get", "PUSR001", "--dry-run"},
		{"incidents", "list", "--dry-run"},
		{"sync", "--resources", "users", "--dry-run"},
	} {
		// Each run reaches its preview, so the count below is not vacuous.
		if out, _ := kitRun(t, srv, ca, home, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
			t.Errorf("%v printed no preview:\n%.1500s", args, out)
		}
	}
	if m := log.all(); len(m) != 0 {
		t.Fatalf("--dry-run sent %v", m)
	}
	kitRun(t, srv, ca, home, "services", "delete", "PSVC001", "--yes")
	kitRun(t, srv, ca, home, "users", "get", "PUSR001", "--json")
	if m := log.all(); len(m) != 2 || m[0] != "DELETE /services/PSVC001" || m[1] != "GET /users/PUSR001" {
		t.Errorf("control: want the one DELETE and the one GET, got %v", m)
	}
}

// TestSyncStoresRecordsWithTheKeyHeld: the store's migrations and writes pass
// the kit's SQL driver with the key held, and a local read returns the row.
func TestSyncStoresRecordsWithTheKeyHeld(t *testing.T) {
	srv, ca, authed := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users" {
			fmt.Fprint(w, offsetPage("users", r.URL.Query(), []string{`{"id":"PUSR001","type":"user","summary":"Pat Clicker","name":"Pat Clicker","email":"pat.clicker@hq-record.example.com","role":"user"}`}))
			return
		}
		fmt.Fprint(w, offsetPage(filepath.Base(r.URL.Path), r.URL.Query(), nil))
	})
	home := testenv.Isolate(t)
	out, code := kitRun(t, srv, ca, home, "sync", "--resources", "users", "--json")
	if code != 0 || strings.Contains(out, "credkit: withheld") {
		t.Fatalf("sync: exit %d:\n%.2000s", code, out)
	}
	if *authed == 0 {
		t.Fatal("the CLI never sent the key to the fake, so nothing was tested")
	}
	out, code = kitRun(t, srv, ca, home, "users", "list", "--data-source", "local", "--json")
	if code != 0 || !strings.Contains(out, "Pat Clicker") {
		t.Fatalf("local read after sync: exit %d:\n%.2000s", code, out)
	}
}
