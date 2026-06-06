package config

import "testing"

// SentinelOne requires the token be sent as "Authorization: ApiToken <token>".
// These tests pin that contract so a future regeneration can't silently drop the
// scheme prefix (which would 401 every call against a real tenant).
func TestAuthHeaderApiTokenPrefix(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"bare token gets prefix", "abc123", "ApiToken abc123"},
		{"whitespace trimmed", "  abc123  ", "ApiToken abc123"},
		{"already prefixed not doubled", "ApiToken abc123", "ApiToken abc123"},
		{"already prefixed lowercase tolerated", "apitoken abc123", "apitoken abc123"},
		{"empty token yields empty header", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{SentineloneApiToken: tc.token}
			if got := c.AuthHeader(); got != tc.want {
				t.Fatalf("AuthHeader() = %q, want %q", got, tc.want)
			}
		})
	}
}

// An explicit full header value (AuthHeaderVal) wins verbatim — it is the
// stored-credential path and must never be re-prefixed.
func TestAuthHeaderExplicitValueWins(t *testing.T) {
	c := &Config{AuthHeaderVal: "ApiToken explicit", SentineloneApiToken: "ignored"}
	if got := c.AuthHeader(); got != "ApiToken explicit" {
		t.Fatalf("AuthHeader() = %q, want explicit value", got)
	}
}
