// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity: the default store scope hashed the bearer token, so `sync`
// (before the mint saved a token) and the fleet command after it opened two
// databases, and every hourly re-mint another. With a client pair the scope
// is the client ID, whatever token it holds; a token-named store is never
// adopted into it.
func TestStoreScopeFollowsTheClientNotTheToken(t *testing.T) {
	c := &Config{ClientID: "client-a-4f1c", ClientSecret: "secret-a-9d2e"}
	before := c.StoreScopeCredential()
	c.AccessToken = "minted-token-one"
	afterMint := c.StoreScopeCredential()
	c.AccessToken = "minted-token-two"
	afterRemint := c.StoreScopeCredential()
	if before != afterMint || afterMint != afterRemint {
		t.Fatalf("the store scope moved with the token: %q, %q, %q", before, afterMint, afterRemint)
	}
	if got := c.StoreScopeLegacyCredential(); got != "" {
		t.Fatalf("a client's store adopts no token-named store, got legacy scope %q", got)
	}
	other := &Config{ClientID: "client-b-7e3a", ClientSecret: "secret-b-1b8f", AccessToken: "minted-token-two"}
	if other.StoreScopeCredential() == afterRemint {
		t.Fatal("two clients share one store scope")
	}
	pasted := &Config{AccessToken: "pasted-token"}
	if pasted.StoreScopeCredential() == "" {
		t.Fatal("a pasted token keeps the generated scope")
	}
}
