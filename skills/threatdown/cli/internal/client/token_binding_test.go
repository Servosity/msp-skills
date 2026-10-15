// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
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

// The binding fails closed: a process that cannot mint (no secret, or no
// client credentials at all) never falls back to another client's token, on
// the request path or the response-cache path.
func TestCachedTokenBindingFailsClosed(t *testing.T) {
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	minted := config.TokenBindingFor("admin-client", resolveClientCredentialsScopes(), effectiveTokenURL(nil))
	cached := func(id, secret, binding string) *Client {
		return &Client{Config: &config.Config{
			AccessToken:  "cached-execute-token",
			TokenExpiry:  time.Now().Add(time.Hour),
			ClientID:     id,
			ClientSecret: secret,
			TokenBinding: binding,
		}}
	}
	ctx := context.Background()
	if _, err := cached("reporting-client", "", minted).authHeader(ctx); err == nil {
		t.Fatal("client ID without a secret rode another client's cached token")
	}
	if err := cached("reporting-client", "", minted).validateCachedRequestAuth(ctx); err == nil {
		t.Fatal("cache path accepted another client's cached token")
	}
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "read")
	if _, err := cached("", "", minted).authHeader(ctx); err == nil {
		t.Fatal("THREATDOWN_OAUTH_SCOPE=read with no client credentials rode the execute token")
	}
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "")
	if h, err := cached("admin-client ", "", minted).authHeader(ctx); err != nil || h != "Bearer cached-execute-token" {
		t.Fatalf("same client (padded ID) lost its own token: %q %v", h, err)
	}
	if !cached("admin-client", "s", "").needsClientCredentialsMint() {
		t.Fatal("unbound token outranked configured client credentials")
	}
	if h, err := cached("", "", "").authHeader(ctx); err != nil || h != "Bearer cached-execute-token" {
		t.Fatalf("auth set-token token unusable with no client credentials: %q %v", h, err)
	}
}

// An auth set-token token is used only while no client ID or secret is set,
// and the refusal is the typed auth-class error.
func TestSetTokenYieldsToAnyClientCredential(t *testing.T) {
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "read")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	c := &Client{Config: &config.Config{AccessToken: "pasted-token", TokenExpiry: time.Now().Add(time.Hour), ClientID: "reporting-client"}}
	if _, err := c.authHeader(context.Background()); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("set-token token rode a client ID with no secret: %v", err)
	}
}
