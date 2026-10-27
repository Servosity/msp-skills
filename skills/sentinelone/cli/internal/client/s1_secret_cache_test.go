// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the HTTP response cache never writes a
// credential to disk.

package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/cliutil/testenv"
	"sentinelone-pp-cli/internal/config"
)

// TestResponseCacheNeverHoldsSecrets: a cached GET of a secret endpoint (the
// account uninstall password, the agent passphrases, a login-by-token
// session, the Ranger credential details) and of an ordinary read whose rows
// carry a secret field (a site's registrationToken) leaves no cache file
// holding the value, while an ordinary read is still cached.
func TestResponseCacheNeverHoldsSecrets(t *testing.T) {
	testenv.Isolate(t)
	bodies := map[string]string{
		"/accounts/123/uninstall-password/view": `{"data":{"password":"cacheuninstallpwsentinel"}}`,
		"/agents/passphrases":                   `{"data":[{"id":"11","computerName":"WS1","passphrase":"CACHE PASSPHRASE SENTINELWORDS"}]}`,
		"/users/login/by-token":                 `{"data":{"token":"cacheloginsessionsentinel"}}`,
		"/ranger/cred-groups/details":           `{"data":[{"id":"c1","community":"cacherangercommunitysentinel"}]}`,
		"/sites/s1":                             `{"data":{"id":"s1","name":"CacheSite","registrationToken":"cachesiteregtokensentinel"}}`,
		"/accounts":                             `{"data":[{"id":"a1","name":"ORDINARYCACHEDACCOUNT"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bodies[r.URL.Path]))
	}))
	t.Cleanup(server.Close)

	c := New(&config.Config{BaseURL: server.URL, SentineloneApiToken: "cache-test-api-token-value"}, 5*time.Second, 0)
	c.HTTPClient = server.Client()
	if c.cacheDir == "" {
		t.Fatal("response cache is disabled in this environment; the test would prove nothing")
	}
	ctx := context.Background()
	for path := range bodies {
		got, err := c.Get(ctx, path, map[string]string{"limit": "10"})
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		// The caller (the operator's own read) still gets the answer.
		if !bytes.Equal(bytes.TrimSpace(got), []byte(bodies[path])) {
			t.Fatalf("GET %s answer changed: %s", path, got)
		}
	}

	cacheRoot, err := cliutil.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	sentinels := []string{"cacheuninstallpwsentinel", "CACHE PASSPHRASE SENTINELWORDS", "cacheloginsessionsentinel", "cacherangercommunitysentinel", "cachesiteregtokensentinel"}
	cachedOrdinary := false
	_ = filepath.Walk(cacheRoot, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		b, rerr := os.ReadFile(filepath.Clean(p))
		if rerr != nil {
			return nil
		}
		for _, s := range sentinels {
			if strings.Contains(string(b), s) {
				t.Errorf("cache file %s holds %q", p, s)
			}
		}
		if strings.Contains(string(b), "ORDINARYCACHEDACCOUNT") {
			cachedOrdinary = true
		}
		return nil
	})
	if !cachedOrdinary {
		t.Fatal("an ordinary GET was not cached; the cache guard must not disable caching")
	}
}
