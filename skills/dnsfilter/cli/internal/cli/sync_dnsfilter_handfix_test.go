// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests for the sync command (handfixes.json:
// sync-clear-never-marks-complete and secrets-never-persisted).

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/store"
)

const syncHFSentinel = "SENTINEL-SECRET-123"

// syncHFServer answers page 1 of each list with body(path) and every later
// page with an empty list. Unknown paths fail the test.
func syncHFServer(t *testing.T, body func(path string) (string, bool)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if p := r.URL.Query().Get("page[number]"); p != "" && p != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		b, ok := body(r.URL.Path)
		if !ok && strings.HasPrefix(r.URL.Path, "/v1/networks/50/") {
			// networks' parent-keyed dependents (lan_ips, subnets).
			b, ok = `{"data":[]}`, true
		}
		if !ok {
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not in fixture"}`)
			return
		}
		fmt.Fprint(w, b)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	return srv
}

func syncHFRun(args ...string) (string, error) {
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String() + errOut.String(), err
}

// syncHFState dumps sync_state as one comparable string.
func syncHFState(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT resource_type, COALESCE(last_cursor, ''), COALESCE(CAST(last_synced_at AS TEXT), 'NULL'), COALESCE(total_count, 0), last_attempt_complete FROM sync_state ORDER BY resource_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var rt, cursor, at string
		var count, complete int
		if err := rows.Scan(&rt, &cursor, &at, &count, &complete); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s cursor=%q at=%s count=%d complete=%d\n", rt, cursor, at, count, complete)
	}
	return b.String()
}

func syncHFComplete(t *testing.T, dbPath, resource string) (complete int, found bool) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, resource).Scan(&complete)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return complete, true
}

func syncHFSeed(t *testing.T, dbPath string, resources ...string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, r := range resources {
		if err := db.SaveSyncStateAt(r, "c-keep", 3, at); err != nil {
			t.Fatal(err)
		}
	}
}

// A usage error after the --full / --latest-only flags, or a --resources
// typo, must leave sync_state exactly as it was: no clear before validation,
// no row for an unknown name.
func TestSyncUsageErrorsLeaveSyncStateUntouched(t *testing.T) {
	testenv.Isolate(t)
	syncHFServer(t, func(string) (string, bool) { return "", false })
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFSeed(t, dbPath, "networks", "users")
	before := syncHFState(t, dbPath)

	for _, args := range [][]string{
		{"sync", "--full", "--since", "bogus", "--db", dbPath},
		{"sync", "--latest-only", "--resources", "networks", "--since", "bogus", "--db", dbPath},
		{"sync", "--full", "--resources", "netwrks", "--db", dbPath},
	} {
		if _, err := syncHFRun(args...); err == nil {
			t.Fatalf("%v: want an error", args)
		}
		if after := syncHFState(t, dbPath); after != before {
			t.Fatalf("%v changed sync_state:\nbefore:\n%safter:\n%s", args, before, after)
		}
	}
}

// --full must not clear a resource the walk will skip at preflight (domains
// needs --resource-param domains:fqdns=...): a cleared row only becomes
// complete again through a walk, so it would read incomplete forever.
func TestSyncFullDoesNotClearAResourceItWillSkip(t *testing.T) {
	testenv.Isolate(t)
	syncHFServer(t, func(string) (string, bool) { return "", false })
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFSeed(t, dbPath, "users")
	before := syncHFState(t, dbPath)
	_, _ = syncHFRun("sync", "--full", "--resources", "domains", "--db", dbPath)
	if after := syncHFState(t, dbPath); after != before {
		t.Fatalf("a preflight-skipped resource was cleared:\nbefore:\n%safter:\n%s", before, after)
	}
}

