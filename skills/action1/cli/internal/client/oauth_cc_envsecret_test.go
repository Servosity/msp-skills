// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"action1-pp-cli/internal/config"
)

// End-to-end receipt for issue #266 on this connector: an ORDINARY
// authenticated command mints a bearer from ACTION1_CLIENT_ID /
// ACTION1_CLIENT_SECRET and caches it, which is the code path that reaches
// Config.save(). The minted token may be cached; the env-supplied client
// credentials must not follow it onto disk.
func TestMintDoesNotPersistEnvSuppliedCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"minted-bearer","refresh_token":"minted-refresh","expires_in":3600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("ACTION1_CONFIG", path)
	t.Setenv("ACTION1_CLIENT_ID", "env-client-id")
	t.Setenv("ACTION1_CLIENT_SECRET", "env-client-secret")
	t.Setenv("ACTION1_BASE_URL", srv.URL)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	c := New(cfg, 10*time.Second, 0)

	h, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader: %v", err)
	}
	if h != "Bearer minted-bearer" {
		t.Fatalf("auth header = %q, want Bearer minted-bearer", h)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the mint should still have written its token cache: %v", err)
	}
	got := string(raw)
	for _, secret := range []string{"env-client-id", "env-client-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("env-supplied %q reached %s in cleartext:\n%s", secret, path, got)
		}
	}
	if !strings.Contains(got, "minted-bearer") {
		t.Errorf("the minted token was not cached, so the fix broke the cache:\n%s", got)
	}
}
