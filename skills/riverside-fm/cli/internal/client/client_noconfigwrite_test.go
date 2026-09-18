// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"riverside-fm-pp-cli/internal/config"
)

// Issue #270: the real persistence route on this connector is the sliding
// session in do(): every response rotates riverside_auth_access and the client
// writes the merged cookie string back through SaveAccessToken. Under
// RIVERSIDE_FM_NO_CONFIG_WRITE the rotated cookie must stay in memory (so the
// retry and every later request in this process use it) while the config file
// stays absent; without the switch the file carries the rotated cookie.

const (
	ncwSeedCookie    = "riverside_auth_access=seed.seed.seed; riverside_auth_refresh=ref.ref.ref"
	ncwRotatedAccess = "rot.rot.rot"
)

func ncwRotate(t *testing.T, set bool) (*config.Config, string, bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "riverside_auth_access", Value: ncwRotatedAccess, Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if set {
		t.Setenv(config.NoConfigWriteEnv, "1")
	} else {
		os.Unsetenv(config.NoConfigWriteEnv)
	}
	cfgPath := filepath.Join(home, "config.toml")
	cfg := &config.Config{BaseURL: srv.URL, Path: cfgPath, AccessToken: ncwSeedCookie, AuthSource: "browser"}
	c := New(cfg, 10*time.Second, 0)
	c.NoCache = true
	c.HTTPClient = srv.Client() // the rotation logic lives in do(), not the transport
	if _, err := c.Get("/api/probe", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := "riverside_auth_access=" + ncwRotatedAccess + "; riverside_auth_refresh=ref.ref.ref"
	if cfg.AccessToken != want {
		t.Fatalf("rotated cookie not in memory: %q, want %q", cfg.AccessToken, want)
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

func TestNoConfigWriteKeepsTheRotatedCookieInMemoryOnly(t *testing.T) {
	if _, body, exists := ncwRotate(t, true); exists {
		t.Errorf("%s=1: the cookie rotation still wrote the config file:\n%s", config.NoConfigWriteEnv, body)
	}
	_, body, exists := ncwRotate(t, false)
	if !exists || !strings.Contains(body, ncwRotatedAccess) {
		t.Fatalf("without the switch the rotation cached nothing, so the ON case proves nothing; file exists=%v body=%q", exists, body)
	}
}
