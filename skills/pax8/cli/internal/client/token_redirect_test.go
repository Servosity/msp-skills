// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pax8-pp-cli/internal/config"
)

// TestTokenMintRefusesARedirectToAnotherOrigin: the automatic
// client-credentials mint (and the refresh grant) POSTed through c.HTTPClient,
// whose CheckRedirect follows a 307 to another host and replays the body, the
// client secret included. Hand-owned (handfixes.json: token-request-stays-on-origin).
func TestTokenMintRefusesARedirectToAnotherOrigin(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "PAX8_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	var elsewhere atomic.Int32
	var leaked atomic.Bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "stub-client-secret") {
			leaked.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer first.Close()
	cfg := &config.Config{
		Path:         filepath.Join(home, "config.toml"),
		BaseURL:      first.URL,
		TokenURL:     first.URL + "/oauth/token",
		ClientID:     "stub-client-id",
		ClientSecret: "stub-client-secret",
	}
	c := New(cfg, 5*time.Second, 0)
	_, _, _ = c.do(context.Background(), http.MethodGet, "/v1/items", nil, nil, nil)
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d token request(s) (client secret in the body: %v)", n, leaked.Load())
	}
	if cfg.AccessToken != "" {
		t.Fatal("a token minted through a cross-origin redirect was cached")
	}
}
