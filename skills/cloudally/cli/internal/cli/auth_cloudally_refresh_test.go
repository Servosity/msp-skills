// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/config"
)

func TestCloudallyDecodeExpiry(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresIn int64
		want      time.Time
	}{
		{name: "epoch milliseconds", expiresIn: 1_792_000_000_000, want: time.UnixMilli(1_792_000_000_000).UTC()},
		{name: "epoch seconds", expiresIn: 1_792_000_000, want: time.Unix(1_792_000_000, 0).UTC()},
		{name: "relative seconds", expiresIn: 3600, want: now.Add(time.Hour)},
		{name: "zero", expiresIn: 0, want: time.Time{}},
		{name: "negative", expiresIn: -5, want: time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cloudallyDecodeExpiry(tc.expiresIn, now)
			if !got.Equal(tc.want) {
				t.Fatalf("cloudallyDecodeExpiry(%d) = %s, want %s", tc.expiresIn, got, tc.want)
			}
		})
	}
}

func TestCloudallyParseTokenResponse(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantAccess    string
		wantRefresh   string
		wantExpiryMS  int64
		wantRefreshMS int64
		wantErr       bool
	}{
		{
			name:          "full response with epoch ms",
			body:          `{"accessToken":"acc","tokenType":"Bearer","expiresIn":1792000000000,"refreshToken":"ref","refreshTokenExpiresIn":1793000000000}`,
			wantAccess:    "acc",
			wantRefresh:   "ref",
			wantExpiryMS:  1792000000000,
			wantRefreshMS: 1793000000000,
		},
		{
			name:         "float expiry and no refresh token",
			body:         `{"accessToken":"acc","expiresIn":1792000000000.0}`,
			wantAccess:   "acc",
			wantExpiryMS: 1792000000000,
		},
		{
			name:    "error message only",
			body:    `{"errorMessage":"Bad credentials"}`,
			wantErr: true,
		},
		{
			name:    "empty body",
			body:    "",
			wantErr: true,
		},
		{
			name:    "not json",
			body:    "<html>nope</html>",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			access, refresh, expiry, refreshExpiry, err := cloudallyParseTokenResponse([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got access=%q", access)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if access != tc.wantAccess || refresh != tc.wantRefresh {
				t.Fatalf("tokens = (%q, %q), want (%q, %q)", access, refresh, tc.wantAccess, tc.wantRefresh)
			}
			if tc.wantExpiryMS != 0 && expiry.UnixMilli() != tc.wantExpiryMS {
				t.Fatalf("expiry = %d ms, want %d", expiry.UnixMilli(), tc.wantExpiryMS)
			}
			if tc.wantRefreshMS == 0 && !refreshExpiry.IsZero() {
				t.Fatalf("refresh expiry = %s, want zero", refreshExpiry)
			}
			if tc.wantRefreshMS != 0 && refreshExpiry.UnixMilli() != tc.wantRefreshMS {
				t.Fatalf("refresh expiry = %d ms, want %d", refreshExpiry.UnixMilli(), tc.wantRefreshMS)
			}
		})
	}
}

// isolateAuthState points every credential and state lookup at a throwaway
// home and clears the env vars that would otherwise override the config.
func isolateAuthState(t *testing.T) {
	t.Helper()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatalf("reset home override: %v", err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("CLOUDALLY_HOME", home)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "")
	t.Setenv("CLOUDALLY_PARTNER_EMAIL", "")
	t.Setenv("CLOUDALLY_PARTNER_PASSWORD", "")
	t.Setenv("CLOUDALLY_EMAIL", "")
	t.Setenv("CLOUDALLY_PASSWORD", "")
	t.Setenv("CLOUDALLY_CLIENT_ID", "")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "")
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.VerifyLiveHTTPEnvVar, "")
	t.Setenv(cliutil.DogfoodEnvVar, "")
}

