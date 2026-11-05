// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

// One-shot URL overrides must not be written back by a real save (token mint).
func TestZoomEnvURLOverridesNotPersisted(t *testing.T) {
	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("ZOOM_PHONE_HOME", dir)
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = 'https://api.zoom.us/v2'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZOOM_PHONE_BASE_URL", "http://127.0.0.1:9/v2")
	t.Setenv("ZOOM_PHONE_TOKEN_URL", "http://127.0.0.1:9/oauth/token")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:9/v2" {
		t.Fatalf("override not applied in-process: %q", cfg.BaseURL)
	}
	if err := cfg.SaveTokens("cid", "sec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "127.0.0.1") {
		t.Fatalf("env URL override persisted into the config file:\n%s", data)
	}
	t.Setenv("ZOOM_PHONE_BASE_URL", "")
	t.Setenv("ZOOM_PHONE_TOKEN_URL", "")
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.BaseURL != "https://api.zoom.us/v2" || strings.Contains(reloaded.TokenURL, "127.0.0.1") {
		t.Fatalf("reload picked up the override: base=%q token=%q", reloaded.BaseURL, reloaded.TokenURL)
	}
}
