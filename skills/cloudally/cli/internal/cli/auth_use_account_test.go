// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudally-pp-cli/internal/config"
)

// fakeMintAPI serves the refresh and partner-access-token endpoints and
// records what each call carried.
type fakeMintAPI struct {
	srv           *httptest.Server
	mu            sync.Mutex
	refreshCalls  int
	refreshTokens []string
	mintAuth      []string
}

func newFakeMintAPI(t *testing.T) *fakeMintAPI {
	t.Helper()
	f := &fakeMintAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == cloudallyRefreshPath:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.refreshCalls++
			f.refreshTokens = append(f.refreshTokens, body["refreshToken"])
			_, _ = w.Write([]byte(`{"accessToken":"new-partner","expiresIn":1893456000000,"refreshToken":"new-refresh","refreshTokenExpiresIn":1894000000000}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			f.mintAuth = append(f.mintAuth, r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"accessToken":"account-token"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	return f
}

func seedActiveToken(t *testing.T, baseURL, access, refresh string, expiry time.Time) {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = baseURL
	if err := cfg.SaveTokens("cid", "csecret", access, refresh, expiry); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestPartnerTokenNeedsRefresh(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		refresh string
		expiry  time.Time
		want    bool
	}{
		{"r", now.Add(-time.Hour), true},
		{"r", now.Add(30 * time.Second), true},
		{"r", now.Add(60 * time.Second), true},
		{"r", now.Add(2 * time.Minute), false},
		{"", now.Add(-time.Hour), false},
		{"r", time.Time{}, false},
	}
	for _, tc := range cases {
		if got := partnerTokenNeedsRefresh(tc.refresh, tc.expiry, now); got != tc.want {
			t.Fatalf("needsRefresh(%q, %s) = %v, want %v", tc.refresh, tc.expiry, got, tc.want)
		}
	}
}

func TestUseAccountRefreshesStashedPartnerTokenBeforeMinting(t *testing.T) {
	isolateAuthState(t)
	api := newFakeMintAPI(t)
	// An account token is active; the stashed partner token expires in 10s.
	seedActiveToken(t, api.srv.URL, "old-account-token", "", time.Now().Add(time.Hour))
	if err := saveAuthContext(&authContext{Scope: authScopeAccount, AccountID: "first", PartnerAccessToken: "old-partner",
		PartnerRefreshToken: "old-refresh", PartnerTokenExpiry: time.Now().Add(10 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a"); err != nil {
		t.Fatalf("use-account: %v (%s)", err, stderr)
	}
	if api.refreshCalls != 1 || api.refreshTokens[0] != "old-refresh" {
		t.Fatalf("refresh calls = %d with %v, want 1 with old-refresh", api.refreshCalls, api.refreshTokens)
	}
	if len(api.mintAuth) != 1 || api.mintAuth[0] != "Bearer new-partner" {
		t.Fatalf("mint auth = %v, want the refreshed partner token", api.mintAuth)
	}
	actx, _ := loadAuthContext()
	if actx.PartnerAccessToken != "new-partner" || actx.PartnerRefreshToken != "new-refresh" || actx.PartnerTokenExpiry.UnixMilli() != 1893456000000 {
		t.Fatalf("stash after refresh = %+v", actx)
	}
	if actx.AccountID != "5f1c2a" || actx.Scope != authScopeAccount {
		t.Fatalf("sidecar scope = %+v", actx)
	}
	cfg, _ := config.Load("")
	if cfg.AccessToken != "account-token" {
		t.Fatalf("active token = %q", cfg.AccessToken)
	}

	// A fresh stashed token is used as is.
	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "abc123"); err != nil {
		t.Fatalf("second use-account: %v", err)
	}
	if api.refreshCalls != 1 || api.mintAuth[1] != "Bearer new-partner" {
		t.Fatalf("fresh token should not refresh: calls=%d mint=%v", api.refreshCalls, api.mintAuth)
	}
}

func TestUseAccountRefreshesExpiredActivePartnerToken(t *testing.T) {
	isolateAuthState(t)
	api := newFakeMintAPI(t)
	seedActiveToken(t, api.srv.URL, "old-partner", "old-refresh", time.Now().Add(-time.Minute))
	if err := saveAuthContext(&authContext{Scope: authScopePartner, PartnerAccessToken: "old-partner", PartnerRefreshToken: "old-refresh",
		PartnerTokenExpiry: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a"); err != nil {
		t.Fatalf("use-account: %v (%s)", err, stderr)
	}
	if api.refreshCalls != 1 || len(api.mintAuth) != 1 || api.mintAuth[0] != "Bearer new-partner" {
		t.Fatalf("refresh calls = %d, mint auth = %v", api.refreshCalls, api.mintAuth)
	}
	actx, _ := loadAuthContext()
	if actx.PartnerAccessToken != "new-partner" || actx.PartnerRefreshToken != "new-refresh" {
		t.Fatalf("stash = %+v", actx)
	}
	// use-account --partner brings back the refreshed partner session.
	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "--partner"); err != nil {
		t.Fatalf("use-account --partner: %v", err)
	}
	cfg, _ := config.Load("")
	if cfg.AccessToken != "new-partner" || cfg.RefreshToken != "new-refresh" {
		t.Fatalf("restored partner tokens = (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
	}
}
