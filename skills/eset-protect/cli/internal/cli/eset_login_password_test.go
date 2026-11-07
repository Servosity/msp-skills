// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestAuthLoginProvesThePassword (handfixes.json: login-proves-the-password):
// an explicit `auth login` re-minted with the cached refresh token, so after
// one good login a wrong ESET_PROTECT_PASSWORD still logged in with exit 0.
func TestAuthLoginProvesThePassword(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("grant_type") == "password" && r.PostForm.Get("password") != "s3cret" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"tok-1","refresh_token":"ref-1","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(tok.Close)
	esetTokenURLOverride = tok.URL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"deviceGroups":[],"nextPageToken":""}`)
	}))
	t.Cleanup(api.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", api.URL)
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_TOKEN", "")
	t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
	login := func() error {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"auth", "login"})
		return cmd.Execute()
	}
	t.Setenv("ESET_PROTECT_PASSWORD", "s3cret")
	if err := login(); err != nil {
		t.Fatalf("login with the right password: %v", err)
	}
	t.Setenv("ESET_PROTECT_PASSWORD", "wrong")
	if err := login(); err == nil {
		t.Fatal("auth login succeeded with a wrong password via the cached refresh token")
	}
}
