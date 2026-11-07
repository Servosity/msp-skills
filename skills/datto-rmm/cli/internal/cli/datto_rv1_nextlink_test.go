// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

// Codex review r1 class "next-link query dropped" (datto-rmm handfix
// next-link-query-followed): Datto documents /v2/activity-logs paging as
// page=next|previous together with the searchAfter pointer it puts in
// pageDetails.nextPageUrl. Sync, list --all and export kept only the page
// value, so the second request asked page=next with no pointer and the walk
// repeated or lost activities. Every walk now carries the whole query of the
// next link (on the configured host).
type activityDouble struct {
	srv  *httptest.Server
	mu   sync.Mutex
	asks []string
}

func newActivityDouble(t *testing.T) *activityDouble {
	t.Helper()
	d := &activityDouble{}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.asks = append(d.asks, r.URL.RawQuery)
		d.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v2/activity-logs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		act := func(id, host string, date int) string {
			return fmt.Sprintf(`{"id":%q,"entity":"DEVICE","category":"job","action":"deployment","date":%d,"hostname":%q,"site":{"id":1,"name":"Acme"},"user":{"id":2,"userName":"tech"}}`, id, date, host)
		}
		switch {
		case q.Get("page") == "next" && q.Get("searchAfter") == "1793232000000,log-1":
			_, _ = fmt.Fprintf(w, `{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":null},"activities":[%s]}`, act("log-2", "PC2", 1793231000))
		default:
			// The first page (and any request without the pointer).
			next := "http://" + r.Host + "/api/v2/activity-logs?page=next&searchAfter=" + url.QueryEscape("1793232000000,log-1") + "&size=1"
			_, _ = fmt.Fprintf(w, `{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":%q},"activities":[%s]}`, next, act("log-1", "PC1", 1793232000))
		}
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func TestEveryWalkFollowsTheWholeNextLink(t *testing.T) {
	t.Run("list --all", func(t *testing.T) {
		d := newActivityDouble(t)
		stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_TOKEN": "nextLinkToken-1a1a"},
			"activity-logs", "--all", "--size", "1", "--json", "--data-source", "live")
		if err != nil {
			t.Fatalf("activity-logs --all: %v\n%s", err, stderr)
		}
		if !strings.Contains(stdout, "log-2") {
			t.Fatalf("--all never reached the linked page (requests %v)", d.asks)
		}
	})
	t.Run("sync", func(t *testing.T) {
		d := newActivityDouble(t)
		db := filepath.Join(t.TempDir(), "sync.db")
		if _, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_TOKEN": "nextLinkToken-2b2b"},
			"sync", "--resources", "activity-logs", "--full", "--json", "--db", db); err != nil {
			t.Fatalf("sync: %v\n%s", err, stderr)
		}
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = 'activity-logs' AND id = 'log-2'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("sync never stored the linked page (requests %v)", d.asks)
		}
	})
	t.Run("export", func(t *testing.T) {
		d := newActivityDouble(t)
		out := filepath.Join(t.TempDir(), "activities.jsonl")
		_, stderr2, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_TOKEN": "nextLinkToken-3c3c"},
			"export", "activity-logs", "--output", out)
		if err != nil {
			t.Fatalf("export: %v\n%s", err, stderr2)
		}
		b, _ := os.ReadFile(out)
		if !strings.Contains(string(b), "log-2") {
			t.Fatalf("export never wrote the linked page (requests %v)\n%s", d.asks, b)
		}
	})
}
