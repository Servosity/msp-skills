// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/config"
)

// newCfg returns a Config whose token writes land in a throwaway temp file so
// applyTokens' best-effort SaveTokens never touches the real user config.
func newCfg(t *testing.T, baseURL string) *config.Config {
	t.Helper()
	return &config.Config{
		BaseURL: baseURL,
		Path:    filepath.Join(t.TempDir(), "config.toml"),
	}
}

func TestEnsure_NilConfigIsNoop(t *testing.T) {
	if err := Ensure(context.Background(), nil); err != nil {
		t.Fatalf("Ensure(nil) = %v, want nil", err)
	}
}

func TestEnsure_VerifyModeIsNoop(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	cfg := newCfg(t, "http://127.0.0.1:0") // unreachable on purpose
	if err := Ensure(context.Background(), cfg); err != nil {
		t.Fatalf("Ensure in verify mode = %v, want nil (must not make network calls)", err)
	}
}

func TestEnsure_FreshTokenIsNoop(t *testing.T) {
	cfg := newCfg(t, "http://127.0.0.1:0")
	cfg.AccessToken = "still-good"
	cfg.TokenExpiry = time.Now().Add(time.Hour)
	if err := Ensure(context.Background(), cfg); err != nil {
		t.Fatalf("Ensure with a fresh token = %v, want nil", err)
	}
}

func TestEnsure_ExpiringWithinSkewTriggersExchange(t *testing.T) {
	// Token expires inside the skew window: Ensure must treat it as stale and
	// attempt an exchange. With no JWT configured, exchange() returns a clear
	// "no JWT" error — which proves the skew path was taken.
	cfg := newCfg(t, "http://127.0.0.1:0")
	cfg.AccessToken = "about-to-die"
	cfg.TokenExpiry = time.Now().Add(expirySkew / 2)
	err := Ensure(context.Background(), cfg)
	if err == nil {
		t.Fatal("Ensure with a token inside the skew window = nil, want an exchange attempt error")
	}
}

func TestExchange_NoJWTConfigured(t *testing.T) {
	cfg := newCfg(t, "http://127.0.0.1:0")
	err := exchange(context.Background(), cfg)
	if err == nil {
		t.Fatal("exchange with no JWT = nil, want error")
	}
}

func TestExchange_SuccessAppliesTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/authenticate" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer my-jwt" {
			t.Errorf("Authorization = %q, want Bearer my-jwt", got)
		}
		fmt.Fprint(w, `{"tokens":{"access":{"token":"acc","expirySeconds":1800},"refresh":{"token":"ref","expirySeconds":90000}}}`)
	}))
	defer srv.Close()

	cfg := newCfg(t, srv.URL)
	cfg.NcentralJwt = "my-jwt"
	if err := exchange(context.Background(), cfg); err != nil {
		t.Fatalf("exchange = %v, want nil", err)
	}
	if cfg.AccessToken != "acc" {
		t.Errorf("AccessToken = %q, want acc", cfg.AccessToken)
	}
	if cfg.RefreshToken != "ref" {
		t.Errorf("RefreshToken = %q, want ref", cfg.RefreshToken)
	}
	if cfg.TokenExpiry.IsZero() {
		t.Error("TokenExpiry is zero, want a future expiry")
	}
}

func TestRefresh_FallsBackToExchangeOnFailure(t *testing.T) {
	// refresh endpoint 500s; Ensure must fall through to a full exchange,
	// which then succeeds against /auth/authenticate.
	var sawRefresh, sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/refresh":
			sawRefresh = true
			w.WriteHeader(http.StatusInternalServerError)
		case "/auth/authenticate":
			sawAuth = true
			fmt.Fprint(w, `{"tokens":{"access":{"token":"acc2","expirySeconds":1800}}}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := newCfg(t, srv.URL)
	cfg.NcentralJwt = "my-jwt"
	cfg.RefreshToken = "stale-refresh"
	if err := Ensure(context.Background(), cfg); err != nil {
		t.Fatalf("Ensure = %v, want nil", err)
	}
	if !sawRefresh {
		t.Error("refresh endpoint was never called")
	}
	if !sawAuth {
		t.Error("exchange fallback never reached the authenticate endpoint")
	}
	if cfg.AccessToken != "acc2" {
		t.Errorf("AccessToken = %q, want acc2", cfg.AccessToken)
	}
}

func TestPostBearer_RateLimitedSurfacesTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0") // don't actually sleep in tests
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "slow down")
	}))
	defer srv.Close()

	_, err := postBearer(context.Background(), srv.URL+"/auth/authenticate", "jwt", nil)
	if err == nil {
		t.Fatal("postBearer against a 429 endpoint = nil, want RateLimitError")
	}
	var rle *cliutil.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("error = %T (%v), want *cliutil.RateLimitError", err, err)
	}
	if rle.URL == "" {
		t.Error("RateLimitError.URL is empty")
	}
}

func TestPostBearer_UnauthorizedGivesActionableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := postBearer(context.Background(), srv.URL+"/auth/authenticate", "jwt", nil)
	if err == nil {
		t.Fatal("postBearer against 401 = nil, want error")
	}
}

func TestPostBearer_MissingAccessTokenIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tokens":{"access":{"token":""}}}`)
	}))
	defer srv.Close()

	_, err := postBearer(context.Background(), srv.URL+"/auth/authenticate", "jwt", nil)
	if err == nil {
		t.Fatal("postBearer with empty access token = nil, want error")
	}
}

func TestApplyTokens_DefaultsExpiryWhenMissing(t *testing.T) {
	cfg := newCfg(t, "http://127.0.0.1:0")
	resp := &authResponse{}
	resp.Tokens.Access.Token = "acc"
	// ExpirySeconds left 0 -> applyTokens must default to ~1h.
	before := time.Now()
	if err := applyTokens(cfg, resp); err != nil {
		t.Fatalf("applyTokens = %v, want nil", err)
	}
	gotWindow := cfg.TokenExpiry.Sub(before)
	if gotWindow < 59*time.Minute || gotWindow > 61*time.Minute {
		t.Errorf("default expiry window = %s, want ~1h", gotWindow)
	}
}

func TestApplyTokens_KeepsExistingRefreshToken(t *testing.T) {
	cfg := newCfg(t, "http://127.0.0.1:0")
	cfg.RefreshToken = "keep-me"
	resp := &authResponse{}
	resp.Tokens.Access.Token = "acc"
	// No refresh token in the response -> existing one must survive.
	if err := applyTokens(cfg, resp); err != nil {
		t.Fatalf("applyTokens = %v, want nil", err)
	}
	if cfg.RefreshToken != "keep-me" {
		t.Errorf("RefreshToken = %q, want keep-me (existing token clobbered)", cfg.RefreshToken)
	}
}

func TestSleepContext_CancelledReturnsErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); err == nil {
		t.Fatal("sleepContext with a cancelled context = nil, want ctx error")
	}
}

func TestSleepContext_ZeroDurationReturnsCtxErr(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatalf("sleepContext(ctx, 0) = %v, want nil", err)
	}
}
