// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json api-origin-pinned,
// documented-field-read-as-credential): the Afi API key, sent as the raw
// Authorization header, goes only to the configured base URL (the Origins
// declaration, enforced by the kit's transport), and the page_token cursors
// every list pages with are data, never held.

package credwire

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const originKey = "originkey-tttt-yyyy-tttt"

// TestKeyGoesOnlyToTheBaseURL: a redirect from the API to another origin is
// not followed with the key, and a plain-http base URL never receives it. The
// control shows the key does reach the https base URL as Authorization.
func TestKeyGoesOnlyToTheBaseURL(t *testing.T) {
	var other recorder
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other.add(r)
		_, _ = io.WriteString(w, `{"id":"T"}`)
	}))
	defer elsewhere.Close()

	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) {
		if r.URL.Path == "/api/v1/tenants/moved" {
			http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"T","name":"HQ-Tenant"}`)
	}, "AFI_API_KEY="+originKey)
	r := e.run(t, "", "tenants", "T", "--json", "--data-source", "live")
	got := e.api.all()
	if len(got) != 1 || got[0].Header.Get("Authorization") != originKey || !strings.Contains(r.out, "HQ-Tenant") {
		t.Fatalf("control: want one request carrying the key as Authorization and its answer printed, got %+v\nprinted:\n%s", got, r.out)
	}
	r = e.run(t, "", "tenants", "moved", "--json", "--data-source", "live")
	for _, s := range append(other.all(), e.proxy.all()...) {
		if strings.Contains(fmt.Sprint(s.Header), originKey) {
			t.Fatalf("the key followed a redirect to %s: %+v\nprinted:\n%s", s.Host, s, r.out)
		}
	}
	if strings.Contains(r.out, originKey) {
		t.Fatalf("the refusal printed the key:\n%s", r.out)
	}

	var plain recorder
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plain.add(r)
		_, _ = io.WriteString(w, `{"id":"T"}`)
	}))
	defer insecure.Close()
	e2 := newEnv(t, nil, "AFI_API_KEY="+originKey)
	e2.vars = append(e2.vars, "AFI_BASE_URL="+insecure.URL)
	r = e2.run(t, "", "tenants", "T", "--json", "--data-source", "live")
	for _, s := range plain.all() {
		if strings.Contains(fmt.Sprint(s.Header), originKey) {
			t.Fatalf("the key went to a plain-http base URL: %+v\nprinted:\n%s", s, r.out)
		}
	}
}

// TestPageTokensAreData: next_page_token and page_token are cursors. A paged
// list prints every page, the cursor round-trips to the API, and nothing is
// withheld, on the CLI and in sync.
func TestPageTokensAreData(t *testing.T) {
	const cursor = "pg-2CursorValue7h3k"
	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/applications/me/installations" {
			_, _ = io.WriteString(w, `{"items":[],"next_page_token":""}`)
			return
		}
		if r.URL.Query().Get("page_token") == cursor {
			_, _ = io.WriteString(w, `{"items":[{"tenant_id":"tnt-page2","org_id":"org-1042"}],"next_page_token":""}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"items":[{"tenant_id":"tnt-page1","org_id":"org-1042"}],"next_page_token":%q}`, cursor)
	}, "AFI_API_KEY="+originKey)
	for _, args := range [][]string{
		{"applications", "--all", "--json", "--data-source", "live"},
		{"export", "applications"},
	} {
		r := e.run(t, "", args...)
		if !strings.Contains(r.out, "tnt-page1") || !strings.Contains(r.out, "tnt-page2") || strings.Contains(r.out, "withheld") {
			t.Errorf("%v did not print both pages whole:\n%s", args, r.out)
		}
	}
	r := e.run(t, "", "sync", "--resources", "applications", "--full")
	if strings.Contains(r.out, "withheld") {
		t.Errorf("sync withheld a page:\n%s", r.out)
	}
	round := 0
	for _, s := range e.api.all() {
		if strings.Contains(s.Query, "page_token="+cursor) {
			round++
		}
	}
	if round < 3 {
		t.Fatalf("the cursor reached the API %d times, want once per run (3)", round)
	}
}