func TestCloudallyRefreshHookRefreshesExpiringToken(t *testing.T) {
	isolateAuthState(t)

	var calls int32
	var gotPath, gotClientID, gotClientSecret, gotContentType string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotPath = r.URL.Path
		gotClientID = r.Header.Get("client-id")
		gotClientSecret = r.Header.Get("client-secret")
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"new-access","tokenType":"Bearer","expiresIn":1792000000000,"refreshToken":"new-refresh","refreshTokenExpiresIn":1793000000000}`))
	}))
	defer srv.Close()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = srv.URL
	cfg.ClientID = "cid"
	cfg.ClientSecret = "csecret"
	cfg.AccessToken = "old-access"
	cfg.RefreshToken = "old-refresh"
	cfg.TokenExpiry = time.Now().Add(10 * time.Second)

	c := client.New(cfg, 5*time.Second, 0)
	if err := cloudallyRefreshHook(c); err != nil {
		t.Fatalf("refresh hook: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
	if gotPath != cloudallyRefreshPath {
		t.Fatalf("path = %q, want %q", gotPath, cloudallyRefreshPath)
	}
	if gotClientID != "cid" || gotClientSecret != "csecret" {
		t.Fatalf("client headers = (%q, %q), want (cid, csecret)", gotClientID, gotClientSecret)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if gotBody["refreshToken"] != "old-refresh" {
		t.Fatalf("body = %v, want refreshToken=old-refresh", gotBody)
	}
	if c.Config.AccessToken != "new-access" || c.Config.RefreshToken != "new-refresh" {
		t.Fatalf("config not updated in place: access=%q refresh=%q", c.Config.AccessToken, c.Config.RefreshToken)
	}
	if c.Config.TokenExpiry.UnixMilli() != 1792000000000 {
		t.Fatalf("expiry = %s, want epoch ms 1792000000000", c.Config.TokenExpiry)
	}

	reloaded, err := config.Load("")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.AccessToken != "new-access" || reloaded.RefreshToken != "new-refresh" {
		t.Fatalf("persisted tokens = (%q, %q), want (new-access, new-refresh)", reloaded.AccessToken, reloaded.RefreshToken)
	}
}

func TestCloudallyRefreshHookSkipsFreshToken(t *testing.T) {
	isolateAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	}))
	defer srv.Close()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = srv.URL
	cfg.AccessToken = "fresh"
	cfg.RefreshToken = "refresh"
	cfg.TokenExpiry = time.Now().Add(time.Hour)
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh hook: %v", err)
	}

	// No refresh token at all: nothing to do even when expired.
	cfg.RefreshToken = ""
	cfg.TokenExpiry = time.Now().Add(-time.Hour)
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh hook without refresh token: %v", err)
	}

	// Dry run never touches the network.
	cfg.RefreshToken = "refresh"
	dry := client.New(cfg, 5*time.Second, 0)
	dry.DryRun = true
	if err := cloudallyRefreshHook(dry); err != nil {
		t.Fatalf("refresh hook in dry run: %v", err)
	}
}

func TestCloudallyRefreshHookFailureIsAuthErr(t *testing.T) {
	isolateAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessage":"refresh token expired"}`))
	}))
	defer srv.Close()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = srv.URL
	cfg.AccessToken = "old"
	cfg.RefreshToken = "stale"
	cfg.TokenExpiry = time.Now().Add(-time.Minute)
	err = cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0))
	if err == nil {
		t.Fatal("expected error from failed refresh")
	}
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 4 {
		t.Fatalf("expected authErr (code 4), got %T %v", err, err)
	}
	if cfg.AccessToken != "old" {
		t.Fatalf("access token changed on failed refresh: %q", cfg.AccessToken)
	}
}

func TestMaskEmail(t *testing.T) {
	tests := map[string]string{
		"tech@example.com": "t***@example.com",
		"a@b.c":            "a***@b.c",
		"nodomain":         "n***",
		"":                 "(none)",
	}
	for in, want := range tests {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
