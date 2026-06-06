package config

import "testing"

// PagerDuty requires the Authorization header to be exactly
// "Token token=<key>". The OpenAPI apiKey scheme only carries the bare key, so
// AuthHeader() must prepend the prefix. These tests pin that behavior — a
// masked --dry-run hides the prefix, so a regression would otherwise ship
// silently and every live call would 401.
func TestAuthHeaderPrependsTokenPrefix(t *testing.T) {
	c := &Config{PagerdutyApiKey: "u+ABCDEF0123456789"}
	got := c.AuthHeader()
	want := "Token token=u+ABCDEF0123456789"
	if got != want {
		t.Fatalf("AuthHeader() = %q, want %q", got, want)
	}
}

func TestAuthHeaderDoesNotDoubleWrap(t *testing.T) {
	c := &Config{PagerdutyApiKey: "Token token=u+ABCDEF0123456789"}
	got := c.AuthHeader()
	want := "Token token=u+ABCDEF0123456789"
	if got != want {
		t.Fatalf("AuthHeader() double-wrapped: got %q, want %q", got, want)
	}
}

func TestAuthHeaderEmptyWhenNoKey(t *testing.T) {
	c := &Config{}
	if got := c.AuthHeader(); got != "" {
		t.Fatalf("AuthHeader() with no key = %q, want empty", got)
	}
}

// An explicitly saved full header value wins verbatim (user pasted the whole
// "Token token=..." string into the config's auth_header field).
func TestAuthHeaderValWinsVerbatim(t *testing.T) {
	c := &Config{AuthHeaderVal: "Token token=explicit"}
	if got := c.AuthHeader(); got != "Token token=explicit" {
		t.Fatalf("AuthHeader() = %q, want verbatim AuthHeaderVal", got)
	}
}
