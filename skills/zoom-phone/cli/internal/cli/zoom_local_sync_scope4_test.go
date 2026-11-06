// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-6: one recorder per store, scoped walks restart from page one,
// and a failed refresh never leaves the previous walk usable.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
)

// ---- item 1: a second sync waits for the store lock ----

func TestLocalSyncWaitsForStoreLock(t *testing.T) {
	localTestEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.TrimPrefix(r.URL.Path, "/v2") == "/phone/numbers" {
			_, _ = w.Write([]byte(`{"phone_numbers":[` + localTestNumbers[0] + `],"next_page_token":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"next_page_token":""}`))
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")
	abs, err := filepath.Abs(db)
	if err != nil {
		t.Fatal(err)
	}

	// Another sync (or recorder) holds the store lock.
	held, release, freed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = cliutil.WithFileLock(abs+".zoom-sync", func() error {
			close(held)
			<-release
			return nil
		})
		close(freed)
	}()
	<-held

	done := make(chan string, 1)
	go func() {
		cmd := RootCmd()
		var out, errb zoomLockedBuffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"sync", "--full", "--resources", "numbers", "--db", db, "--json"})
		_ = cmd.Execute()
		done <- errb.String()
	}()
	select {
	case <-done:
		t.Fatal("sync ran while another holder had the store lock")
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	<-freed
	var stderr string
	select {
	case stderr = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("sync did not finish after the lock was released")
	}
	if !strings.Contains(stderr, "another sync is running against") {
		t.Fatalf("no wait message: %q", stderr)
	}
	if scope := localScopeRows(t, db); scope["numbers"] != "0|1|" {
		t.Fatalf("scope after the waiting sync = %v", scope)
	}
}

// ---- items 2 and 3: scoped walks across failures ----

// localByocPagedServer serves a two-page type=byoc list (pn-2 then pn-3)
// and a one-page unfiltered list; failPage controls which byoc page fails.
type localByocPagedServer struct {
	*httptest.Server
	mu       sync.Mutex
	failPage string // "", "1" or "2"
	// plainQueries records the query of every unfiltered numbers request.
	plainQueries []string
}

func localNewByocPagedServer(t *testing.T) *localByocPagedServer {
	t.Helper()
	ps := &localByocPagedServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/users":
			_, _ = w.Write([]byte(`{"users":[` + localTestUsers[0] + `],"next_page_token":""}`))
		case "/phone/numbers":
			if r.URL.Query().Get("type") != "byoc" {
				ps.mu.Lock()
				ps.plainQueries = append(ps.plainQueries, r.URL.RawQuery)
				ps.mu.Unlock()
				_, _ = w.Write([]byte(`{"phone_numbers":[` +
					`{"id":"pn-1","number":"+14155550101","source":"internal","status":"available"},` +
					`{"id":"pn-2","number":"+15125550102","source":"external","status":"available"},` +
					`{"id":"pn-3","number":"+15125550103","source":"external","status":"available"}` +
					`],"next_page_token":""}`))
				return
			}
			ps.mu.Lock()
			fail := ps.failPage
			ps.mu.Unlock()
			page := "1"
			if tok := r.URL.Query().Get("next_page_token") + r.URL.Query().Get("page_token"); tok != "" {
				page = "2"
			}
			if fail == page {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":300,"message":"fixture failure on page ` + page + `"}`))
				return
			}
			if page == "1" {
				_, _ = w.Write([]byte(`{"phone_numbers":[{"id":"pn-2","number":"+15125550102","source":"external","status":"available","carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":1}],"next_page_token":"p2","page_size":1,"total_records":2}`))
				return
			}
			_, _ = w.Write([]byte(`{"phone_numbers":[{"id":"pn-3","number":"+15125550103","source":"external","status":"available","carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":1}],"next_page_token":"","page_size":1,"total_records":2}`))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	t.Cleanup(ps.Close)
	t.Setenv("ZOOM_PHONE_BASE_URL", ps.URL+"/v2")
	return ps
}

func (ps *localByocPagedServer) setFail(page string) {
	ps.mu.Lock()
	ps.failPage = page
	ps.mu.Unlock()
}

func localE911(t *testing.T, db string) localE911Result {
	t.Helper()
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", db)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	var e localE911Result
	localDecode(t, stdout, &e)
	return e
}

// An interrupted byoc walk followed by a successful retry: the retry starts
// from page one, so the BYOC numbers fetched before the interruption are in
// the walk's row set.
func TestLocalByocRetryRestartsFromPageOne(t *testing.T) {
	localTestEnv(t)
	ps := localNewByocPagedServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	byocArgs := []string{"--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json"}

	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	ps.setFail("2") // page 1 (pn-2) lands, then the walk fails
	localRunSync(t, byocArgs...)
	ps.setFail("")
	localRunSync(t, byocArgs...)
	if got := localByocFindings(localE911(t, db)); got != "pn-2,pn-3" {
		t.Fatalf("after a resumed retry BYOC = %q, want pn-2,pn-3 (page one must be re-walked)", got)
	}
}

