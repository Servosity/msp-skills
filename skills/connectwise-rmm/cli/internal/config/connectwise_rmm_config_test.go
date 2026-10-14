// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

func TestAsioRegionAndTokenURL(t *testing.T) {
	cases := []struct{ clientID, wantBase string }{
		{"0e30abcd", AsioNABaseURL},
		{"0E31abcd", "https://openapi.service.euplatform.connectwise.com"},
		{"0e32abcd", "https://openapi.service.auplatform.connectwise.com"},
		{"zzzz", AsioNABaseURL},
		{"", AsioNABaseURL},
	}
	for _, tc := range cases {
		t.Setenv("CONNECTWISE_RMM_BASE_URL", "")
		c := &Config{BaseURL: AsioNABaseURL, ConnectwiseRmmClientId: tc.clientID}
		c.applyAsioRegion()
		if c.BaseURL != tc.wantBase {
			t.Errorf("client %q: base = %q, want %q", tc.clientID, c.BaseURL, tc.wantBase)
		}
		if got, want := c.AsioTokenURL(), tc.wantBase+"/v1/token"; got != want {
			t.Errorf("client %q: token URL = %q, want %q", tc.clientID, got, want)
		}
	}
	// An explicit base URL is never overridden by the client ID prefix.
	c := &Config{BaseURL: "https://mock.local/", ConnectwiseRmmClientId: "0e31abcd"}
	c.applyAsioRegion()
	if c.BaseURL != "https://mock.local/" || c.AsioTokenURL() != "https://mock.local/v1/token" {
		t.Errorf("explicit base overridden: %q %q", c.BaseURL, c.AsioTokenURL())
	}
	// auth login --client-id: a fresh config still mints on the client's region.
	c = &Config{BaseURL: AsioNABaseURL}
	if got := c.AsioTokenURLForClient("0e32abcd"); got != "https://openapi.service.auplatform.connectwise.com/v1/token" {
		t.Errorf("login token URL = %q", got)
	}
	// Config-file credentials (rmm_client_id/secret) are used when no login exists.
	c = &Config{ConnectwiseRmmClientId: "0e31cfg", ConnectwiseRmmClientSecret: "sec"}
	if id, sec := c.AsioClientCredentials(); id != "0e31cfg" || sec != "sec" {
		t.Errorf("config credentials = %q %q", id, sec)
	}
	c = &Config{ClientID: "login", ClientSecret: "lsec", ConnectwiseRmmClientId: "cfg", ConnectwiseRmmClientSecret: "csec"}
	if id, sec := c.AsioClientCredentials(); id != "login" || sec != "lsec" {
		t.Errorf("login credentials must win: %q %q", id, sec)
	}
	// A host inferred from stored EU credentials is not an explicit override:
	// logging in with an AU client still mints on AU.
	c = &Config{BaseURL: AsioNABaseURL, ConnectwiseRmmClientId: "0e31stored"}
	c.snapshotFileConfig()
	c.applyAsioRegion()
	if got := c.AsioTokenURLForClient("0e32login"); got != "https://openapi.service.auplatform.connectwise.com/v1/token" {
		t.Errorf("AU login after EU inference = %q", got)
	}
	// Saving never persists the inferred host (else the next load treats it as explicit).
	if saved := c.configForSave(); saved.BaseURL != AsioNABaseURL {
		t.Errorf("inferred base persisted on save: %q", saved.BaseURL)
	}
	// A config-file base URL is explicit.
	c = &Config{BaseURL: "https://openapi.service.euplatform.connectwise.com"}
	c.snapshotFileConfig()
	if got := c.AsioTokenURLForClient("0e32login"); got != "https://openapi.service.euplatform.connectwise.com/v1/token" {
		t.Errorf("file base must win: %q", got)
	}
	// An explicit token URL wins over the derived one.
	c = &Config{BaseURL: AsioNABaseURL, TokenURL: "https://idp.local/token"}
	if c.AsioTokenURL() != "https://idp.local/token" {
		t.Errorf("token override ignored: %q", c.AsioTokenURL())
	}
}

// The store key must not move when the first mint caches credentials, when
// the env copies are dropped afterwards, or when the secret rotates.
func TestAsioStoreScopeStableAcrossMint(t *testing.T) {
	base := "https://openapi.service.itsupport247.net"
	envOnly := &Config{BaseURL: base, ConnectwiseRmmClientId: "0e30abc", ConnectwiseRmmClientSecret: "s1"}
	minted := &Config{BaseURL: base, ConnectwiseRmmClientId: "0e30abc", ConnectwiseRmmClientSecret: "s1",
		ClientID: "0e30abc", ClientSecret: "s1", AccessToken: "tok"}
	cachedOnly := &Config{BaseURL: base, ClientID: "0e30abc", ClientSecret: "s1", AccessToken: "tok2"}
	rotated := &Config{BaseURL: base, ConnectwiseRmmClientId: "0e30abc", ConnectwiseRmmClientSecret: "s2"}
	want := envOnly.StoreScopeCredential()
	if want == "" {
		t.Fatal("empty store scope for a configured client")
	}
	for name, c := range map[string]*Config{"minted": minted, "cached-only": cachedOnly, "rotated": rotated} {
		if got := c.StoreScopeCredential(); got != want {
			t.Errorf("%s: store scope %q, want %q", name, got, want)
		}
	}
	other := &Config{BaseURL: base, ConnectwiseRmmClientId: "0e30xyz", ConnectwiseRmmClientSecret: "s1"}
	if other.StoreScopeCredential() == want {
		t.Error("two API clients share one store")
	}
}