// syncResourceWalksAfterClear must agree with syncResource's own preflight for
// every flat resource sync accepts: a walk that starts writes a sync_state row
// before its first request, a preflight skip writes none.
func TestSyncResourceWalksAfterClearMatchesPreflight(t *testing.T) {
	deps := dependentSyncResourceNames()
	for _, sinceTS := range []string{"", "2026-10-01T00:00:00Z"} {
		for _, resource := range knownSyncResourceNames() {
			if deps[resource] {
				continue
			}
			db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
			if err != nil {
				t.Fatal(err)
			}
			_ = syncResource(context.Background(), syncHFStubClient{}, db, resource, sinceTS, true, 1, false, false, nil, io.Discard)
			var rows int
			if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE resource_type = ?`, resource).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			db.Close()
			walked := rows > 0
			if got := syncResourceWalksAfterClear(resource, sinceTS, nil); got != walked {
				t.Errorf("%s (since=%q): syncResourceWalksAfterClear=%v but syncResource walked=%v", resource, sinceTS, got, walked)
			}
		}
	}
}

type syncHFStubClient struct{}

func (syncHFStubClient) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"data":[]}`), nil
}
func (syncHFStubClient) RateLimit() float64 { return 0 }

// A --full run cleared every selected collection before walking any of them.
// While the first walk is in flight (the moment a kill, a timeout or an MCP
// host SIGTERM would freeze the store), a queued collection must not read as
// complete; once both walks finish, both read complete.
func TestSyncFullClearLeavesQueuedCollectionIncomplete(t *testing.T) {
	testenv.Isolate(t)
	inFlight := make(chan struct{})
	release := make(chan struct{})
	var once, releaseOnce sync.Once
	syncHFServer(t, func(path string) (string, bool) {
		switch path {
		case "/v1/networks/all":
			once.Do(func() { close(inFlight) })
			<-release
			return `{"data":[{"id":"50","type":"networks","attributes":{"name":"HQ"}}]}`, true
		case "/v1/users/all":
			return `{"data":[{"id":"9","type":"users","attributes":{"email":"a@example.com"}}]}`, true
		}
		return "", false
	})
	// Registered after the server so it runs first: srv.Close waits for the
	// blocked networks handler.
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFSeed(t, dbPath, "networks", "users")

	done := make(chan error, 1)
	go func() {
		_, err := syncHFRun("sync", "--full", "--resources", "networks,users", "--concurrency", "1", "--db", dbPath)
		done <- err
	}()
	select {
	case <-inFlight:
	case err := <-done:
		t.Fatalf("sync finished before the networks walk started: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("networks walk never started")
	}
	if c, found := syncHFComplete(t, dbPath, "users"); !found || c != 0 {
		t.Fatalf("users was cleared but not walked yet: found=%v last_attempt_complete=%d, want a row with 0\n%s", found, c, syncHFState(t, dbPath))
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, r := range []string{"networks", "users"} {
		if c, _ := syncHFComplete(t, dbPath, r); c != 1 {
			t.Fatalf("%s walked fully but reads incomplete\n%s", r, syncHFState(t, dbPath))
		}
	}
}

// End to end through the real sync command: a networks row and an api-keys
// row carrying live secrets are synced, then every file under the isolated
// home (config, data, state and cache dirs) is grepped for the sentinel.
func TestSyncNeverPersistsSecrets(t *testing.T) {
	home := testenv.Isolate(t)
	syncHFServer(t, func(path string) (string, bool) {
		switch path {
		case "/v1/networks/all":
			return `{"data":[{"id":"50","type":"networks","attributes":{"name":"VISIBLE-HQ","secret_key":"` + syncHFSentinel + `","organization_id":1,"uninstall_pin":"` + syncHFSentinel + `-PIN","pin":"` + syncHFSentinel + `-PIN2","nested":"{\"secret_key\":\"` + syncHFSentinel + `-NESTED\",\"name\":\"VISIBLE-NESTED\"}"},"relationships":{"organization":{"data":{"id":"1","type":"organizations"}}}}]}`, true
		case "/v1/api_keys":
			return `{"data":[{"id":"4","type":"api_keys","attributes":{"name":"VISIBLE-KEY","token":"` + syncHFSentinel + `","last_four":"z123"}}]}`, true
		}
		return "", false
	})
	if out, err := syncHFRun("sync", "--resources", "networks,api-keys"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var visible, leaked []string
	sawNested := false
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		lower := bytes.ToLower(b)
		if bytes.Contains(lower, []byte("visible-hq")) {
			visible = append(visible, path)
		}
		sawNested = sawNested || bytes.Contains(lower, []byte("visible-nested"))
		if bytes.Contains(lower, []byte("sentinel")) {
			leaked = append(leaked, path)
		}
		return nil
	})
	if len(visible) == 0 {
		t.Fatal("the synced networks row was not found under the home dir; the grep is not looking where sync writes")
	}
	if !sawNested {
		t.Fatal("the JSON inside the nested string was withheld whole instead of scrubbed")
	}
	if len(leaked) > 0 {
		t.Fatalf("secret sentinel persisted in %v", leaked)
	}
}

// api-keys is not in the default sync set (no local command reads it; its
// rows carry ApiKey.token) but stays syncable by name.
func TestDefaultSyncSkipsApiKeys(t *testing.T) {
	for _, r := range defaultSyncResources() {
		if r == "api-keys" {
			t.Fatal("api-keys is in defaultSyncResources()")
		}
	}
	if !syncResourceWalksAfterClear("api-keys", "", nil) {
		t.Fatal("api-keys must stay syncable by name")
	}
}

// The secret-key generate / rotate and api-keys revoke commands write their
// response through to the store. The response is the parent network or api
// key record; the store must keep it on the parent partition without the
// "typed-table upsert failed" warning (a false alarm on a successful
// credential operation) and without the secret.
func TestCredentialActionsWriteThroughWithoutWarning(t *testing.T) {
	home := testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/networks/100/secret_key") && (r.Method == http.MethodPost || r.Method == http.MethodPatch):
			fmt.Fprint(w, `{"data":{"id":"100","type":"networks","attributes":{"name":"VISIBLE-HQ","secret_key":"`+syncHFSentinel+`"},"relationships":{"organization":{"data":{"id":"1","type":"organizations"}}}}}`)
		case r.URL.Path == "/v1/api_keys/4/revoke" && r.Method == http.MethodPost:
			fmt.Fprint(w, `{"data":{"id":"4","type":"api_keys","attributes":{"name":"VISIBLE-KEY","status":"revoked","token":"***`+syncHFSentinel+`"}}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")

	// The store writes its warning to os.Stderr directly.
	captureStderr := func(fn func()) string {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := os.Stderr
		os.Stderr = w
		done := make(chan string, 1)
		go func() {
			b, _ := io.ReadAll(r)
			done <- string(b)
		}()
		defer func() { os.Stderr = orig }()
		fn()
		w.Close()
		os.Stderr = orig
		return <-done
	}
	for _, args := range [][]string{
		{"networks", "secret-key", "networks-generate", "100"},
		{"networks", "secret-key", "networks-rotate", "100"},
		{"api-keys", "revoke", "api-keys", "4"},
	} {
		var out string
		var runErr error
		stderr := captureStderr(func() { out, runErr = syncHFRun(args...) })
		if runErr != nil {
			t.Fatalf("%v: %v\n%s", args, runErr, out)
		}
		if strings.Contains(stderr+out, "typed-table upsert failed") {
			t.Fatalf("%v printed a false typed-table warning:\n%s%s", args, stderr, out)
		}
	}

	var leaked []string
	sawNetwork, sawKey := false, false
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.Contains(filepath.Base(path), ".db") {
			return nil
		}
		b, _ := os.ReadFile(path)
		lower := bytes.ToLower(b)
		sawNetwork = sawNetwork || bytes.Contains(lower, []byte("visible-hq"))
		sawKey = sawKey || bytes.Contains(lower, []byte("visible-key"))
		if bytes.Contains(lower, []byte("sentinel")) {
			leaked = append(leaked, path)
		}
		return nil
	})
	if !sawNetwork || !sawKey {
		t.Fatalf("the write-through rows were not found in the store (network=%v key=%v)", sawNetwork, sawKey)
	}
	if len(leaked) > 0 {
		t.Fatalf("secret sentinel persisted in %v", leaked)
	}
}

// syncFleetAPI serves list endpoints from a per-path function of the query
// and records every query it saw.
type syncFleetAPI struct {
	mu      sync.Mutex
	queries map[string][]url.Values
}

func syncFleetServer(t *testing.T, body func(path string, q url.Values) (string, bool)) *syncFleetAPI {
	t.Helper()
	api := &syncFleetAPI{queries: map[string][]url.Values{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.queries[r.URL.Path] = append(api.queries[r.URL.Path], r.URL.Query())
		api.mu.Unlock()
		b, ok := body(r.URL.Path, r.URL.Query())
		if !ok && strings.HasPrefix(r.URL.Path, "/v1/networks/") {
			b, ok = `{"data":[]}`, true // networks' dependents (lan_ips, subnets)
		}
		if !ok {
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, b)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	return api
}

func (a *syncFleetAPI) last(path string) url.Values {
	a.mu.Lock()
	defer a.mu.Unlock()
	qs := a.queries[path]
	if len(qs) == 0 {
		return nil
	}
	return qs[len(qs)-1]
}

func syncHFRunOK(t *testing.T, args ...string) {
	t.Helper()
	if out, err := syncHFRun(args...); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

func syncHFWantComplete(t *testing.T, dbPath, resource string, want int, step string) {
	t.Helper()
	if got, _ := syncHFComplete(t, dbPath, resource); got != want {
		t.Fatalf("%s: %s last_attempt_complete=%d, want %d\n%s", step, resource, got, want, syncHFState(t, dbPath))
	}
}

func syncHFAgents(n, page int) string {
	if page != 1 {
		return `{"data":[]}`
	}
	items := make([]string, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{"id":"a%d","type":"user_agents","attributes":{"hostname":"PC-%d"}}`, i, i))
	}
	return `{"data":[` + strings.Join(items, ",") + `]}`
}

