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
			return `{"data":[{"id":"50","type":"networks","attributes":{"name":"VISIBLE-HQ","secret_key":"` + syncHFSentinel + `","organization_id":1},"relationships":{"organization":{"data":{"id":"1","type":"organizations"}}}}]}`, true
		case "/v1/api_keys":
			return `{"data":[{"id":"4","type":"api_keys","attributes":{"name":"VISIBLE-KEY","token":"` + syncHFSentinel + `","last_four":"z123"}}]}`, true
		}
		return "", false
	})
	if out, err := syncHFRun("sync", "--resources", "networks,api-keys"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var visible, leaked []string
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
		if bytes.Contains(lower, []byte("sentinel")) {
			leaked = append(leaked, path)
		}
		return nil
	})
	if len(visible) == 0 {
		t.Fatal("the synced networks row was not found under the home dir; the grep is not looking where sync writes")
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
