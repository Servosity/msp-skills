// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cisco-umbrella-pp-cli/internal/config"
)

// fakeUmbrella serves /auth/v2/token and echoes the bearer it receives on API calls.
func fakeUmbrella(t *testing.T, mints *atomic.Int32, lastOrg *atomic.Value) *httptest.Server {
	t.Helper()
	// Sandbox every path the client may write (credentials file, caches) so a
	// regression that persists a token can never touch the real data dir.
	t.Setenv("CISCO_UMBRELLA_HOME", t.TempDir())
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v2/token" {
			mints.Add(1)
			user, pass, ok := r.BasicAuth()
			if !ok || user != "key" || pass != "secret" {
				t.Errorf("token request basic auth = %q/%q ok=%v", user, pass, ok)
			}
			_ = r.ParseForm()
			if r.PostForm.Get("grant_type") != "client_credentials" || r.PostForm.Has("scope") {
				t.Errorf("token form = %v; want only grant_type=client_credentials", r.PostForm)
			}
			org := r.Header.Get(OrgHeader)
			lastOrg.Store(org)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token_type":"bearer","access_token":"tok-org-` + org + `","expires_in":3600}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":"` + r.Header.Get("Authorization") + `"}`))
	}))
}

func newTestConfig(dir, base string) *config.Config {
	return &config.Config{
		BaseURL:      base,
		TokenURL:     base + "/auth/v2/token",
		Path:         filepath.Join(dir, "config.toml"),
		ClientID:     "key",
		ClientSecret: "secret",
		AccessToken:  "parent-token",
		TokenExpiry:  time.Now().Add(time.Hour),
	}
}

func TestChildOrgMintsScopedTokenAndNeverPersistsIt(t *testing.T) {
	var mints atomic.Int32
	var lastOrg atomic.Value
	srv := fakeUmbrella(t, &mints, &lastOrg)
	defer srv.Close()
	dir := t.TempDir()

	cfg := newTestConfig(dir, srv.URL)
	c := New(cfg, 5*time.Second, 0)
	ScopeToChildOrg(c, "8675309")
	if !c.NoCache {
		t.Fatal("child-org client must not use the shared response cache")
	}
	got, err := c.Get(context.Background(), "/deployments/v2/networks", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != `{"auth":"Bearer tok-org-8675309"}` {
		t.Fatalf("API saw %s; want the child-org token, never the parent token", got)
	}
	if mints.Load() != 1 || lastOrg.Load() != "8675309" {
		t.Fatalf("mints=%d org=%v; want one mint carrying X-Umbrella-OrgId 8675309", mints.Load(), lastOrg.Load())
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Fatalf("child-org token was persisted to %s (err=%v)", cfg.Path, err)
	}
}

func TestUnscopedClientKeepsParentToken(t *testing.T) {
	var mints atomic.Int32
	var lastOrg atomic.Value
	srv := fakeUmbrella(t, &mints, &lastOrg)
	defer srv.Close()

	c := New(newTestConfig(t.TempDir(), srv.URL), 5*time.Second, 0)
	c.NoCache = true
	got, err := c.Get(context.Background(), "/deployments/v2/networks", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != `{"auth":"Bearer parent-token"}` || mints.Load() != 0 {
		t.Fatalf("got %s with %d mints; want the cached parent token and no mint", got, mints.Load())
	}
	if ChildOrgOf(c) != "" {
		t.Fatal("unscoped client reports a child org")
	}
}

func TestPerOrgClientsStayIndependent(t *testing.T) {
	var mints atomic.Int32
	var lastOrg atomic.Value
	srv := fakeUmbrella(t, &mints, &lastOrg)
	defer srv.Close()
	dir := t.TempDir()

	a := New(newTestConfig(dir, srv.URL), 5*time.Second, 0)
	b := New(newTestConfig(dir, srv.URL), 5*time.Second, 0)
	ScopeToChildOrg(a, "111")
	ScopeToChildOrg(b, "222")
	ga, errA := a.Get(context.Background(), "/x", nil)
	gb, errB := b.Get(context.Background(), "/x", nil)
	if errA != nil || errB != nil {
		t.Fatalf("errs: %v %v", errA, errB)
	}
	if string(ga) != `{"auth":"Bearer tok-org-111"}` || string(gb) != `{"auth":"Bearer tok-org-222"}` {
		t.Fatalf("cross-org token leak: a=%s b=%s", ga, gb)
	}
}

func TestDryRunNeverMintsAToken(t *testing.T) {
	var mints atomic.Int32
	var lastOrg atomic.Value
	inner := fakeUmbrella(t, &mints, &lastOrg)
	defer inner.Close()
	var anyRequest atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anyRequest.Add(1)
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	c := New(newTestConfig(t.TempDir(), srv.URL), 5*time.Second, 0)
	ScopeToChildOrg(c, "8675309") // drops the cached token, so a live call would have to mint
	c.DryRun = true
	if _, err := c.Get(context.Background(), "/deployments/v2/networks", nil); err != nil {
		t.Fatalf("dry-run Get: %v", err)
	}
	if mints.Load() != 0 || anyRequest.Load() != 0 {
		t.Fatalf("--dry-run sent %d request(s) (%d token mints); it must send nothing", anyRequest.Load(), mints.Load())
	}
}

func TestChildOrgDropsStaticAuthorizationHeader(t *testing.T) {
	var mints atomic.Int32
	var lastOrg atomic.Value
	srv := fakeUmbrella(t, &mints, &lastOrg)
	defer srv.Close()
	cfg := newTestConfig(t.TempDir(), srv.URL)
	cfg.Headers = map[string]string{"authorization": "Bearer parent-static", "X-Trace": "1"}
	c := New(cfg, 5*time.Second, 0)
	ScopeToChildOrg(c, "42")
	got, err := c.Get(context.Background(), "/x", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != `{"auth":"Bearer tok-org-42"}` || cfg.Headers["X-Trace"] != "1" {
		t.Fatalf("static Authorization must not override the child token (other headers kept): %s %v", got, cfg.Headers)
	}
}