// last_attempt_complete = 1 must mean the collection holds the whole fleet
// as the API returns it unfiltered: a walk narrowed by --resource-param,
// --param or --global-param, or cut by --max-pages, records incomplete; a
// plain repeated sync stays complete; --full after a narrowed walk restores
// complete.
func TestSyncCompleteMeansWholeFleet(t *testing.T) {
	testenv.Isolate(t)
	full := true // serve a 100-item page 1 (a continuation exists) when false
	api := syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		page := 1
		if p := q.Get("page[number]"); p != "" {
			fmt.Sscan(p, &page)
		}
		if !full {
			if page <= 2 {
				return syncHFAgents(100, 1), true
			}
			return `{"data":[]}`, true
		}
		if q.Get("organization_id") != "" || q.Get("x") != "" {
			return syncHFAgents(1, page), true
		}
		return syncHFAgents(2, page), true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	ua := []string{"sync", "--resources", "user-agents", "--db", dbPath}
	with := func(extra ...string) []string { return append(append([]string(nil), ua...), extra...) }

	syncHFRunOK(t, with("--resource-param", "user-agents:organization_id=1")...)
	if api.last("/v1/user_agents/all").Get("organization_id") != "1" {
		t.Fatal("the --resource-param was not sent; the test is not exercising a narrowed walk")
	}
	syncHFWantComplete(t, dbPath, "user-agents", 0, "fresh mirror, --resource-param")
	syncHFRunOK(t, ua...)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "plain sync")
	syncHFRunOK(t, ua...)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "plain sync repeated")
	syncHFRunOK(t, with("--param", "x=1")...)
	syncHFWantComplete(t, dbPath, "user-agents", 0, "--param")
	syncHFRunOK(t, with("--full")...)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "--full after a narrowed walk")
	syncHFRunOK(t, with("--global-param", "x=1")...)
	syncHFWantComplete(t, dbPath, "user-agents", 0, "--global-param")
	syncHFRunOK(t, ua...)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "plain sync after --global-param")
	full = false
	_, _ = syncHFRun(with("--max-pages", "1")...)
	syncHFWantComplete(t, dbPath, "user-agents", 0, "--max-pages cap hit")
}

