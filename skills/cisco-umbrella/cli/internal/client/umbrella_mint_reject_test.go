// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/cliutil/testenv"
	"cisco-umbrella-pp-cli/internal/config"
)

// A token endpoint that answers the client-credentials mint with 401 must
// produce an error, not a deadlock on the token-state lock the mint already
// holds while its error body is shown. Every path that mints is walked: the
// parent token and the child-org token.
func TestRejectedMintReturnsInsteadOfHanging(t *testing.T) {
	for _, org := range []string{"", "7777777"} {
		t.Run("org="+org, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"invalid_client","echo":"mint-reject-secret-0000"}`, http.StatusUnauthorized)
			}))
			defer srv.Close()

			restore, err := cliutil.SetHomeOverride("")
			if err != nil {
				t.Fatalf("reset home override: %v", err)
			}
			t.Cleanup(restore)
			home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
			t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
			t.Setenv("APPDATA", filepath.Join(home, "appdata"))
			t.Setenv("CISCO_UMBRELLA_BASE_URL", "")
			t.Setenv("CISCO_UMBRELLA_TOKEN_URL", "")
			t.Setenv("CISCO_UMBRELLA_CONFIG", "")
			t.Setenv("CISCO_UMBRELLA_API_KEY", "mint-reject-id")
			t.Setenv("CISCO_UMBRELLA_API_SECRET", "mint-reject-secret-0000")

			path := filepath.Join(t.TempDir(), "config.toml")
			content := "base_url = \"" + srv.URL + "\"\ntoken_url = \"" + srv.URL + "/token\"\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			c := New(cfg, 2*time.Second, 0)
			c.HTTPClient = srv.Client()
			if org != "" {
				ScopeToChildOrg(c, org)
			}

			done := make(chan error, 1)
			go func() {
				_, err := c.Get(context.Background(), "/deployments/v2/networks", nil)
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected the rejected mint to fail the call")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("rejected client-credentials mint hung: the token-state lock is taken twice")
			}
		})
	}
}
