// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ninjaone-pp-cli/internal/config"
)

// The client-credentials mint asks NinjaOne for every scope the API defines
// (monitoring, management, control), or exactly the space-separated
// NINJAONE_OAUTH_SCOPE when it is set.
func TestTokenMintRequestsTheConfiguredScopes(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"", "control management monitoring"},
		{"  monitoring   management ", "monitoring management"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			home := t.TempDir()
			for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "NINJAONE_HOME"} {
				t.Setenv(k, home)
			}
			t.Setenv("PRINTING_PRESS_VERIFY", "")
			t.Setenv("NINJAONE_OAUTH_SCOPE", tc.env)
			var mu sync.Mutex
			got := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "token") {
					body, _ := io.ReadAll(r.Body)
					form, _ := url.ParseQuery(string(body))
					mu.Lock()
					got = form.Get("scope")
					mu.Unlock()
					_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
					return
				}
				_, _ = io.WriteString(w, `[]`)
			}))
			defer srv.Close()
			cfg := &config.Config{
				Path:         filepath.Join(home, "config.toml"),
				BaseURL:      srv.URL,
				TokenURL:     srv.URL + "/ws/oauth/token",
				ClientID:     "stub-client-id",
				ClientSecret: "stub-client-secret",
			}
			c := New(cfg, 5*time.Second, 0)
			_, _, _ = c.do(context.Background(), http.MethodGet, "/v2/devices", nil, nil, nil)
			mu.Lock()
			defer mu.Unlock()
			if got != tc.want {
				t.Fatalf("token request scope = %q, want %q", got, tc.want)
			}
		})
	}
}