// A narrowed --full walk must not prune the rows its filter excluded.
func TestSyncNarrowedFullWalkNeverPrunes(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		page := 1
		if p := q.Get("page[number]"); p != "" {
			fmt.Sscan(p, &page)
		}
		if q.Get("organization_id") != "" {
			return syncHFAgents(1, page), true
		}
		return syncHFAgents(2, page), true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--db", dbPath)
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--resource-param", "user-agents:organization_id=1", "--db", dbPath)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count("user-agents"); n != 2 {
		t.Fatalf("a narrowed --full walk pruned the rows its filter excluded: %d rows left, want 2", n)
	}
}

// Incremental walks: a fresh mirror plus --since stays incomplete (and sets
// no watermark); the next plain sync walks everything and completes; a
// repeated plain sync (window = watermark) stays complete; --since that
// starts after the watermark leaves a gap, records incomplete and keeps the
// watermark.
func TestSyncIncrementalCompletion(t *testing.T) {
	testenv.Isolate(t)
	api := syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/distributors/msps" {
			return "", false
		}
		if p := q.Get("page[number]"); p != "" && p != "1" {
			return `{"data":[]}`, true
		}
		return `{"data":[{"id":"m1","type":"msps","attributes":{"name":"MSP"}}]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	msps := []string{"sync", "--resources", "distributors-msps", "--db", dbPath}
	with := func(extra ...string) []string { return append(append([]string(nil), msps...), extra...) }
	watermark := func() string {
		t.Helper()
		db, err := store.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		_, at, _, err := db.GetSyncState("distributors-msps")
		if err != nil {
			t.Fatal(err)
		}
		if at.IsZero() {
			return ""
		}
		return at.UTC().Format(time.RFC3339)
	}

	syncHFRunOK(t, with("--since", "7d")...)
	if api.last("/v1/distributors/msps").Get("updated_at_from") == "" {
		t.Fatal("--since did not send updated_at_from; the test is not exercising an incremental walk")
	}
	syncHFWantComplete(t, dbPath, "distributors-msps", 0, "fresh mirror, --since 7d")
	if w := watermark(); w != "" {
		t.Fatalf("an incomplete incremental walk set the watermark %s", w)
	}
	syncHFRunOK(t, msps...)
	if api.last("/v1/distributors/msps").Get("updated_at_from") != "" {
		t.Fatal("the plain sync after an incomplete --since walk was still incremental")
	}
	syncHFWantComplete(t, dbPath, "distributors-msps", 1, "plain sync")
	syncHFRunOK(t, msps...)
	if api.last("/v1/distributors/msps").Get("updated_at_from") == "" {
		t.Fatal("the repeated plain sync did not resume from the watermark")
	}
	syncHFWantComplete(t, dbPath, "distributors-msps", 1, "plain sync repeated (incremental from the watermark)")
	syncHFRunOK(t, with("--since", "7d")...)
	syncHFWantComplete(t, dbPath, "distributors-msps", 1, "--since 7d covering the watermark")

	// Age the watermark so --since 7d starts after it.
	old := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE sync_state SET last_synced_at = ? WHERE resource_type = 'distributors-msps'`, old); err != nil {
		t.Fatal(err)
	}
	db.Close()
	syncHFRunOK(t, with("--since", "7d")...)
	syncHFWantComplete(t, dbPath, "distributors-msps", 0, "--since 7d leaving a gap after the watermark")
	if w := watermark(); w != old {
		t.Fatalf("a gapped incremental walk moved the watermark: %s, want %s", w, old)
	}
	syncHFRunOK(t, msps...)
	syncHFWantComplete(t, dbPath, "distributors-msps", 1, "plain sync from the kept watermark")
}

