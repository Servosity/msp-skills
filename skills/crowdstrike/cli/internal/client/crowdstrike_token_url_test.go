// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the per-instance OAuth token URL derivation.

package client

import (
	"testing"

	"crowdstrike-pp-cli/internal/config"
)

func TestResolveCrowdStrikeTokenURL(t *testing.T) {
	cases := []struct {
		name     string
		tokenURL string
		baseURL  string
		want     string
	}{
		{"explicit token URL wins", "https://custom.example.com/token", "https://mp.example.com/api", "https://custom.example.com/token"},
		{"derived from https base", "", "https://mp.example.com/api", "https://mp.example.com/oauth2/token"},
		{"derived ignores deep path", "", "https://mp.example.com/api/v2/deep", "https://mp.example.com/oauth2/token"},
		{"scheme-less base assumes https", "", "mp.example.com/api", "https://mp.example.com/oauth2/token"},
		{"scheme-less bare host", "", "mp.example.com", "https://mp.example.com/oauth2/token"},
		{"http base preserved", "", "http://localhost:8080/api", "http://localhost:8080/oauth2/token"},
		{"scheme-less host with port", "", "mp.example.com:9000", "https://mp.example.com:9000/oauth2/token"},
		{"empty base falls back to default", "", "", "https://api.crowdstrike.com/oauth2/token"},
		// Each Falcon cloud mints at its own /oauth2/token, never at US-1.
		{"US-1", "", "https://api.crowdstrike.com", "https://api.crowdstrike.com/oauth2/token"},
		{"US-2", "", "https://api.us-2.crowdstrike.com", "https://api.us-2.crowdstrike.com/oauth2/token"},
		{"EU-1", "", "https://api.eu-1.crowdstrike.com", "https://api.eu-1.crowdstrike.com/oauth2/token"},
		{"US-GOV-1", "", "https://api.laggar.gcw.crowdstrike.com", "https://api.laggar.gcw.crowdstrike.com/oauth2/token"},
		{"EU-1 with a trailing slash", "", "https://api.eu-1.crowdstrike.com/", "https://api.eu-1.crowdstrike.com/oauth2/token"},
		{"scheme-less EU-1", "", "api.eu-1.crowdstrike.com", "https://api.eu-1.crowdstrike.com/oauth2/token"},
		{"CROWDSTRIKE_TOKEN_URL wins over the cloud", "https://api.us-2.crowdstrike.com/oauth2/token", "https://api.eu-1.crowdstrike.com", "https://api.us-2.crowdstrike.com/oauth2/token"},
		// Adversarial / mistyped inputs must never route credentials to a
		// userinfo-confused host, nor to CrowdStrike's US-1 cloud (another
		// tenant): they name no token URL at all, and nothing is posted
		// (Codex r1 4.34.2 class credential-sent-to-another-host; 0.1.9 fell
		// back to the spec default here).
		{"userinfo confusion rejected", "", "victim.com@attacker.com", ""},
		{"EU-1 with userinfo rejected", "", "https://ro:pw@api.eu-1.crowdstrike.com", ""},
		{"explicit scheme with userinfo rejected", "", "https://user@victim.com/api", ""},
		{"protocol-relative rejected", "", "//evil.com/api", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{TokenURL: tc.tokenURL, BaseURL: tc.baseURL}
			if got := ResolveCrowdStrikeTokenURL(cfg); got != tc.want {
				t.Fatalf("ResolveCrowdStrikeTokenURL(%q, %q) = %q, want %q", tc.tokenURL, tc.baseURL, got, tc.want)
			}
		})
	}
	if got := ResolveCrowdStrikeTokenURL(nil); got != "https://api.crowdstrike.com/oauth2/token" {
		t.Fatalf("nil config = %q, want default", got)
	}
}
