// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

func TestStoreScopeIsPerChildOrg(t *testing.T) {
	cfg := &Config{BaseURL: "https://api.umbrella.com", CiscoUmbrellaApiKey: "key", CiscoUmbrellaApiSecret: "secret"}
	t.Setenv(OrgEnv, "")
	parent := cfg.StoreScopeCredential()
	if parent == "" {
		t.Fatal("parent scope is empty; credentials should scope the store")
	}
	t.Setenv(OrgEnv, "111")
	a := cfg.StoreScopeCredential()
	t.Setenv(OrgEnv, "222")
	b := cfg.StoreScopeCredential()
	if a == parent || b == parent || a == b {
		t.Fatalf("scopes must differ per org: parent=%q a=%q b=%q", parent, a, b)
	}
	if (&Config{BaseURL: "https://api.umbrella.com"}).StoreScopeCredential() != "" {
		t.Fatal("logged-out config must stay unscoped even with an org set")
	}
}

func TestValidOrgID(t *testing.T) {
	for id, want := range map[string]bool{"8675309": true, "": false, "12a": false, "1\r\nX-Evil: 1": false, "-1": false} {
		if got := ValidOrgID(id); got != want {
			t.Errorf("ValidOrgID(%q) = %v, want %v", id, got, want)
		}
	}
}
