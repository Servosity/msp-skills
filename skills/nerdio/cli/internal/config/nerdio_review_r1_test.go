// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity: a store named by a bearer token is adopted into the
// client's store, so with a client pair in effect the store a token in the
// files named (a pasted token, another NMM instance's, another client's) is
// moved into this client's store and its records read as this client's
// (internal/cli TestStoreOfAnotherInstanceIsNeverAdopted shows it end to
// end). With a client pair the scope follows the client, whatever token it
// holds, and no token-named store is adopted into it.
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
