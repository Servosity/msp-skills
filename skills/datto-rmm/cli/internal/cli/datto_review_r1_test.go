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
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"datto-rmm-pp-cli/internal/store"
)

// dattoListPage answers one page of a Datto list the way the API does: the
// records under their plural name and pageDetails with the absolute link to
// the next page (null on the last). The page is the number the request asks
// for (0 when it asks none); the size is the max it asks for, capped at
// limit whatever it asks (Datto caps at 250), so a list longer than a page
// needs nextPageUrl.
func dattoListPage(w http.ResponseWriter, r *http.Request, plural string, total, limit int, record func(i int) map[string]any) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size := limit
	if m, err := strconv.Atoi(q.Get("max")); err == nil && m > 0 && m < size {
		size = m
	}
	items := []map[string]any{}
	for i := page * size; i < total && i < (page+1)*size; i++ {
		items = append(items, record(i))
	}
	var next any
	if (page+1)*size < total {
		next = fmt.Sprintf("http://%s%s?max=%d&page=%d", r.Host, r.URL.Path, size, page+1)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"pageDetails": map[string]any{"count": len(items), "totalCount": total, "prevPageUrl": nil, "nextPageUrl": next},
		plural:        items,
	})
}

// Codex r1 (action1 reprint on 4.34.2) class incomplete paging read as
// complete, on Datto's list shape: every list is a page of records plus
// pageDetails.nextPageUrl, capped by the server whatever max asks for (50
// here; Datto caps at 250). Each list holds more records than one page
// (devices keyed id, open alerts keyed alertUid, users keyed username: the
// resources whose keys the reprint fixed), so sync must follow every link to
// store them all, on a token the API key and secret mint.
func TestSyncReadsEveryPage(t *testing.T) {
	const devices, alerts, users, pageCap = 123, 101, 51, 50
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/oauth/token":
			_ = r.ParseForm()
			if r.Header.Get("Authorization") != dattoPublicClientForTest || r.PostForm.Get("username") != "paging-api-key" || r.PostForm.Get("password") != "paging-api-secret-3b7d" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			mints.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"paging-minted-token-6c1e","token_type":"bearer","expires_in":359999}`)
		case r.Header.Get("Authorization") != "Bearer paging-minted-token-6c1e":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/api/v2/account/devices":
			dattoListPage(w, r, "devices", devices, pageCap, func(i int) map[string]any {
				return map[string]any{"id": 5000 + i, "uid": fmt.Sprintf("dev-%04d", i), "hostname": fmt.Sprintf("HOST-%04d", i), "siteUid": "site-1"}
			})
		case r.URL.Path == "/api/v2/account/alerts/open":
			dattoListPage(w, r, "alerts", alerts, pageCap, func(i int) map[string]any {
				return map[string]any{"alertUid": fmt.Sprintf("alert-%04d", i), "priority": "High", "resolved": false}
			})
		case r.URL.Path == "/api/v2/account/users":
			dattoListPage(w, r, "users", users, pageCap, func(i int) map[string]any {
				return map[string]any{"username": fmt.Sprintf("tech%04d", i), "email": fmt.Sprintf("tech%04d@example.com", i)}
			})
		default:
			_, _ = io.WriteString(w, `{"pageDetails":{"count":0,"totalCount":0,"prevPageUrl":null,"nextPageUrl":null}}`)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"DATTO_RMM_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "DATTO_RMM_TOKEN": "",
		"DATTO_RMM_PLATFORM": "", "DATTO_RMM_API_URL": "", "DATTO_RMM_CONFIG": filepath.Join(dir, "config.toml"),
		"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_API_KEY": "paging-api-key", "DATTO_RMM_API_SECRET_KEY": "paging-api-" + "secret-3b7d",
		"DATTO_RMM_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	db := filepath.Join(dir, "fleet.db")
	var out, errb bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--db", db, "--full", "--resources", "account-devices,account-alerts-open,account-users", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if mints.Load() == 0 {
		t.Fatalf("sync never minted; the fake answered nothing:\n%s%s", out.String(), errb.String())
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for rt, want := range map[string]int{"account-devices": devices, "account-alerts-open": alerts, "account-users": users} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: sync stored %d of %d records; it stopped before the last page:\n%s", rt, n, want, strings.TrimSpace(errb.String()))
		}
	}
}

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity, as it reaches Datto: each platform host is its own tenant.
// Host A was used with a pasted token (auth set-token) and synced; the
// operator then points the CLI at host B with B's API key and secret. The
// default store is named by the token in effect when no key is configured and
// must never be adopted into the key's store on the next run, so B's fleet
// views (fleet stale, read from the local store) show only what B synced.
func TestStoreOfAnotherHostIsNeverAdopted(t *testing.T) {
	lastSeen := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	host := func(tag string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/auth/oauth/token":
				_, _ = io.WriteString(w, `{"access_token":"scope-minted-`+tag+`-4d2a","token_type":"bearer","expires_in":359999}`)
			case r.URL.Path == "/api/v2/account/devices":
				_, _ = io.WriteString(w, `{"pageDetails":{"count":1,"totalCount":1,"prevPageUrl":null,"nextPageUrl":null},"devices":[`+
					`{"id":7`+fmt.Sprint(len(tag))+`,"uid":"dev-`+tag+`","hostname":"HOST-`+tag+`","siteName":"Site-`+tag+`","lastSeen":"`+lastSeen+`","online":false}]}`)
			default:
				_, _ = io.WriteString(w, `{"pageDetails":{"count":0,"totalCount":0,"prevPageUrl":null,"nextPageUrl":null}}`)
			}
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	a, b := host("HostA"), host("HostB")
	dir := t.TempDir()
	for k, v := range map[string]string{"DATTO_RMM_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "DATTO_RMM_CONFIG": "",
		"DATTO_RMM_TOKEN": "", "DATTO_RMM_PLATFORM": "", "DATTO_RMM_API_URL": "", "DATTO_RMM_API_KEY": "", "DATTO_RMM_API_SECRET_KEY": "",
		"DATTO_RMM_NO_CONFIG_WRITE": ""} {
		t.Setenv(k, v)
	}
	run := func(stdin string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	t.Setenv("DATTO_RMM_BASE_URL", a.URL+"/api")
	run("scope-pasted-token-HostA-0123456789\n", "auth", "set-token")
	if out := run("", "sync", "--resources", "account-devices", "--json"); !strings.Contains(out, `"total_records": 1`) && !strings.Contains(out, `"total_records":1`) {
		t.Fatalf("host A's sync stored nothing; the test did not run:\n%s", out)
	}
	if out := run("", "fleet", "stale", "--days", "1", "--json"); !strings.Contains(out, "HOST-HostA") {
		t.Fatalf("host A's own view lost its device; the control did not run:\n%s", out)
	}
	t.Setenv("DATTO_RMM_BASE_URL", b.URL+"/api")
	t.Setenv("DATTO_RMM_API_KEY", "scope-key-HostB")
	t.Setenv("DATTO_RMM_API_SECRET_KEY", "scope-secret-HostB-7c3e")
	if out := run("", "fleet", "stale", "--days", "1", "--json"); strings.Contains(out, "HOST-HostA") {
		t.Errorf("host B's view reads host A's synced device (A's store was adopted into B's):\n%s", out)
	}
	if out := run("", "sync", "--resources", "account-devices", "--json"); !strings.Contains(out, `"errored": 0`) && !strings.Contains(out, `"errored":0`) {
		t.Fatalf("host B's sync failed:\n%s", out)
	}
	out := run("", "fleet", "stale", "--days", "1", "--json")
	if !strings.Contains(out, "HOST-HostB") {
		t.Fatalf("host B's view lost its own device; the control did not run:\n%s", out)
	}
	if strings.Contains(out, "HOST-HostA") {
		t.Errorf("host B's view reads host A's synced device:\n%s", out)
	}
}

// export-follows-next-page-url (tests-helper finding): export reads a Datto
// list the way the API pages it (from page 0, pageDetails.nextPageUrl until
// it is null) and writes each record, not each page envelope, then stops.
// 4.34.2's generic export asked page=1 first (skipping page 0), wrote every
// page object as one record, and never saw an empty page (Datto answers a
// page past the end with an empty list inside the envelope), so it walked
// 100000 pages before failing. The fake refuses after a page budget, so a
// runaway fails fast instead of hanging the test.
func TestExportWalksEveryDattoPageAndStops(t *testing.T) {
	const components = 263
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) > 20 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"page budget exhausted"}`)
			return
		}
		if r.URL.Path != "/api/v2/account/components" || r.Header.Get("Authorization") != "Bearer export-token-5f1a" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		dattoListPage(w, r, "components", components, 250, func(i int) map[string]any {
			return map[string]any{"id": 900 + i, "uid": fmt.Sprintf("comp-%04d", i), "name": fmt.Sprintf("Component %d", i), "credentialsRequired": false}
		})
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"DATTO_RMM_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "DATTO_RMM_CONFIG": "",
		"DATTO_RMM_PLATFORM": "", "DATTO_RMM_API_URL": "", "DATTO_RMM_API_KEY": "", "DATTO_RMM_API_SECRET_KEY": "",
		"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "export-token-5f1a", "DATTO_RMM_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	for _, format := range []string{"jsonl", "json"} {
		requests.Store(0)
		out := filepath.Join(dir, "components."+format)
		var stdout, stderr bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"export", "account", "--format", format, "--output", out})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("export --format %s: %v (%d requests)\n%s", format, err, requests.Load(), stderr.String())
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		var records []map[string]any
		if format == "json" {
			if err := json.Unmarshal(raw, &records); err != nil {
				t.Fatalf("export --format json is not a JSON array: %v\n%.500s", err, raw)
			}
		} else {
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				var rec map[string]any
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					t.Fatalf("export line is not a JSON object: %v: %.200s", err, line)
				}
				records = append(records, rec)
			}
		}
		if len(records) != components {
			t.Fatalf("--format %s: exported %d records of %d in %d requests", format, len(records), components, requests.Load())
		}
		if records[0]["uid"] != "comp-0000" || records[components-1]["uid"] != fmt.Sprintf("comp-%04d", components-1) {
			t.Fatalf("--format %s: want comp-0000 .. comp-%04d, got %v .. %v (a page skipped, or envelopes written)", format, components-1, records[0]["uid"], records[components-1]["uid"])
		}
		if n := requests.Load(); n > 3 {
			t.Fatalf("--format %s: %d requests for a two-page list", format, n)
		}
	}
}
