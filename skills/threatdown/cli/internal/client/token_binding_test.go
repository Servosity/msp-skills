// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"testing"
	"time"

	"threatdown-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): a cached token minted for
// one OAuth client or scope must not be reused by a process configured with
// another.
func TestCachedTokenBoundToClientAndScope(t *testing.T) {
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "")
	minted := config.TokenBindingFor("admin-client", resolveClientCredentialsScopes(), effectiveTokenURL(nil))
	newClient := func(id string) *Client {
		return &Client{Config: &config.Config{
			AccessToken:  "cached-execute-token",
			TokenExpiry:  time.Now().Add(time.Hour),
			ClientID:     id,
			ClientSecret: "s",
			TokenBinding: minted,
		}}
	}
	if newClient("admin-client").needsClientCredentialsMint() {
		t.Fatal("same client, same scope: cached token should be reused")
	}
	if !newClient("reporting-client").needsClientCredentialsMint() {
		t.Fatal("different client reused another client's cached token")
	}
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "read")
	if !newClient("admin-client").needsClientCredentialsMint() {
		t.Fatal("narrower THREATDOWN_OAUTH_SCOPE reused a token minted with execute scope")
	}
}
