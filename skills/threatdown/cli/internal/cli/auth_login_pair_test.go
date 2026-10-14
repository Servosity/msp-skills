// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): auth login given one flag
// saves the exact client pair it minted with, so the saved secret can mint
// again later and never sits beside another client's ID.
func TestAuthLoginSavesThePairItUsed(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("THREATDOWN_HOME", home)
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "login", "--client-secret", "adminsec"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out.String())
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_TOKEN_URL", "")
	cfg, err := config.Load(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "admin" || cfg.ClientSecret != "adminsec" {
		t.Fatalf("login saved id=%q secret=%q, want the pair it used", cfg.ClientID, cfg.ClientSecret)
	}
}
