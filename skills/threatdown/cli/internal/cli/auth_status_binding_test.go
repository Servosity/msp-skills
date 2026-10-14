// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/config"
)

func runAuthCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err := cmd.Execute()
	return out.String(), err
}

func authStatusJSON(t *testing.T) (map[string]any, error) {
	t.Helper()
	out, err := runAuthCmd(t, "", "auth", "status", "--json")
	var got map[string]any
	if jerr := json.Unmarshal([]byte(out[:strings.LastIndex(out, "}")+1]), &got); jerr != nil {
		t.Fatalf("auth status --json: %v\n%s", jerr, out)
	}
	return got, err
}

// hand-fix (handfixes.json auth-status-tracks-binding): auth status reports
// what this process can authenticate with, the way doctor does.
func TestAuthStatusTracksTokenBinding(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	t.Setenv("THREATDOWN_HOME", t.TempDir())
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")

	// Fresh home, env pair exported: it can mint, so it is authenticated.
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "adminsec")
	got, err := authStatusJSON(t)
	if err != nil || got["authenticated"] != true {
		t.Fatalf("env pair on a fresh home: authenticated=%v err=%v", got["authenticated"], err)
	}

	// Saved admin login, but this process names another client and has
	// no secret: every call would refuse the cached token.
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "admin", "--client-secret", "adminsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "reporting")
	got, err = authStatusJSON(t)
	if err == nil || got["authenticated"] != false || got["token_bound_elsewhere"] != true {
		t.Fatalf("token bound elsewhere: got %v err=%v, want authenticated=false + error", got, err)
	}
}

// hand-fix (handfixes.json set-token-clears-saved-client): set-token clears
// the saved client pair even while THREATDOWN_CLIENT_ID is exported, so a
// leftover client_id never refuses the new token.
func TestSetTokenClearsSavedClientUnderEnvID(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("THREATDOWN_HOME", home)
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "admin", "--client-secret", "adminsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	if out, err := runAuthCmd(t, "settok\n", "auth", "set-token"); err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_TOKEN_URL", "")
	cfg, err := config.Load(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" || cfg.AccessToken != "settok" {
		t.Fatalf("after set-token: id=%q secret=%q token=%q, want only the token", cfg.ClientID, cfg.ClientSecret, cfg.AccessToken)
	}
}

// Placeholder or unresolved ${...} credentials cannot mint, so neither auth
// status nor doctor may call them configured; a real env pair is configured
// for both on a fresh home.
func TestAuthStatusAndDoctorAgreeOnEnvPairs(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	for _, tc := range []struct {
		id, secret string
		want       bool
	}{
		{"your-token-here", "your-token-here", false},
		{"${user_config.client_id}", "${user_config.client_secret}", false},
		{"admin", "adminsec", true},
	} {
		t.Setenv("THREATDOWN_HOME", t.TempDir())
		t.Setenv("THREATDOWN_CLIENT_ID", tc.id)
		t.Setenv("THREATDOWN_CLIENT_SECRET", tc.secret)
		got, _ := authStatusJSON(t)
		if got["authenticated"] != tc.want {
			t.Errorf("%s: auth status authenticated=%v, want %v", tc.id, got["authenticated"], tc.want)
		}
		out, _ := runAuthCmd(t, "", "doctor", "--json")
		var doc map[string]any
		_ = json.Unmarshal([]byte(out[strings.Index(out, "{"):strings.LastIndex(out, "}")+1]), &doc)
		if configured := doc["auth"] == "configured"; configured != tc.want {
			t.Errorf("%s: doctor auth=%v, want configured=%v", tc.id, doc["auth"], tc.want)
		}
	}
}

func TestSetTokenDoesNotInheritOldExpiry(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("THREATDOWN_HOME", home)
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "admin", "--client-secret", "adminsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	if out, err := runAuthCmd(t, "settok\n", "auth", "set-token"); err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	cfg, err := config.Load(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TokenExpiry.IsZero() {
		t.Fatalf("set-token kept the replaced token's expiry %v", cfg.TokenExpiry)
	}
}
