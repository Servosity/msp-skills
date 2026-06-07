// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the per-instance OAuth token URL derivation.

package client

import (
	"testing"

	"appdirect-pp-cli/internal/config"
)

func TestResolveAppDirectTokenURL(t *testing.T) {
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
		{"empty base falls back to default", "", "", "https://marketplace.appdirect.com/oauth2/token"},
		// Adversarial / mistyped inputs must never route credentials to a
		// userinfo-confused host; they fall back to the spec default.
		{"userinfo confusion rejected", "", "victim.com@attacker.com", "https://marketplace.appdirect.com/oauth2/token"},
		{"explicit scheme with userinfo rejected", "", "https://user@victim.com/api", "https://marketplace.appdirect.com/oauth2/token"},
		{"protocol-relative rejected", "", "//evil.com/api", "https://marketplace.appdirect.com/oauth2/token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{TokenURL: tc.tokenURL, BaseURL: tc.baseURL}
			if got := ResolveAppDirectTokenURL(cfg); got != tc.want {
				t.Fatalf("ResolveAppDirectTokenURL(%q, %q) = %q, want %q", tc.tokenURL, tc.baseURL, got, tc.want)
			}
		})
	}
	if got := ResolveAppDirectTokenURL(nil); got != "https://marketplace.appdirect.com/oauth2/token" {
		t.Fatalf("nil config = %q, want default", got)
	}
}
