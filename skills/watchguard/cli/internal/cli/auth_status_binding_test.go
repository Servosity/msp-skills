// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil/testenv"
	"watchguard-pp-cli/internal/config"
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
	end := strings.LastIndex(out, "}")
	if end < 0 {
		t.Fatalf("auth status --json printed no JSON: %v\n%s", err, out)
	}
	if jerr := json.Unmarshal([]byte(out[:end+1]), &got); jerr != nil {
		t.Fatalf("auth status --json: %v\n%s", jerr, out)
	}
	return got, err
}

func doctorJSON(t *testing.T) map[string]any {
	t.Helper()
	out, _ := runAuthCmd(t, "", "doctor", "--json")
	var doc map[string]any
	start, end := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if start < 0 || end < start {
		t.Fatalf("doctor --json printed no JSON:\n%s", out)
	}
	if err := json.Unmarshal([]byte(out[start:end+1]), &doc); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	return doc
}

// tokenServer answers every token request with a token and every API call
// with an empty object.
func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func isolateAuthEnv(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	for _, name := range []string{"WATCHGUARD_OAUTH_SCOPE", "WATCHGUARD_AUDIENCE", "WATCHGUARD_API_KEY", "WATCHGUARD_BASE_URL", "WATCHGUARD_ACCOUNT_ID", "WATCHGUARD_CLIENT_ID", "WATCHGUARD_CLIENT_SECRET"} {
		t.Setenv(name, "")
	}
}

// hand-fix (handfixes.json auth-status-tracks-binding): auth status reports
// what this process can authenticate with, the way doctor does.
func TestAuthStatusTracksTokenBinding(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")

	// Fresh home, env pair exported: it can mint, so it is authenticated.
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "rwsec")
	got, err := authStatusJSON(t)
	if err != nil || got["authenticated"] != true {
		t.Fatalf("env pair on a fresh home: authenticated=%v err=%v", got["authenticated"], err)
	}

	// Saved read-write login, but this process names the read-only Access ID
	// and has no secret: every call would refuse the cached token.
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "ro")
	got, err = authStatusJSON(t)
	if !errors.Is(err, client.ErrTokenBoundElsewhere) || got["authenticated"] != false || got["token_bound_elsewhere"] != true {
		t.Fatalf("token bound elsewhere: got %v err=%v, want authenticated=false + ErrTokenBoundElsewhere", got, err)
	}
	out, err := runAuthCmd(t, "", "auth", "status")
	if !errors.Is(err, client.ErrTokenBoundElsewhere) || !strings.Contains(out, "different OAuth client") {
		t.Fatalf("human auth status with a token bound elsewhere: err=%v\n%s", err, out)
	}
	if code := ExitCode(err); code != 4 {
		t.Fatalf("token bound elsewhere exit code = %d, want 4", code)
	}
}

// hand-fix (handfixes.json set-token-clears-saved-client): set-token clears
// the saved client pair even while WATCHGUARD_CLIENT_ID is exported, so a
// leftover client_id never refuses the new token.
func TestSetTokenClearsSavedClientUnderEnvID(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	if out, err := runAuthCmd(t, "settok\n", "auth", "set-token"); err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_TOKEN_URL", "")
	cfg, err := config.Load(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" || cfg.AccessToken != "settok" {
		t.Fatalf("after set-token: id=%q secret=%q token=%q, want only the token", cfg.ClientID, cfg.ClientSecret, cfg.AccessToken)
	}
	if got, err := authStatusJSON(t); err != nil || got["authenticated"] != true {
		t.Fatalf("set-token token with no client configured: authenticated=%v err=%v", got["authenticated"], err)
	}
}

// The pasted token's lifetime is unknown; set-token never inherits the
// replaced OAuth token's expiry.
func TestSetTokenDoesNotInheritOldExpiry(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "rwsec"); err != nil {
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

// Placeholder or unresolved ${...} credentials cannot mint, so neither auth
// status nor doctor may call them configured; a real env pair is configured
// for both on a fresh home.
func TestAuthStatusAndDoctorAgreeOnEnvPairs(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	for _, tc := range []struct {
		id, secret string
		want       bool
	}{
		{"your-token-here", "your-token-here", false},
		{"${user_config.client_id}", "${user_config.client_secret}", false},
		{"<client-id>", "<client-secret>", false},
		{"<your-client-id>", "<value>", false},
		{"rw", "rwsec", true},
	} {
		t.Setenv("WATCHGUARD_HOME", t.TempDir())
		t.Setenv("WATCHGUARD_CLIENT_ID", tc.id)
		t.Setenv("WATCHGUARD_CLIENT_SECRET", tc.secret)
		got, _ := authStatusJSON(t)
		if got["authenticated"] != tc.want {
			t.Errorf("%s: auth status authenticated=%v, want %v", tc.id, got["authenticated"], tc.want)
		}
		doc := doctorJSON(t)
		if configured := doc["auth"] == "configured"; configured != tc.want {
			t.Errorf("%s: doctor auth=%v, want configured=%v", tc.id, doc["auth"], tc.want)
		}
	}
}

// A wrong API password is a credentials error in doctor, not "API unreachable".
func TestDoctorReportsRejectedMintAsCredentials(t *testing.T) {
	isolateAuthEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "wrong")
	doc := doctorJSON(t)
	creds, _ := doc["credentials"].(string)
	api, _ := doc["api"].(string)
	if !strings.HasPrefix(creds, "ERROR") || strings.Contains(api, "unreachable") {
		t.Fatalf("doctor api=%q credentials=%q, want a credentials ERROR", api, creds)
	}
}

// doctor reports a token bound to another client (no secret to mint a
// replacement) as a credentials ERROR without probing the API.
func TestDoctorReportsTokenBoundElsewhere(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "ro")
	doc := doctorJSON(t)
	creds, _ := doc["credentials"].(string)
	api, _ := doc["api"].(string)
	if !strings.HasPrefix(creds, "ERROR") || !strings.Contains(creds, "different OAuth client") || !strings.HasPrefix(api, "not probed") {
		t.Fatalf("doctor api=%q credentials=%q, want a token-bound-elsewhere credentials ERROR", api, creds)
	}
}

// An API command refused because the cached token is bound to another client
// exits with the auth code (4), like any other credential problem.
func TestBoundElsewhereReadExitsWithAuthCode(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	t.Setenv("WATCHGUARD_NO_LEARN", "1")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "ro")
	out, err := runAuthCmd(t, "", "firebox-devices", "--json", "--no-cache")
	if code := ExitCode(err); code != 4 {
		t.Fatalf("read with a token bound elsewhere: exit %d (err=%v), want 4\n%s", code, err, out)
	}
}
