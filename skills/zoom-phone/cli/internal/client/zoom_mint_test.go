// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"

	"zoom-phone-pp-cli/internal/config"
)

// Zoom S2S OAuth needs grant_type=account_credentials plus account_id; the
// generated client_credentials grant is rejected (hand-fix, see
// .printing-press-patches/zoom-phone-account-credentials-grant.json).
func TestMintUsesZoomAccountCredentialsGrant(t *testing.T) {
	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	var form map[string]string
	var user, pass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = map[string]string{"grant_type": r.PostForm.Get("grant_type"), "account_id": r.PostForm.Get("account_id"), "scope": r.PostForm.Get("scope")}
		user, pass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3599}`))
	}))
	defer srv.Close()

	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenURL = srv.URL
	cfg.ZoomPhoneAccountId = "acct-123"
	c := New(cfg, 5*time.Second, 0)
	if err := c.mintClientCredentials(context.Background(), "cid", "csecret"); err != nil {
		t.Fatal(err)
	}
	if form["grant_type"] != "account_credentials" || form["account_id"] != "acct-123" || form["scope"] != "" {
		t.Fatalf("token form = %v; want account_credentials + account_id, no scope", form)
	}
	if user != "cid" || pass != "csecret" {
		t.Fatalf("basic auth = %q/%q", user, pass)
	}
	if cfg.AccessToken != "tok" {
		t.Fatalf("access token not stored: %q", cfg.AccessToken)
	}

	cfg.ZoomPhoneAccountId = ""
	if err := c.mintClientCredentials(context.Background(), "cid", "csecret"); err == nil {
		t.Fatal("mint without account ID should fail")
	}
}

// An automatic mint with environment credentials saves the account ID together
// with the app and token, so a later run cannot pair this token with another account.
func TestAutoMintPersistsAccountID(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "ZOOM_PHONE_HOME"} {
		t.Setenv(k, dir)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok-A","expires_in":3599}`))
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-env-A")
	path := filepath.Join(dir, "config.toml")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenURL = srv.URL
	c := New(cfg, 5*time.Second, 0)
	if err := c.mintClientCredentials(context.Background(), "cid-A", "sec-A"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "")
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ZoomPhoneAccountId != "acct-env-A" || reloaded.AccessToken != "tok-A" {
		t.Fatalf("saved token without its account: account=%q token=%q", reloaded.ZoomPhoneAccountId, reloaded.AccessToken)
	}
}

// Codex review r1 (class mint-error-disclosure): every token-endpoint path
// (the automatic S2S mint and the refresh-token exchange) shows only the HTTP
// status and a recognized OAuth error code, never endpoint text that may echo
// the client secret or refresh token in some encoding.
func TestTokenEndpointErrorsWithholdEndpointText(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "ZOOM_PHONE_HOME"} {
		t.Setenv(k, dir)
	}
	secret, refresh := "csecret-value-123", "refresh-token-value-456"
	var echoes []string
	for _, v := range []string{secret, refresh, "cid:" + secret} {
		echoes = append(echoes, v, base64.StdEncoding.EncodeToString([]byte(v)), base64.RawURLEncoding.EncodeToString([]byte(v)), hex.EncodeToString([]byte(v)), strings.ToUpper(v))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"reason":"` + strings.Join(echoes, " ") + `","error":"invalid_grant"}`))
	}))
	defer srv.Close()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenURL = srv.URL
	cfg.ZoomPhoneAccountId = "acct-123"
	c := New(cfg, 5*time.Second, 0)
	check := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: HTTP 400 accepted", label)
		}
		for _, e := range echoes {
			if strings.Contains(err.Error(), e) {
				t.Fatalf("%s error repeats endpoint text %q: %v", label, e, err)
			}
		}
		if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid_grant") {
			t.Fatalf("%s error lost the status or OAuth error code: %v", label, err)
		}
	}
	check("mint", c.mintClientCredentials(context.Background(), "cid", secret))
	cfg.ClientID, cfg.ClientSecret, cfg.RefreshToken = "cid", secret, refresh
	check("refresh", c.refreshAccessToken(context.Background()))
}