// A refresh that fails part-way leaves the walk pending: the previous walk
// is not used over the rows it partly rewrote.
func TestLocalFailedByocRefreshIsNotUsed(t *testing.T) {
	localTestEnv(t)
	ps := localNewByocPagedServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	byocArgs := []string{"--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json"}

	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	localRunSync(t, byocArgs...)
	if got := localByocFindings(localE911(t, db)); got != "pn-2,pn-3" {
		t.Fatalf("first walk BYOC = %q", got)
	}
	// A usage error never starts fetching: the walk stays usable.
	localRunSync(t, "--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--resource-param", "typo:key=value", "--json")
	if got := localByocFindings(localE911(t, db)); got != "pn-2,pn-3" {
		t.Fatalf("after a usage error BYOC = %q, want the walk still usable", got)
	}
	// The refresh rewrites page 1 then fails on page 2.
	ps.setFail("2")
	localRunSync(t, byocArgs...)
	e := localE911(t, db)
	if localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "the last BYOC numbers walk did not complete") ||
		!strings.Contains(strings.Join(e.SkippedChecks, "\n"), "re-run: zoom-phone-cli sync --resources numbers --resource-param numbers:type=byoc") {
		t.Fatalf("after a failed refresh: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
	// Re-running the walk restores it.
	ps.setFail("")
	localRunSync(t, byocArgs...)
	if got := localByocFindings(localE911(t, db)); got != "pn-2,pn-3" {
		t.Fatalf("after re-running the walk BYOC = %q", got)
	}
}

func TestLocalFailedSiteWalkRefreshIsNotUsed(t *testing.T) {
	localTestEnv(t)
	var mu sync.Mutex
	failHQ := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/sites":
			_, _ = w.Write([]byte(`{"sites":[{"id":"site-hq","name":"San Jose HQ"}],"next_page_token":""}`))
		case "/phone/locations":
			mu.Lock()
			fail := failHQ
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":300,"message":"fixture failure"}`))
				return
			}
			_, _ = w.Write([]byte(`{"locations":[{"id":"loc-hq","name":"HQ Floor 1","site":{"id":"site-hq","name":"San Jose HQ"}}],"next_page_token":""}`))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")
	siteArgs := []string{"--resources", "locations", "--db", db, "--resource-param", "locations:site_id=site-hq", "--json"}

	localRunSync(t, "--full", "--resources", "sites", "--db", db, "--json")
	localRunSync(t, siteArgs...)
	if e := localE911(t, db); localContains(e.MissingLocal, "locations") {
		t.Fatalf("after the HQ walk: missing=%v reason=%q", e.MissingLocal, e.MissingReasons["locations"])
	}
	mu.Lock()
	failHQ = true
	mu.Unlock()
	localRunSync(t, siteArgs...)
	e := localE911(t, db)
	if !localContains(e.MissingLocal, "locations") || !strings.Contains(e.MissingReasons["locations"], "the last walk for site(s) site-hq did not complete") {
		t.Fatalf("after a failed HQ refresh: missing=%v reason=%q", e.MissingLocal, e.MissingReasons["locations"])
	}
	if strings.Contains(localFindingSites(e, "SITE_NO_ERL"), "site-hq") {
		t.Fatalf("SITE_NO_ERL judged from a failed walk")
	}
}

// After a failed byoc walk, a plain numbers sync must start from page one,
// not resume from the byoc listing's cursor.
func TestLocalPlainSyncDoesNotResumeScopedCursor(t *testing.T) {
	localTestEnv(t)
	ps := localNewByocPagedServer(t)
	db := filepath.Join(t.TempDir(), "data.db")

	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--json")
	ps.setFail("2") // page 1 lands, the byoc walk fails holding cursor p2
	localRunSync(t, "--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json")
	ps.mu.Lock()
	ps.plainQueries = nil
	ps.mu.Unlock()

	localRunSync(t, "--resources", "numbers", "--db", db, "--json")
	ps.mu.Lock()
	queries := append([]string(nil), ps.plainQueries...)
	ps.mu.Unlock()
	if len(queries) == 0 {
		t.Fatal("the plain sync made no unfiltered numbers request")
	}
	if strings.Contains(queries[0], "page_token") {
		t.Fatalf("the plain sync resumed from the byoc cursor: first request ?%s", queries[0])
	}
	// The failed walk is still pending: completeness semantics unchanged.
	if e := localE911(t, db); !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "the last BYOC numbers walk did not complete") {
		t.Fatalf("failed walk no longer pending: skipped=%v", e.SkippedChecks)
	}
}
