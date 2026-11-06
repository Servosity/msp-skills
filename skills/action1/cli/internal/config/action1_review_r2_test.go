// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// Codex r2 (action1 reprint on 4.34.2) class cross-tenant offline store
// aliasing: tenant A's client pair is stored and an explicit token for tenant
// B (ACTION1_OAUTH2, or a configured auth header) authenticates. The local
// database follows the credential that authenticates: B's token opens B's
// store, never the one named by A's client ID.
func TestR2ExplicitTokenOpensItsOwnStoreNotTheStoredClients(t *testing.T) {
	a := &Config{ClientID: "client-a-4f1c", ClientSecret: "secret-a-9d2e"}
	for name, c := range map[string]*Config{
		"ACTION1_OAUTH2": {ClientID: "client-a-4f1c", ClientSecret: "secret-a-9d2e", Action1Oauth2: "tenant-b-token"},
		"auth_header":    {ClientID: "client-a-4f1c", ClientSecret: "secret-a-9d2e", AuthHeaderVal: "Bearer tenant-b-token"},
	} {
		if c.StoreScopeCredential() == a.StoreScopeCredential() {
			t.Fatalf("%s for tenant B opens tenant A's store (scope %q)", name, c.StoreScopeCredential())
		}
		other := &Config{Action1Oauth2: "tenant-c-token"}
		if c.StoreScopeCredential() == other.StoreScopeCredential() {
			t.Fatalf("%s: two explicit tokens share one store", name)
		}
	}
}
