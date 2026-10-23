// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/config"
)

// TestMemberCIDMintRefusesATokenRedirectToAnotherOrigin: `fleet sync
// --member-cid` mints a child-tenant token through its own client, and a plain
// http.Client replays the form (client_secret included) on a 307 to another
// host. The second host must receive nothing and the mint must fail.
// Hand-owned (handfixes.json: member-cid-token-stays-on-origin).
func TestMemberCIDMintRefusesATokenRedirectToAnotherOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	var leaked atomic.Bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "stub-client-secret") {
			leaked.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-child-token","token_type":"Bearer","expires_in":1799}`)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer first.Close()

	flags := &rootFlags{timeout: 5 * time.Second}
	_, err := fleetClientForCID(context.Background(), &config.Config{BaseURL: first.URL}, flags,
		first.URL+"/oauth2/token", "stub-client-id", "stub-client-secret", "child-cid")
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d member-CID token request(s) (client secret in the body: %v)", n, leaked.Load())
	}
	if err == nil {
		t.Fatal("the member-CID mint succeeded although its token endpoint redirected to another origin")
	}
}

// TestMemberCIDMintFollowsASameOriginRedirect is the control: a token endpoint
// that moves within its own origin still mints.
func TestMemberCIDMintFollowsASameOriginRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			http.Redirect(w, r, srv.URL+"/oauth2/token-v2", http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "member_cid=child-cid") {
			http.Error(w, "member_cid lost across a same-origin redirect", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-child-token","token_type":"Bearer","expires_in":1799}`)
	}))
	defer srv.Close()

	flags := &rootFlags{timeout: 5 * time.Second}
	if _, err := fleetClientForCID(context.Background(), &config.Config{BaseURL: srv.URL}, flags,
		srv.URL+"/oauth2/token", "stub-client-id", "stub-client-secret", "child-cid"); err != nil {
		t.Fatalf("same-origin member-CID mint: %v", err)
	}
}