// A walk resumed from a saved cursor saw only the pages after it: incomplete,
// and the next plain sync (from the top) completes.
func TestSyncResumedWalkIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		page := 1
		if p := q.Get("page[number]"); p != "" {
			fmt.Sscan(p, &page)
		}
		return syncHFAgents(2, page), true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--resources", "user-agents", "--db", dbPath)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("user-agents", "2", 2); err != nil {
		t.Fatal(err)
	}
	db.Close()
	// The resumed walk warns (an all-warned run exits non-zero) instead of
	// reporting the collection done.
	out, _ := syncHFRun("sync", "--resources", "user-agents", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "user-agents", 0, "walk resumed from page 2")
	if !strings.Contains(out, `"reason":"resumed_from_cursor"`) || !strings.Contains(out, "sync --full --resources user-agents") {
		t.Fatalf("a resumed walk did not warn with the --full remedy:\n%s", out)
	}
	if strings.Contains(out, `{"event":"sync_complete","resource":"user-agents"`) || !strings.Contains(out, `"warned":1`) {
		t.Fatalf("a resumed walk still reported the collection done:\n%s", out)
	}
	out = func() string {
		o, err := syncHFRun("sync", "--resources", "user-agents", "--db", dbPath)
		if err != nil {
			t.Fatalf("next plain sync: %v\n%s", err, o)
		}
		return o
	}()
	syncHFWantComplete(t, dbPath, "user-agents", 1, "next plain sync")
	if strings.Contains(out, "resumed_from_cursor") {
		t.Fatalf("a walk from the top still warned as resumed:\n%s", out)
	}
}

