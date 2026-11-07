// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package client_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "quickbooks-pp-cli/internal/cli" // installs client.QBOTokenGrant, as every binary does
	"quickbooks-pp-cli/internal/client"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/config"
)

// Invented credentials for these tests.
const (
	xpClientID     = "xp-client-id-0002"
	xpClientSecret = "xp-client-secret-6YbN"
	xpOldAccess    = "xp-access-old-2HsL"
	xpRefresh1     = "xp-refresh-r1-7JwQ"
)

// xpServers is a fake token endpoint that mints a new token pair per grant
// (slowly, so a second refresher has time to queue behind the first) and a
// fake API that accepts only the newest access token.
type xpServers struct {
	grants  atomic.Int64
	latest  atomic.Value // string
	token   *httptest.Server
	api     *httptest.Server
	apiSeen sync.Map // bearer token -> true
}

func newXPServers(t *testing.T, delay time.Duration) *xpServers {
	t.Helper()
	s := &xpServers{}
	s.latest.Store("")
	s.token = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := s.grants.Add(1)
		time.Sleep(delay)
		access := fmt.Sprintf("xp-access-new-%d", n)
		s.latest.Store(access)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"xp-refresh-rotated-%d","expires_in":3600}`, access, n)
	}))
	s.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.apiSeen.Store(tok, true)
		w.Header().Set("Content-Type", "application/json")
		if latest, _ := s.latest.Load().(string); tok == "" || tok != latest {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"fault":{"type":"AUTHENTICATION"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"QueryResponse":{}}`)
	}))
	t.Cleanup(s.token.Close)
	t.Cleanup(s.api.Close)
	return s
}

func xpIsolate(t *testing.T, s *xpServers) {
	t.Helper()
	testenv.Isolate(t)
	for _, name := range []string{"QUICKBOOKS_ACCESS_TOKEN", "QUICKBOOKS_CLIENT_ID", "QUICKBOOKS_CLIENT_SECRET", "QUICKBOOKS_REFRESH_TOKEN", "QUICKBOOKS_REALM_ID", "QUICKBOOKS_DB_PATH", "PRINTING_PRESS_VERIFY"} {
		t.Setenv(name, "")
	}
	t.Setenv("QUICKBOOKS_BASE_URL", s.api.URL+"/v3/company/4620816365")
	t.Setenv("QUICKBOOKS_TOKEN_URL", s.token.URL)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(xpClientID, xpClientSecret, xpOldAccess, xpRefresh1, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func xpGet(c *client.Client) error {
	_, err := c.Get(context.Background(), "/query", map[string]string{"query": "select * from Customer"})
	return err
}

// TestQBORefreshHelperProcess is one refreshing process for
// TestQBOTwoProcessesRefreshOnce. It loads the configuration (an expired
// token), says so, waits for the go signal and makes one request.
func TestQBORefreshHelperProcess(t *testing.T) {
	dir := os.Getenv("QBO_REFRESH_HELPER_DIR")
	if dir == "" {
		t.Skip("helper process only")
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(cfg, 30*time.Second, 0)
	c.NoCache = true
	if err := os.WriteFile(filepath.Join(dir, "ready-"+os.Getenv("QBO_REFRESH_HELPER_ID")), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no go signal")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := xpGet(c); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

// Two processes (the CLI and the MCP server, say) that both loaded an
// expired token make ONE token call between them: the second waits on the
// cross-process lock, re-reads the store and uses the token the first saved.
func TestQBOTwoProcessesRefreshOnce(t *testing.T) {
	s := newXPServers(t, 400*time.Millisecond)
	xpIsolate(t, s)
	dir := t.TempDir()
	var cmds []*exec.Cmd
	var outs []*strings.Builder
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestQBORefreshHelperProcess$", "-test.count=1")
		cmd.Env = append(os.Environ(), "QBO_REFRESH_HELPER_DIR="+dir, fmt.Sprintf("QBO_REFRESH_HELPER_ID=%d", i))
		out := &strings.Builder{}
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds, outs = append(cmds, cmd), append(outs, out)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, e0 := os.Stat(filepath.Join(dir, "ready-0"))
		_, e1 := os.Stat(filepath.Join(dir, "ready-1"))
		if e0 == nil && e1 == nil {
			break
		}
		if time.Now().After(deadline) {
			for _, c := range cmds {
				_ = c.Process.Kill()
			}
			t.Fatalf("helpers never became ready: %s | %s", outs[0], outs[1])
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("helper %d: %v\n%s", i, err, outs[i])
		}
	}
	if n := s.grants.Load(); n != 1 {
		t.Fatalf("two processes made %d token calls, want 1", n)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "xp-access-new-1" || cfg.RefreshToken != "xp-refresh-rotated-1" {
		t.Fatalf("the store does not hold the one refreshed pair")
	}
}

// Concurrent requests in one process, on one shared client and on clients
// built from separate configuration loads (the MCP server builds one per
// call), refresh once.
func TestQBOConcurrentRequestsRefreshOnce(t *testing.T) {
	s := newXPServers(t, 200*time.Millisecond)
	xpIsolate(t, s)
	shared, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	// The shared client keeps its response cache on, so the cache's
	// credential check reads the token while another request refreshes it.
	sharedClient := client.New(shared, 30*time.Second, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := sharedClient
			if i%2 == 1 {
				cfg, err := config.Load("")
				if err != nil {
					errs <- err
					return
				}
				c = client.New(cfg, 30*time.Second, 0)
				c.NoCache = true
			}
			errs <- xpGet(c)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	if n := s.grants.Load(); n != 1 {
		t.Fatalf("8 concurrent requests made %d token calls, want 1", n)
	}
	if _, sawOld := s.apiSeen.Load(xpOldAccess); sawOld {
		t.Fatal("a request went out with the expired token")
	}
}

// A refresh token Intuit rejects (invalid_grant) is sent once per process:
// later requests on a long command do not call the token endpoint again
// with it, and the 401 is returned.
func TestQBORejectedRefreshTokenIsNotResentInTheSameProcess(t *testing.T) {
	testenv.Isolate(t)
	var grants atomic.Int64
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	defer token.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"fault":{"type":"AUTHENTICATION"}}`)
	}))
	defer api.Close()
	for _, name := range []string{"QUICKBOOKS_ACCESS_TOKEN", "QUICKBOOKS_CLIENT_ID", "QUICKBOOKS_CLIENT_SECRET", "QUICKBOOKS_REFRESH_TOKEN", "QUICKBOOKS_REALM_ID", "QUICKBOOKS_DB_PATH", "PRINTING_PRESS_VERIFY"} {
		t.Setenv(name, "")
	}
	t.Setenv("QUICKBOOKS_BASE_URL", api.URL+"/v3/company/4620816365")
	t.Setenv("QUICKBOOKS_TOKEN_URL", token.URL)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(xpClientID, xpClientSecret, xpOldAccess, "xp-refresh-dead-4Kq", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(cfg, 30*time.Second, 0)
	c.NoCache = true
	for i := 0; i < 5; i++ {
		if err := xpGet(c); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
			t.Fatalf("request %d: %v, want the 401", i, err)
		}
	}
	if n := grants.Load(); n != 1 {
		t.Fatalf("a rejected refresh token was sent %d times, want once", n)
	}
}
