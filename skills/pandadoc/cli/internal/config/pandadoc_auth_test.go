package config

import "testing"

// TestAuthHeaderAPIKeyPrefix pins the PandaDoc auth contract: the upstream API
// requires `Authorization: API-Key <key>` (spec x-auth-value-prefix). The
// generator emits a bare-token AuthHeader; this test guards the hand-applied
// prefix so a regen without the fix fails loudly instead of silently breaking
// every live call with a 401.
func TestAuthHeaderAPIKeyPrefix(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"bare key gets prefix", "abc123", "API-Key abc123"},
		{"already prefixed is untouched", "API-Key abc123", "API-Key abc123"},
		{"bearer token is untouched", "Bearer xyz789", "Bearer xyz789"},
		{"empty token yields empty header", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{PandadocApiKey: tt.token}
			if got := c.AuthHeader(); got != tt.want {
				t.Errorf("AuthHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAuthHeaderValWins pins precedence: an explicit auth_header config value
// overrides the env-var-derived key entirely.
func TestAuthHeaderValWins(t *testing.T) {
	c := &Config{AuthHeaderVal: "API-Key explicit", PandadocApiKey: "ignored"}
	if got := c.AuthHeader(); got != "API-Key explicit" {
		t.Errorf("AuthHeader() = %q, want %q", got, "API-Key explicit")
	}
}