// A walk a page cap stopped is not reported done, and a --param scoped walk
// succeeds but says it is not a complete snapshot (sync_complete with
// complete=false), matching last_attempt_complete=0.
func TestSyncIncompleteWalksNeverReportDone(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		page := 1
		if p := q.Get("page[number]"); p != "" {
			fmt.Sscan(p, &page)
		}
		return syncHFAgents(100, page), true // a full page 1: a continuation exists
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out, _ := syncHFRun("sync", "--full", "--resources", "user-agents", "--max-pages", "1", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "user-agents", 0, "walk capped at one page")
	if strings.Contains(out, `{"event":"sync_complete","resource":"user-agents"`) || !strings.Contains(out, `"warned":1`) || !strings.Contains(out, "sync --full --resources user-agents") {
		t.Fatalf("a capped walk reported the collection done:\n%s", out)
	}
	out = func() string {
		o, err := syncHFRun("sync", "--full", "--resources", "user-agents", "--resource-param", "user-agents:organization_id=1", "--db", dbPath)
		if err != nil {
			t.Fatalf("scoped sync: %v\n%s", err, o)
		}
		return o
	}()
	syncHFWantComplete(t, dbPath, "user-agents", 0, "scoped walk")
	if !strings.Contains(out, `"complete":false`) || !strings.Contains(out, `"reason":"param_scoped"`) {
		t.Fatalf("a scoped walk did not say it is not a complete snapshot:\n%s", out)
	}
}

