// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"testing"
)

func TestApplyDerivedOAuthScope(t *testing.T) {
	tests := []struct {
		name      string
		scope     string
		baseURL   string
		subdomain string
		want      string
	}{
		{name: "explicit scope wins", scope: "api://custom/.default", subdomain: "acme", want: "api://custom/.default"},
		{name: "derives from subdomain", subdomain: "acme", want: "https://acme.immy.bot/.default"},
		{name: "base url beats subdomain", baseURL: "https://other.immy.bot", subdomain: "acme", want: "https://other.immy.bot/.default"},
		{name: "trailing slash trimmed", baseURL: "https://acme.immy.bot/", want: "https://acme.immy.bot/.default"},
		{name: "placeholder subdomain ignored", subdomain: "your-instance", want: ""},
		{name: "nothing known", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IMMYBOT_OAUTH_SCOPE", tc.scope)
			t.Setenv("IMMYBOT_BASE_URL", tc.baseURL)
			t.Setenv("IMMYBOT_SUBDOMAIN", tc.subdomain)
			applyDerivedOAuthScope()
			if got := os.Getenv("IMMYBOT_OAUTH_SCOPE"); got != tc.want {
				t.Fatalf("IMMYBOT_OAUTH_SCOPE = %q, want %q", got, tc.want)
			}
		})
	}
}
