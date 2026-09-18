// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"n-central-pp-cli/internal/config"
)

// Issue #270: the real persistence route on this connector is applyTokens,
// which SaveTokens best-effort after every JWT exchange and refresh. Under
// N_CENTRAL_NO_CONFIG_WRITE the exchange must still succeed and the access
// token must land in memory, while the config file stays absent; without the
// switch the file carries the exchanged token.

const ncwAccess = "not-a-real-access-token"

func ncwExchange(t *testing.T, set bool) (*config.Config, string, bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/authenticate" || r.Method != http.MethodPost {
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tokens":{"access":{"token":"` + ncwAccess + `","type":"Bearer","expirySeconds":3600},"refresh":{"token":"not-a-real-refresh","type":"Body","expirySeconds":90000}}}`))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	if set {
		t.Setenv(config.NoConfigWriteEnv, "1")
	} else {
		os.Unsetenv(config.NoConfigWriteEnv)
	}
	cfgPath := filepath.Join(home, "config.toml")
	cfg := &config.Config{BaseURL: srv.URL, Path: cfgPath, NcentralJwt: "not-a-real-jwt"}
	if err := Ensure(context.Background(), cfg); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if cfg.AccessToken != ncwAccess {
		t.Fatalf("exchanged token not in memory: %q", cfg.AccessToken)
	}
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, "", false
		}
		t.Fatalf("reading %s: %v", cfgPath, err)
	}
	return cfg, string(body), true
}

func TestNoConfigWriteGatesTheJWTExchange(t *testing.T) {
	if _, body, exists := ncwExchange(t, true); exists {
		t.Errorf("%s=1: the JWT exchange still wrote the config file:\n%s", config.NoConfigWriteEnv, body)
	}
	_, body, exists := ncwExchange(t, false)
	if !exists || !strings.Contains(body, ncwAccess) {
		t.Fatalf("without the switch the exchange cached nothing, so the ON case proves nothing; file exists=%v body=%q", exists, body)
	}
}