// A plain sync stores the global (MSP-shared) policies: the policies list
// request always carries include_global_policies=true, which is a built-in
// default and not a narrowing param, so the collection is complete and
// lists where finds a global policy's entry.
func TestSyncPoliciesIncludeGlobal(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if p := q.Get("page[number]"); p != "" && p != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		switch r.URL.Path {
		case "/v1/organizations/all":
			fmt.Fprint(w, `{"data":[{"id":"1","type":"organizations","attributes":{"name":"Org One"}}]}`)
		case "/v1/policies/all":
			own := `{"id":"10","type":"policies","attributes":{"name":"Std","organization_id":1,"is_global_policy":false,"whitelist_domains":[],"blacklist_domains":[]}}`
			global := `{"id":"90","type":"policies","attributes":{"name":"MSP Global","is_global_policy":true,"whitelist_domains":[],"blacklist_domains":["blocked.example"]}}`
			if q.Get("include_global_policies") == "true" {
				fmt.Fprint(w, `{"data":[`+own+`,`+global+`]}`)
				return
			}
			fmt.Fprint(w, `{"data":[`+own+`]}`)
		default:
			fmt.Fprint(w, `{"data":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--resources", "organizations,networks,policies,scheduled-policies,user-agents", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "policies", 1, "plain sync with the built-in include_global_policies default")
	out, err := syncHFRun("lists", "where", "blocked.example", "--db", dbPath)
	if err != nil {
		t.Fatalf("lists where: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"policy_id": "90"`) || !strings.Contains(out, `"is_global": true`) {
		t.Fatalf("lists where did not find the global policy's block-list entry:\n%s", out)
	}
	if strings.Contains(out, "incomplete_collections") {
		t.Fatalf("lists where reported an incomplete mirror after a full plain sync:\n%s", out)
	}
}

// Dependents of a narrowed parent walk cover only the parents it stored, and
// a dependent narrowed by its own --resource-param is not its whole set.
func TestSyncDependentsNeedACompleteParent(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/networks/all" {
			return "", false
		}
		if p := q.Get("page[number]"); p != "" && p != "1" {
			return `{"data":[]}`, true
		}
		return `{"data":[{"id":"50","type":"networks","attributes":{"name":"HQ"}}]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--resources", "networks", "--resource-param", "networks:organization_id=1", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "networks", 0, "narrowed networks walk")
	syncHFWantComplete(t, dbPath, "lan_ips", 0, "lan_ips under a narrowed networks walk")
	syncHFRunOK(t, "sync", "--resources", "networks", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "networks", 1, "plain networks walk")
	syncHFWantComplete(t, dbPath, "lan_ips", 1, "lan_ips under a complete networks walk")
	syncHFRunOK(t, "sync", "--resources", "networks", "--resource-param", "lan_ips:x=1", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "networks", 1, "networks walk with only a dependent param")
	syncHFWantComplete(t, dbPath, "lan_ips", 0, "lan_ips narrowed by its own --resource-param")
	syncHFWantComplete(t, dbPath, "subnets", 1, "subnets untouched by the lan_ips param")
}

func TestSyncSinceCoveredByWatermark(t *testing.T) {
	w := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		sinceTS, effective string
		watermark          time.Time
		want               bool
	}{
		{"", "", time.Time{}, true},                                          // not incremental
		{"", "2026-10-10T00:00:00Z", w, true},                                // resumed from the watermark
		{"2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z", w, true},            // --since before the watermark
		{"2026-10-15T00:00:00Z", "2026-10-15T00:00:00Z", w, false},           // gap after the watermark
		{"2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z", time.Time{}, false}, // fresh mirror
	} {
		if got := syncSinceCoveredByWatermark(c.sinceTS, c.effective, c.watermark); got != c.want {
			t.Errorf("syncSinceCoveredByWatermark(%q, %q, %v) = %v, want %v", c.sinceTS, c.effective, c.watermark, got, c.want)
		}
	}
}

// `sync --full --since 7d` fetches only the window: it must not prune the
// rows outside it.
func TestSyncFullSinceNeverPrunes(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/distributors/msps" {
			return "", false
		}
		if p := q.Get("page[number]"); p != "" && p != "1" {
			return `{"data":[]}`, true
		}
		if q.Get("updated_at_from") != "" {
			return `{"data":[{"id":"m1","type":"msps","attributes":{"name":"Changed"}}]}`, true
		}
		return `{"data":[{"id":"m1","type":"msps","attributes":{"name":"A"}},{"id":"m2","type":"msps","attributes":{"name":"B"}}]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--resources", "distributors-msps", "--db", dbPath)
	syncHFRunOK(t, "sync", "--full", "--since", "7d", "--resources", "distributors-msps", "--db", dbPath)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count("distributors-msps"); n != 2 {
		t.Fatalf("--full --since pruned rows outside its window: %d rows left, want 2", n)
	}
	syncHFWantComplete(t, dbPath, "distributors-msps", 0, "--full --since 7d")
}
